package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Alfex4936/chulbong-kr/config"
	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/model"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"go.uber.org/fx"
	"golang.org/x/crypto/bcrypt"
)

var (
	nameReplacements = map[string]string{
		"chulbong-kr": "user",
		"k-pullup":    "user",
		"admin":       "user",
		"관리자":         "user",
		"k-풀업":        "user",
		"k풀업":         "user",
		"익명":          "user",
	}
)

const (
	verifyOpaqueTokenQuery = "SELECT UserID, ExpiresAt FROM OpaqueTokens WHERE OpaqueToken = ?"
	fetchTokenQuery        = `
    SELECT u.UserID, u.Role, ot.ExpiresAt
    FROM OpaqueTokens ot
    JOIN Users u ON ot.UserID = u.UserID
    WHERE ot.OpaqueToken = ?`
	profileQuery                  = "SELECT Username, Email FROM Users WHERE UserID = ?"
	deletePasswordTokenQuery      = "DELETE FROM PasswordTokens WHERE Email = ? AND Verified = TRUE"
	loginGetQuery                 = "SELECT UserID, Username, Email, PasswordHash, Provider FROM Users WHERE Email = ? AND Provider = 'website'"
	checkExpiredTokenQuery        = "SELECT UserID FROM PasswordResetTokens WHERE Token = ? AND ExpiresAt > NOW()"
	updatePasswordQuery           = "UPDATE Users SET PasswordHash = ? WHERE UserID = ?"
	deleteResetTokenQuery         = "DELETE FROM PasswordResetTokens WHERE Token = ?"
	getUserEmailQuery             = "SELECT UserID FROM Users WHERE Email = ? AND Provider = 'website'"
	insertPasswordResetTokenQuery = `
INSERT INTO PasswordResetTokens (UserID, Token, ExpiresAt)
VALUES (?, ?, ?)
ON DUPLICATE KEY UPDATE Token = VALUES(Token), ExpiresAt = VALUES(ExpiresAt)
	`
	insertUserQuery = "INSERT INTO Users (Username, Email, PasswordHash, Provider, ProviderID, Role, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, 'user', NOW(), NOW())"

	selectUserByEmailAndProviderQuery    = "SELECT * FROM Users WHERE Email = ? AND Provider = ?"
	selectCountByUsernameQuery           = "SELECT COUNT(*) FROM Users WHERE Username = ?"
	insertNewUserQuery                   = "INSERT INTO Users (Username, Email, Provider, ProviderID, Role) VALUES (?, ?, ?, ?, 'user')"
	updateUserProviderIDAndUsernameQuery = "UPDATE Users SET Username = ?, ProviderID = ? WHERE Email = ? AND Provider = ?"
	updateUserProviderIDQuery            = "UPDATE Users SET ProviderID = ? WHERE Email = ? AND Provider = ?"
	selectUserAfterUpdateQuery           = "SELECT * FROM Users WHERE Email = ? AND Provider = ?"
)

type UserDetails struct {
	ExpiresAt time.Time
	UserID    int
	Username  string
	Email     string
	Role      string
}

type AuthService struct {
	DB          *sqlx.DB
	Config      *config.AppConfig
	OAuthConfig *config.OAuthConfig
	TokenUtil   *util.TokenUtil
	HTTPClient  *http.Client
	ChatUtil    *util.ChatUtil

	fetchTokenStmt  *sqlx.Stmt
	profileStmt     *sqlx.Stmt
	verifyTokenStmt *sqlx.Stmt
}

func NewAuthService(db *sqlx.DB, config *config.AppConfig, oconfig *config.OAuthConfig, tokenUtil *util.TokenUtil, httpClient *http.Client, chatUtil *util.ChatUtil) *AuthService {
	// Prepare the statements
	fetchTokenStmt, _ := db.Preparex(fetchTokenQuery)
	profileStmt, _ := db.Preparex(profileQuery)
	verifyTokenStmt, _ := db.Preparex(verifyOpaqueTokenQuery)

	return &AuthService{
		DB:          db,
		Config:      config,
		OAuthConfig: oconfig,
		TokenUtil:   tokenUtil,
		HTTPClient:  httpClient,
		ChatUtil:    chatUtil,

		fetchTokenStmt:  fetchTokenStmt,
		profileStmt:     profileStmt,
		verifyTokenStmt: verifyTokenStmt,
	}
}

func RegisterAuthLifecycle(lifecycle fx.Lifecycle, service *AuthService) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return nil
		},
		OnStop: func(context.Context) error {
			service.fetchTokenStmt.Close()
			service.profileStmt.Close()
			service.verifyTokenStmt.Close()
			return nil
		},
	})
}

func (s *AuthService) VerifyOpaqueToken(token string) (int, time.Time, error) {
	var userID int
	var expiresAt time.Time
	err := s.verifyTokenStmt.QueryRow(token).Scan(&userID, &expiresAt)
	if err != nil {
		return 0, time.Time{}, err
	}
	return userID, expiresAt, nil
}

func (s *AuthService) FetchUserDetails(jwtCookie string, fetchProfile bool) (UserDetails, error) {
	details := UserDetails{}

	// Fetch user ID, role, and expiration based on the opaque token
	/// MYSQL
	/// access_type: const, query_cost: 1.00
	err := s.fetchTokenStmt.QueryRow(jwtCookie).Scan(&details.UserID, &details.Role, &details.ExpiresAt)
	if err != nil {
		return UserDetails{}, err
	}

	// Optionally fetch additional user profile information
	if fetchProfile {
		/// MYSQL
		/// access_type: const, query_cost: 1.00
		err = s.profileStmt.QueryRow(details.UserID).Scan(&details.Username, &details.Email)
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
	defer tx.Rollback() // rollback unless tx.Commit() is called

	// /(?=.*\d)(?=.*[a-z])(?=.*[A-Z]).{8,}/
	// at least one digit (?=.*\d), one lowercase letter (?=.*[a-z]), and one uppercase letter (?=.*[A-Z]), all within a string of at least 8 characters.
	hashedPassword, err := hashPassword(signUpReq.Password)
	if err != nil {
		return nil, err
	}

	// change admin, k-pullup, chulbong-kr to user
	normalizeUsername(signUpReq)

	userID, err := s.insertUserWithRetry(tx, signUpReq, hashedPassword)
	if err != nil {
		return nil, err
	}

	newUser, err := fetchNewUser(tx, userID)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(deletePasswordTokenQuery, newUser.Email)
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

	/// MYSQL
	/// access_type: const, query_cost: 1.00
	err := s.DB.Get(user, loginGetQuery, email)
	if err != nil {
		return nil, err // User not found or db error
	}

	// Check if the user was registered through an external provider
	if user.Provider.Valid && user.Provider.String != "website" {
		// The user did not register through the website's traditional sign-up process
		return nil, errors.New("external provider login not supported here")
	}

	// Use StringToBytes to avoid unnecessary allocation
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash.String), util.StringToBytes(password)) // optimized
	if err != nil {
		// Password does not match
		return nil, errors.New("invalid credentials")
	}

	return user, nil
}

// SaveOAuthUser saves or updates a user after OAuth2 authentication
func (s *AuthService) SaveOAuthUser(provider string, providerID string, email string, username string) (*model.User, error) {
	if email == "" {
		return nil, errors.New("email and username are required")
	}

	// Generate a random Korean nickname instead of using the OAuth username
	randomNickname := s.ChatUtil.GenerateUsernameForAuth()

	tx, err := s.DB.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() // rollback unless tx.Commit() is called

	user := &model.User{}
	err = tx.Get(user, selectUserByEmailAndProviderQuery, email, provider)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("error fetching user: %w", err)
	}

	if err == sql.ErrNoRows {
		// New user
		const maxRetries = 5
		for i := 0; i < maxRetries; i++ {
			uniqueUsername := randomNickname
			if i > 0 {
				// If collision, generate a new random nickname
				uniqueUsername = s.ChatUtil.GenerateUsernameForAuth()
			}

			_, err = tx.Exec(insertNewUserQuery, uniqueUsername, email, provider, providerID)
			if err != nil {
				if me, ok := err.(*mysql.MySQLError); ok && me.Number == 1062 {
					// Duplicate entry error code
					continue
				}
				return nil, fmt.Errorf("error inserting user: %w", err)
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to insert user after retries: %w", err)
		}
	} else {
		// Existing user - don't change username unless it's one of the restricted names
		currentUsername := user.Username
		shouldUpdateUsername := false

		// Check if current username needs to be replaced
		for restrictedName := range nameReplacements {
			if strings.Contains(strings.ToLower(currentUsername), strings.ToLower(restrictedName)) {
				shouldUpdateUsername = true
				break
			}
		}

		if shouldUpdateUsername {
			// Update username and ProviderID with a new random nickname
			const maxRetries = 5
			for i := 0; i < maxRetries; i++ {
				uniqueUsername := randomNickname
				if i > 0 {
					uniqueUsername = s.ChatUtil.GenerateUsernameForAuth()
				}

				_, err = tx.Exec(updateUserProviderIDAndUsernameQuery, uniqueUsername, providerID, email, provider)
				if err != nil {
					if me, ok := err.(*mysql.MySQLError); ok && me.Number == 1062 {
						// Duplicate entry error code
						continue
					}
					return nil, fmt.Errorf("error updating user: %w", err)
				}
				break
			}
			if err != nil {
				return nil, fmt.Errorf("failed to update user after retries: %w", err)
			}
		} else {
			// Just update the ProviderID
			_, err = tx.Exec(updateUserProviderIDQuery, providerID, email, provider)
			if err != nil {
				return nil, fmt.Errorf("error updating provider ID: %w", err)
			}
		}
	}

	err = tx.Get(user, selectUserAfterUpdateQuery, email, provider)
	if err != nil {
		return nil, fmt.Errorf("error fetching updated user: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("error committing transaction: %w", err)
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
	err = tx.Get(&userID, checkExpiredTokenQuery, token)
	if err != nil {
		return err // Token not found or expired
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(util.StringToBytes(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Use the transaction (tx) to update the user's password
	_, err = tx.Exec(updatePasswordQuery, string(hashedPassword), userID)
	if err != nil {
		return err
	}

	// Use the transaction (tx) to delete the reset token
	_, err = tx.Exec(deleteResetTokenQuery, token)
	if err != nil {
		return err
	}

	// Commit the transaction
	return tx.Commit()
}

func (s *AuthService) GeneratePasswordResetToken(email string) (string, error) {
	user := model.User{}

	err := s.DB.Get(&user, getUserEmailQuery, email)
	if err != nil {
		return "", err // User not found or db error
	}

	token, err := s.TokenUtil.GenerateOpaqueToken(s.TokenUtil.Config.TokenLength)
	if err != nil {
		return "", err
	}

	_, err = s.DB.Exec(insertPasswordResetTokenQuery,
		user.UserID, token, time.Now().Add(24*time.Hour))
	if err != nil {
		return "", err
	}

	return token, nil
}

// VerifyNaverEmail can check naver email existence before sending
func (s *AuthService) VerifyNaverEmail(naverAddress string) (bool, error) {
	const suffix = "@naver.com"
	if !strings.HasSuffix(naverAddress, suffix) {
		return false, errors.New("invalid naver username")
	}

	// Extract the username part
	username := naverAddress[:len(naverAddress)-len(suffix)]
	if len(username) == 0 {
		return false, errors.New("invalid naver username")
	}

	// Build the request URL without fmt.Sprintf
	reqURL := s.Config.NaverEmailVerifyURL + "=" + username

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return false, err
	}

	// Use Set instead of Add for known headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, errors.New("unexpected status code: " + strconv.Itoa(resp.StatusCode))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		// Better to return err than crash the entire server with log.Fatal
		return false, err
	}

	// Check the last byte directly without converting to string
	if len(bodyBytes) > 0 && bodyBytes[len(bodyBytes)-1] == 'N' {
		return true, nil
	}

	return false, nil
}

// private
func hashPassword(password string) (string, error) {
	// Use StringToBytes to convert the password string to a byte slice without extra allocation
	hashedBytes, err := bcrypt.GenerateFromPassword(util.StringToBytes(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	// Convert the resulting hashed byte slice back to a string
	return string(hashedBytes), nil
}

func generateUsername(signUpReq *dto.SignUpRequest) string {
	if signUpReq.Username != nil && *signUpReq.Username != "" {
		return *signUpReq.Username
	}
	emailParts := strings.Split(signUpReq.Email, "@")
	return SegmentConsonants(emailParts[0])
}

func (s *AuthService) insertUserWithRetry(tx *sqlx.Tx, signUpReq *dto.SignUpRequest, hashedPassword string) (int64, error) {
	username := generateUsername(signUpReq)
	const maxRetries = 5
	for i := 0; i < maxRetries; i++ {
		res, err := tx.Exec(insertUserQuery,
			username, signUpReq.Email, hashedPassword, signUpReq.Provider, signUpReq.ProviderID)
		if err != nil {
			if me, ok := err.(*mysql.MySQLError); ok && me.Number == 1062 { // Duplicate entry
				username = username + "-" + s.TokenUtil.GenerateRandomString(5)
				continue
			}
			return 0, fmt.Errorf("error registering user: %w", err)
		}
		userID, _ := res.LastInsertId()
		return userID, nil
	}
	return 0, errors.New("failed to insert user after retries")
}

func normalizeUsername(signUpReq *dto.SignUpRequest) {
	if signUpReq.Username == nil {
		return
	}

	// Convert to lowercase once
	originalUsername := *signUpReq.Username
	usernameLower := strings.ToLower(originalUsername)

	for pattern, replacement := range nameReplacements {
		patternLower := strings.ToLower(pattern)
		if strings.Contains(usernameLower, patternLower) {
			// Use strings.Builder to minimize allocations
			var sb strings.Builder
			sb.Grow(len(originalUsername))
			idx := strings.Index(usernameLower, patternLower)
			sb.WriteString(originalUsername[:idx])
			sb.WriteString(replacement)
			sb.WriteString(originalUsername[idx+len(pattern):])
			*signUpReq.Username = sb.String()
			break
		}
	}
}
