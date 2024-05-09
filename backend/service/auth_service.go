package service

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Alfex4936/chulbong-kr/config"
	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/model"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

type UserDetails struct {
	UserID    int
	Username  string
	Email     string
	Role      string
	ExpiresAt time.Time
}

type AuthService struct {
	DB         *sqlx.DB
	Config     *config.AppConfig
	TokenUtil  *util.TokenUtil
	HTTPClient *http.Client
}

func NewAuthService(db *sqlx.DB, config *config.AppConfig, tokenUtil *util.TokenUtil, httpClient *http.Client) *AuthService {
	return &AuthService{
		DB:         db,
		Config:     config,
		TokenUtil:  tokenUtil,
		HTTPClient: httpClient,
	}
}

func (s *AuthService) VerifyOpaqueToken(token string) (int, time.Time, error) {
	var userID int
	var expiresAt time.Time
	const query = `SELECT UserID, ExpiresAt FROM OpaqueTokens WHERE OpaqueToken = ?`
	err := s.DB.QueryRow(query, token).Scan(&userID, &expiresAt)
	if err != nil {
		return 0, time.Time{}, err
	}
	return userID, expiresAt, nil
}

func (s *AuthService) FetchUserDetails(jwtCookie string, fetchProfile bool) (UserDetails, error) {
	details := UserDetails{}

	// Fetch user ID, role, and expiration based on the opaque token
	const tokenQuery = `
    SELECT u.UserID, u.Role, ot.ExpiresAt
    FROM OpaqueTokens ot
    JOIN Users u ON ot.UserID = u.UserID
    WHERE ot.OpaqueToken = ?`
	err := s.DB.QueryRow(tokenQuery, jwtCookie).Scan(&details.UserID, &details.Role, &details.ExpiresAt)
	if err != nil {
		return UserDetails{}, err
	}

	// Optionally fetch additional user profile information
	if fetchProfile {
		const profileQuery = `SELECT Username, Email FROM Users WHERE UserID = ?`
		err = s.DB.QueryRow(profileQuery, details.UserID).Scan(&details.Username, &details.Email)
		if err != nil {
			return UserDetails{}, err
		}
	}

	return details, nil
}

// SaveUser creates a new user with hashed password
func (s *AuthService) SaveUser(signUpReq *dto.SignUpRequest) (*model.User, error) {
	tx, err := s.DB.Beginx()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// /(?=.*\d)(?=.*[a-z])(?=.*[A-Z]).{8,}/
	// at least one digit (?=.*\d), one lowercase letter (?=.*[a-z]), and one uppercase letter (?=.*[A-Z]), all within a string of at least 8 characters.
	hashedPassword, err := hashPassword(signUpReq.Password)
	if err != nil {
		return nil, err
	}

	userID, err := s.insertUserWithRetry(tx, signUpReq, hashedPassword)
	if err != nil {
		return nil, err
	}

	newUser, err := fetchNewUser(tx, userID)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec("DELETE FROM PasswordTokens WHERE Email = ? AND Verified = TRUE", newUser.Email)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error removing verified token: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return newUser, nil
}

// Login checks if a user exists with the given email and password.
func (s *AuthService) Login(email, password string) (*model.User, error) {
	user := &model.User{}
	query := `SELECT UserID, Username, Email, PasswordHash, Provider FROM Users WHERE Email = ?`
	err := s.DB.Get(user, query, email)
	if err != nil {
		return nil, err // User not found or db error
	}

	// Check if the user was registered through an external provider
	if user.Provider.Valid && user.Provider.String != "website" {
		// The user did not register through the website's traditional sign-up process
		return nil, fmt.Errorf("external provider login not supported here")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), []byte(password)) // heavy
	if err != nil {
		// Password does not match
		return nil, fmt.Errorf("invalid credentials")
	}

	return user, nil
}

func (s *AuthService) ResetPassword(token string, newPassword string) error {
	// Start a transaction
	tx, err := s.DB.Beginx()
	if err != nil {
		return err
	}

	// Ensure the transaction is rolled back if an error occurs
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var userID int
	// Use the transaction (tx) to perform the query
	err = tx.Get(&userID, "SELECT UserID FROM PasswordResetTokens WHERE Token = ? AND ExpiresAt > NOW()", token)
	if err != nil {
		return err // Token not found or expired
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Use the transaction (tx) to update the user's password
	_, err = tx.Exec("UPDATE Users SET PasswordHash = ? WHERE UserID = ?", string(hashedPassword), userID)
	if err != nil {
		return err
	}

	// Use the transaction (tx) to delete the reset token
	_, err = tx.Exec("DELETE FROM PasswordResetTokens WHERE Token = ?", token)
	if err != nil {
		return err
	}

	// Commit the transaction
	return tx.Commit()
}

func (s *AuthService) GeneratePasswordResetToken(email string) (string, error) {
	user := model.User{}
	err := s.DB.Get(&user, "SELECT UserID FROM Users WHERE Email = ?", email)
	if err != nil {
		return "", err // User not found or db error
	}

	token, err := s.TokenUtil.GenerateOpaqueToken(16)
	if err != nil {
		return "", err
	}

	_, err = s.DB.Exec(`
    INSERT INTO PasswordResetTokens (UserID, Token, ExpiresAt)
    VALUES (?, ?, ?)
    ON DUPLICATE KEY UPDATE Token = VALUES(Token), ExpiresAt = VALUES(ExpiresAt)`,
		user.UserID, token, time.Now().Add(24*time.Hour))
	if err != nil {
		return "", err
	}

	return token, nil
}

// VerifyNaverEmail can check naver email existence before sending
func (s *AuthService) VerifyNaverEmail(naverAddress string) (bool, error) {
	naverAddress = strings.Split(naverAddress, "@naver.com")[0]
	reqURL := fmt.Sprintf("%s=%s", s.Config.NaverEmailVerifyURL, naverAddress)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read the body: %v", err)
	}

	// Convert body bytes to a string
	bodyString := string(bodyBytes)

	// Check if the body is non-empty and ends with 'N'
	if len(bodyString) > 0 && bodyString[len(bodyString)-1] == 'N' {
		return true, nil
	}

	return false, nil
}

// private
func hashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

func generateUsername(signUpReq *dto.SignUpRequest) string {
	if signUpReq.Username != nil && *signUpReq.Username != "" {
		return *signUpReq.Username
	}
	emailParts := strings.Split(signUpReq.Email, "@")
	return emailParts[0]
}

func (s *AuthService) insertUserWithRetry(tx *sqlx.Tx, signUpReq *dto.SignUpRequest, hashedPassword string) (int64, error) {
	username := generateUsername(signUpReq)
	const maxRetries = 5
	for i := 0; i < maxRetries; i++ {
		res, err := tx.Exec(`INSERT INTO Users (Username, Email, PasswordHash, Provider, ProviderID, Role, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, 'user', NOW(), NOW())`,
			username, signUpReq.Email, hashedPassword, signUpReq.Provider, signUpReq.ProviderID)
		if err != nil {
			if strings.Contains(err.Error(), "Duplicate entry") && strings.Contains(err.Error(), "for key 'idx_users_username'") {
				username = fmt.Sprintf("%s-%s", username, s.TokenUtil.GenerateRandomString(5))
				continue
			}
			return 0, fmt.Errorf("error registering user: %w", err)
		}
		userID, _ := res.LastInsertId()
		return userID, nil
	}
	return 0, fmt.Errorf("failed to insert user after retries")
}
