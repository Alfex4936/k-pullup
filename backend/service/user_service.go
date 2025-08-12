package service

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/model"
	"github.com/Alfex4936/chulbong-kr/util"

	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

const (
	findUserExistenceAndProviderQuery = "SELECT 1 as exist, COALESCE(Provider, '') as provider FROM Users WHERE UserID = ?"
	getUserById                       = "SELECT UserID, Username, Email, Provider FROM Users WHERE UserID = ?"
	checkWebsiteUserById              = "SELECT EXISTS(SELECT 1 FROM Users WHERE UserID = ? AND Provider = 'website')"
	getManyDetailsUserByEmail         = "SELECT UserID, Username, Email, PasswordHash, Provider, ProviderID, CreatedAt, UpdatedAt FROM Users WHERE Email = ? AND Provider = 'website'"
	getUserByUsernameQuery            = "SELECT UserID FROM Users WHERE Username = ?"
	userGetEmailQuery                 = "SELECT UserID FROM Users WHERE Email = ? AND Provider = 'website'" // Renamed to avoid conflict with auth service
	getAllReportsByUserQuery          = `
SELECT r.ReportID, r.MarkerID, r.UserID, ST_X(r.Location) AS Latitude, ST_Y(r.Location) AS Longitude,
ST_X(r.NewLocation) AS NewLatitude, ST_Y(r.NewLocation) AS NewLongitude,
r.Description, r.CreatedAt, r.Status, r.DoesExist, m.Address, p.PhotoURL
FROM Reports r
LEFT JOIN ReportPhotos p ON r.ReportID = p.ReportID
LEFT JOIN Markers m ON r.MarkerID = m.MarkerID
WHERE r.UserID = ?
ORDER BY r.CreatedAt DESC`
	getAllReportsForMyMarkersQuery = `
SELECT 
	r.ReportID,
	r.MarkerID,
	r.UserID,
	ST_X(r.Location) as Latitude,
	ST_Y(r.Location) as Longitude,
	ST_X(r.NewLocation) as NewLatitude,
	ST_Y(r.NewLocation) as NewLongitude,
	r.Description,
	r.CreatedAt,
	r.Status,
	r.DoesExist,
	m.Address,
	rp.PhotoURL
FROM 
	Reports r
LEFT JOIN 
	ReportPhotos rp ON r.ReportID = rp.ReportID
LEFT JOIN
	Markers m ON r.MarkerID = m.MarkerID
WHERE 
	EXISTS (
		SELECT 1
		FROM Markers
		WHERE Markers.MarkerID = r.MarkerID
		AND Markers.UserID = ?
	)
ORDER BY
	r.MarkerID, r.CreatedAt DESC;`
	getAllFavQuery = `
SELECT Markers.MarkerID, ST_X(Markers.Location) AS Latitude, ST_Y(Markers.Location) AS Longitude, Markers.Description, Markers.Address
FROM Favorites
JOIN Markers ON Favorites.MarkerID = Markers.MarkerID
WHERE Favorites.UserID = ?
ORDER BY Markers.CreatedAt DESC` // Order by CreatedAt in descending order

	getPhotoByUserIdQuery = "SELECT PhotoURL FROM Photos WHERE MarkerID IN (SELECT MarkerID FROM Markers WHERE UserID = ?)"

	deleteOpaqueTokensQuery   = "DELETE FROM OpaqueTokens WHERE UserID = ?"
	deleteCommentsQuery       = "DELETE FROM Comments WHERE UserID = ?"
	deleteMarkerDislikesQuery = "DELETE FROM MarkerDislikes WHERE UserID = ?"
	deletePhotosQuery         = "DELETE FROM Photos WHERE MarkerID IN (SELECT MarkerID FROM Markers WHERE UserID = ?)"
	updateMarkersQuery        = "UPDATE Markers SET UserID = NULL WHERE UserID = ?" // Set UserID to NULL for Markers instead of deleting
	deleteUserQuery           = "DELETE FROM Users WHERE UserID = ?"
	getNewUserQuery           = "SELECT UserID, Username, Email, Provider, ProviderID, Role, CreatedAt, UpdatedAt FROM Users WHERE UserID = ?"

	// Count query to get how many reports a user makes for any marker by UserID
	countQueryHowManyReportsAUserMakesQuery = "SELECT COUNT(*) As ReportCount FROM Reports WHERE UserID = ?"
	// Count query to get how many markers a user makes by UserID
	countQueryHowManyMarkersAUserMakesQuery = "SELECT COUNT(*) AS MarkerCount FROM Markers WHERE UserID = ?"

	sumContributionScoresForAUserQuery = "SELECT SUM(Points) AS TotalPoints FROM UserContributions WHERE UserID = ?"

	getUsernameByIdQuery = "SELECT Username FROM Users WHERE UserID = ?"
)

var ContributionLevelNames = []struct {
	MinPoints int
	MaxPoints int
	LevelName string
}{
	{0, 99, "초보 운동자"},
	{100, 299, "운동 길잡이"},
	{300, 699, "철봉 탐험가"},
	{700, 1499, "스트릿 워리어"},
	{1500, 2999, "피트니스 전도사"},
	{3000, 4999, "철봉 레인저"},
	{5000, 7999, "철봉 매버릭"},
	{8000, 11999, "거장"},
	{12000, 19999, "명인"},
	{20000, int(^uint(0) >> 1), "철봉 신화"}, // Max int value for the upper bound
}

type UserService struct {
	DB          *sqlx.DB
	S3Service   *S3Service
	BadWordUtil *util.BadWordUtil
}

func NewUserService(db *sqlx.DB, s3Service *S3Service, badWordUtil *util.BadWordUtil) *UserService {
	return &UserService{
		DB:          db,
		S3Service:   s3Service,
		BadWordUtil: badWordUtil,
	}
}

// GetUserById retrieves a user by their email address
func (s *UserService) GetUserById(userID int) (*dto.UserResponse, error) {
	var user dto.UserResponse

	// Execute the query
	err := s.DB.Get(&user, getUserById, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no user found with userID %d", userID)
		}
		return nil, fmt.Errorf("error fetching user by userID: %w", err)
	}

	return &user, nil
}

// GetUserByEmail retrieves a user by their email address
func (s *UserService) GetUserByEmail(email string) (*model.User, error) {
	var user model.User

	// Execute the query
	err := s.DB.Get(&user, getManyDetailsUserByEmail, email)
	if err != nil {
		return nil, err
		// if err == sql.ErrNoRows {
		// 	// No user found with the provided email
		// 	return nil, fmt.Errorf("no user found with email %s", email)
		// }
		// // An error occurred during the query execution
		// return nil, fmt.Errorf("error fetching user by email: %w", err)
	}

	return &user, nil
}

func (s *UserService) UpdateUserProfile(userID int, updateReq *dto.UpdateUserRequest) (*dto.UserResponse, error) {
	tx, err := s.DB.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check if the user exists (allow both website and oauth users)
	var user struct {
		Exists   int    `db:"exist"`
		Provider string `db:"provider"`
	}
	err = tx.Get(&user, findUserExistenceAndProviderQuery, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no user found with userID %d", userID)
		}
		return nil, fmt.Errorf("error checking user existence: %w", err)
	}

	if updateReq.Username != nil {
		normalizedUsername := strings.TrimSpace(SegmentConsonants(*updateReq.Username))

		// Replace restricted names with their replacements
		cleanedUsername := normalizedUsername
		lowerUsername := strings.ToLower(cleanedUsername)
		for restrictedName, replacement := range nameReplacements {
			if strings.Contains(lowerUsername, strings.ToLower(restrictedName)) {
				cleanedUsername = strings.ReplaceAll(cleanedUsername, restrictedName, replacement)
				lowerUsername = strings.ToLower(cleanedUsername) // Update for next iteration
			}
		}

		// Check and replace bad words
		cleanedUsername, err := s.BadWordUtil.ReplaceBadWords(cleanedUsername)
		if err != nil {
			return nil, fmt.Errorf("error checking username for bad words: %w", err)
		}

		// Check if username already exists
		var existingID int
		err = tx.Get(&existingID, getUserByUsernameQuery, cleanedUsername)
		if err == nil || err != sql.ErrNoRows {
			return nil, fmt.Errorf("username %s is already in use", cleanedUsername)
		}
		*updateReq.Username = cleanedUsername
	}

	// Only allow email and password updates for website users
	if updateReq.Email != nil {
		if user.Provider != "website" {
			return nil, fmt.Errorf("email updates are only allowed for website users")
		}
		var existingID int
		err = tx.Get(&existingID, userGetEmailQuery, *updateReq.Email)
		if err == nil || err != sql.ErrNoRows {
			return nil, fmt.Errorf("email %s is already in use", *updateReq.Email)
		}
	}

	if updateReq.Password != nil {
		if user.Provider != "website" {
			return nil, fmt.Errorf("password updates are only allowed for website users")
		}
	}

	var setParts []string
	var args []any

	if updateReq.Username != nil {
		setParts = append(setParts, "Username = ?")
		args = append(args, *updateReq.Username)
	}

	if updateReq.Email != nil && user.Provider == "website" {
		setParts = append(setParts, "Email = ?")
		args = append(args, *updateReq.Email)
	}

	if updateReq.Password != nil && user.Provider == "website" {
		hashedPassword, hashErr := bcrypt.GenerateFromPassword(util.StringToBytes(*updateReq.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			return nil, hashErr
		}
		setParts = append(setParts, "PasswordHash = ?")
		args = append(args, util.BytesToString(hashedPassword)) // Use BytesToString to avoid extra allocation
	}

	if len(setParts) > 0 {
		args = append(args, userID)
		query := fmt.Sprintf("UPDATE Users SET %s WHERE UserID = ?", strings.Join(setParts, ", "))
		_, err = tx.Exec(query, args...)
		if err != nil {
			return nil, fmt.Errorf("error updating user: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("error committing update: %w", err)
	}

	// Fetch the updated user details
	updatedUser, err := s.GetUserById(userID)
	if err != nil {
		return nil, fmt.Errorf("error fetching updated user: %w", err)
	}

	return updatedUser, nil
}

// GetAllReportsByUser retrieves all reports submitted by a specific user from the database.
func (s *UserService) GetAllReportsByUser(userID int) ([]dto.MarkerReportResponse, error) {
	rows, err := s.DB.Queryx(getAllReportsByUserQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("error querying reports by user: %w", err)
	}
	defer rows.Close()

	reportMap := make(map[int]*dto.MarkerReportResponse)
	for rows.Next() {
		var (
			r   dto.MarkerReportResponse
			url sql.NullString // Use sql.NullString to handle possible NULL values from PhotoURL
		)
		if err := rows.Scan(&r.ReportID, &r.MarkerID, &r.UserID, &r.Latitude, &r.Longitude,
			&r.NewLatitude, &r.NewLongitude, &r.Description, &r.CreatedAt, &r.Status, &r.DoesExist, &r.Address, &url); err != nil {
			return nil, err
		}
		if report, exists := reportMap[r.ReportID]; exists {
			// Append only if url is valid to avoid appending empty strings for reports without photos
			if url.Valid {
				report.PhotoURLs = append(report.PhotoURLs, url.String)
			}
		} else {
			r.PhotoURLs = make([]string, 0)
			if url.Valid {
				r.PhotoURLs = append(r.PhotoURLs, url.String)
			}
			reportMap[r.ReportID] = &r
		}
	}

	// Convert map to slice
	reports := make([]dto.MarkerReportResponse, 0, len(reportMap))
	for _, report := range reportMap {
		reports = append(reports, *report)
	}

	return reports, nil
}

// GetAllReportsForMyMarkersByUser retrieves all reports for markers owned by a specific user
func (s *UserService) GetAllReportsForMyMarkersByUser(userID int) (dto.GroupedReportsResponse, error) {
	rows, err := s.DB.Queryx(getAllReportsForMyMarkersQuery, userID)
	if err != nil {
		return dto.GroupedReportsResponse{}, fmt.Errorf("error querying reports by user: %w", err)
	}
	defer rows.Close()

	groupedReports := make(map[int][]dto.ReportWithPhotos, 0)
	reportMap := make(map[int]*dto.MarkerReportResponse)
	// Map to track if address is already added for a marker
	addressAdded := make(map[int]string)

	for rows.Next() {
		var r dto.MarkerReportResponse
		var url sql.NullString
		if err := rows.Scan(&r.ReportID, &r.MarkerID, &r.UserID, &r.Latitude, &r.Longitude,
			&r.NewLatitude, &r.NewLongitude, &r.Description, &r.CreatedAt, &r.Status, &r.DoesExist, &r.Address, &url); err != nil {
			return dto.GroupedReportsResponse{}, err
		}

		report, exists := reportMap[r.ReportID]
		if exists {
			if url.Valid {
				report.PhotoURLs = append(report.PhotoURLs, url.String)
			}
		} else {
			r.PhotoURLs = make([]string, 0)
			if url.Valid {
				r.PhotoURLs = append(r.PhotoURLs, url.String)
			}
			reportMap[r.ReportID] = &r

			// Add address only if it's the first report for the marker
			reportWithPhotos := dto.ReportWithPhotos{
				ReportID:     r.ReportID,
				Description:  r.Description,
				Status:       r.Status,
				CreatedAt:    r.CreatedAt,
				Photos:       r.PhotoURLs,
				NewLatitude:  r.NewLatitude,
				NewLongitude: r.NewLongitude,
			}
			if _, added := addressAdded[r.MarkerID]; !added {
				// reportWithPhotos.Address = r.Address
				addressAdded[r.MarkerID] = r.Address
			}
			groupedReports[r.MarkerID] = append(groupedReports[r.MarkerID], reportWithPhotos)
		}
	}

	// Sort each group by status and CreatedAt
	for _, reports := range groupedReports {
		sort.SliceStable(reports, func(i, j int) bool {
			if reports[i].Status == "PENDING" && reports[j].Status != "PENDING" {
				return true
			}
			if reports[i].Status != "PENDING" && reports[j].Status == "PENDING" {
				return false
			}
			return reports[i].CreatedAt.After(reports[j].CreatedAt)
		})
	}

	// Build a slice of markers with reports
	markersWithReports := make([]dto.MarkerWithReports, 0, len(groupedReports))
	for markerID, reports := range groupedReports {
		markersWithReports = append(markersWithReports, dto.MarkerWithReports{
			MarkerID: markerID,
			Address:  addressAdded[markerID],
			Reports:  reports,
		})
	}

	// Sort markers by the date of their latest report
	sort.SliceStable(markersWithReports, func(i, j int) bool {
		return markersWithReports[i].Reports[0].CreatedAt.After(markersWithReports[j].Reports[0].CreatedAt)
	})

	response := dto.GroupedReportsResponse{
		TotalReports: len(reportMap),
		Markers:      markersWithReports,
	}

	return response, nil
}

func (s *UserService) GetAllFavorites(userID int) ([]dto.MarkerSimpleWithDescription, error) {
	favorites := make([]dto.MarkerSimpleWithDescription, 0)

	err := s.DB.Select(&favorites, getAllFavQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("error fetching favorites: %w", err)
	}

	return favorites, nil
}

// DeleteUserWithRelatedData
func (s *UserService) DeleteUserWithRelatedData(ctx context.Context, userID int) error {
	// Begin a transaction
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	// Fetch Photo URLs associated with the user
	var photoURLs []string
	if err := tx.SelectContext(ctx, &photoURLs, getPhotoByUserIdQuery, userID); err != nil {
		tx.Rollback()
		return fmt.Errorf("fetching photo URLs: %w", err)
	}

	// Delete each photo from S3
	for _, url := range photoURLs {
		if err := s.S3Service.DeleteDataFromS3(url); err != nil {
			tx.Rollback()
			return fmt.Errorf("deleting photo from S3: %w", err)
		}
	}

	// Note: Order matters due to foreign key constraints
	var deletionQueries = []string{
		deleteOpaqueTokensQuery,
		deleteCommentsQuery,
		deleteMarkerDislikesQuery,
		deletePhotosQuery,
		updateMarkersQuery,
		deleteUserQuery,
	}

	// Execute each deletion query within the transaction
	for _, query := range deletionQueries {
		if _, err := tx.ExecContext(ctx, query, userID); err != nil {
			tx.Rollback() // Attempt to rollback, but don't override the original error
			return fmt.Errorf("executing deletion query (%s): %w", query, err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// GetUserFromContext extracts and validates the user information from the Fiber context.
func (s *UserService) GetUserFromContext(c *fiber.Ctx) (*dto.UserData, error) {
	userID, ok := c.Locals("userID").(int)
	if !ok {
		return nil, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "User ID is required",
		})
	}

	username, ok := c.Locals("username").(string)
	if !ok {
		return nil, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Username not found"})
	}

	return &dto.UserData{
		UserID:   userID,
		Username: username,
	}, nil
}

func (s *UserService) GetUserStatistics(userID int) (int, int, error) {
	var reportCount, markerCount int

	// Execute the query for reports count
	err := s.DB.Get(&reportCount, countQueryHowManyReportsAUserMakesQuery, userID)
	if err != nil {
		return 0, 0, fmt.Errorf("error fetching report count for userID %d: %w", userID, err)
	}

	// Execute the query for markers count
	err = s.DB.Get(&markerCount, countQueryHowManyMarkersAUserMakesQuery, userID)
	if err != nil {
		return 0, 0, fmt.Errorf("error fetching marker count for userID %d: %w", userID, err)
	}

	return reportCount, markerCount, nil
}

// GetUserContributionScores returns the total score and the corresponding level name
func (s *UserService) GetUserContributionScores(userID int) (int, string, error) {
	var contributions int

	// Execute the query for total contribution scores
	err := s.DB.Get(&contributions, sumContributionScoresForAUserQuery, userID)
	if err != nil {
		return 0, "", fmt.Errorf("error fetching contribution scores for userID %d: %w", userID, err)
	}

	// Determine the level name based on the contribution score
	levelName := getLevelName(contributions)

	return contributions, levelName, nil
}

func fetchNewUser(tx *sqlx.Tx, userID int64) (*model.User, error) {
	var newUser model.User
	err := tx.QueryRowx(getNewUserQuery, userID).StructScan(&newUser)
	if err != nil {
		return nil, fmt.Errorf("error fetching newly created user: %w", err)
	}
	return &newUser, nil
}

// getLevelName determines the level name based on the points
func getLevelName(points int) string {
	for _, level := range ContributionLevelNames {
		if points >= level.MinPoints && points <= level.MaxPoints {
			return level.LevelName
		}
	}
	return "병아리" // In case something goes wrong
}
