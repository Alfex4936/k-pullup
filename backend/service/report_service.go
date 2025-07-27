package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"sync"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

const (
	getAllReportsQuery = `
SELECT r.ReportID, r.MarkerID, r.UserID, ST_X(r.Location) AS Latitude, ST_Y(r.Location) AS Longitude,
ST_X(r.NewLocation) AS NewLatitude, ST_Y(r.NewLocation) AS NewLongitude,
r.Description, r.CreatedAt, r.Status, r.DoesExist, COALESCE(p.PhotoURL, '')
FROM Reports r
LEFT JOIN ReportPhotos p ON r.ReportID = p.ReportID
ORDER BY r.CreatedAt DESC`

	getAllReportsByQuery = `
SELECT r.ReportID, r.MarkerID, r.UserID, ST_X(r.Location) AS Latitude, ST_Y(r.Location) AS Longitude,
ST_X(r.NewLocation) AS NewLatitude, ST_Y(r.NewLocation) AS NewLongitude,
r.Description, r.CreatedAt, r.Status, m.Address, COALESCE(p.PhotoURL, '') AS PhotoURL
FROM Reports r
LEFT JOIN ReportPhotos p ON r.ReportID = p.ReportID
LEFT JOIN Markers m ON r.MarkerID = m.MarkerID
WHERE r.MarkerID = ?
ORDER BY r.CreatedAt DESC`

	insertReportQuery      = "INSERT INTO Reports (MarkerID, UserID, Location, NewLocation, Description, DoesExist) VALUES (?, ?, ST_PointFromText(?, 4326), ST_PointFromText(?, 4326), ?, ?)"
	insertReportPhotoQuery = "INSERT INTO ReportPhotos (ReportID, PhotoURL) VALUES (?, ?)"

	// Use a derived table to avoid Error 1093
	// SQL query tries to update a table (Reports) and simultaneously select from the same table within a subquery.
	approveReportQuery = `
UPDATE Reports
SET Status = 'APPROVED'
WHERE ReportID = ?
AND (
	MarkerID IN (
		SELECT MarkerID
		FROM (
			SELECT MarkerID
			FROM Markers
			WHERE UserID = ? AND MarkerID IN (
				SELECT MarkerID
				FROM Reports
				WHERE ReportID = ?
			)
		) AS subquery1
	)
	OR EXISTS (
		SELECT 1
		FROM (
			SELECT UserID
			FROM Users
			WHERE UserID = ? AND Role = 'admin'
		) AS subquery2
	)
)`

	denyReportQuery = `
UPDATE Reports
SET Status = 'DENIED'
WHERE ReportID = ?
AND (
	MarkerID IN (
		SELECT MarkerID
		FROM (
			SELECT MarkerID
			FROM Markers
			WHERE UserID = ? AND MarkerID IN (
				SELECT MarkerID
				FROM Reports
				WHERE ReportID = ?
			)
		) AS subquery1
	)
	OR EXISTS (
		SELECT 1
		FROM (
			SELECT UserID
			FROM Users
			WHERE UserID = ? AND Role = 'admin'
		) AS subquery2
	)
)`

	// Update marker details based on the approved report
	// CASE update statement to conditionally update the description only if the Reports.Description is not an empty string
	updateMarkeryReportQuery = `
UPDATE Markers 
JOIN Reports ON Markers.MarkerID = Reports.MarkerID
SET Markers.Location = COALESCE(Reports.NewLocation, Markers.Location),
	Markers.Description = CASE WHEN Reports.Description != '' THEN COALESCE(Reports.Description, Markers.Description) ELSE Markers.Description END,
	Markers.UpdatedAt = CURRENT_TIMESTAMP
WHERE Reports.ReportID = ? AND Reports.Status = 'APPROVED'	`

	updateReportPhotoQuery = `
INSERT INTO Photos (MarkerID, PhotoURL, ThumbnailURL, Blurhash, UploadedAt)
SELECT r.MarkerID, rp.PhotoURL, rp.ThumbnailURL, rp.Blurhash, rp.UploadedAt
FROM ReportPhotos rp
JOIN Reports r ON rp.ReportID = r.ReportID
WHERE r.ReportID = ? AND r.Status = 'APPROVED'
`
	deleteExcessPhotosQuery = `
WITH OrderedPhotos AS (
	SELECT PhotoID, ROW_NUMBER() OVER (PARTITION BY MarkerID ORDER BY UploadedAt ASC) AS RowNum
	FROM Photos
	WHERE MarkerID = (SELECT MarkerID FROM Reports WHERE ReportID = ?)
)
DELETE FROM Photos
WHERE PhotoID IN (SELECT PhotoID FROM OrderedPhotos WHERE RowNum > 5)`
	deleteReportPhotosQuery = "DELETE FROM ReportPhotos WHERE ReportID = ?"

	checkAuthQuery = `
SELECT EXISTS(
	SELECT 1 FROM Reports
	JOIN Markers ON Reports.MarkerID = Markers.MarkerID
	LEFT JOIN Users ON Users.UserID = Reports.UserID
	WHERE Reports.ReportID = ? AND (Reports.UserID = ? OR Markers.UserID = ? OR Users.Role = 'admin')
)`
	selectPhotosQuery = "SELECT PhotoURL FROM ReportPhotos WHERE ReportID = ?"

	deleteReportQuery = "DELETE FROM Reports WHERE ReportID = ?"

	getReportByIdQuery = "SELECT MarkerID FROM Reports WHERE ReportID = ?"

	updateDbLocationQuery        = "SELECT ST_X(Location) as Latitude, ST_Y(Location) as Longitude FROM Markers WHERE MarkerID = ?"
	updateMarkerAddressByIdQuery = "UPDATE Markers SET Address = ? WHERE MarkerID = ?"

	getPendingReportsQuery = `
SELECT r.ReportID, r.MarkerID, r.UserID, ST_X(r.Location) AS Latitude, ST_Y(r.Location) AS Longitude,
ST_X(r.NewLocation) AS NewLatitude, ST_Y(r.NewLocation) AS NewLongitude,
r.Description, r.CreatedAt, r.Status, r.DoesExist, COALESCE(p.PhotoURL, '')
FROM Reports r
LEFT JOIN ReportPhotos p ON r.ReportID = p.ReportID
WHERE r.Status = 'PENDING'
ORDER BY r.CreatedAt DESC`

	getReportByMarkerUserIdsQuery = "SELECT MarkerID, UserID FROM Reports WHERE ReportID = ?"
)

type ReportService struct {
	DB              *sqlx.DB
	S3Service       *S3Service
	LocationService *MarkerLocationService
	CacheService    *MarkerCacheService
	Logger          *zap.Logger
}

func NewReportService(db *sqlx.DB, s3Service *S3Service,
	location *MarkerLocationService,
	cache *MarkerCacheService,
	logger *zap.Logger) *ReportService {
	return &ReportService{
		DB:              db,
		S3Service:       s3Service,
		LocationService: location,
		CacheService:    cache,
		Logger:          logger,
	}
}

// GetAllReports retrieves reports for all markers from the database.
func (s *ReportService) GetAllReports() ([]dto.MarkerReportResponse, error) {
	rows, err := s.DB.Queryx(getAllReportsQuery)
	if err != nil {
		return nil, fmt.Errorf("error querying reports: %w", err)
	}
	defer rows.Close()

	reportMap := make(map[int]*dto.MarkerReportResponse)
	for rows.Next() {
		var (
			r   dto.MarkerReportResponse
			url string
		)
		if err := rows.Scan(&r.ReportID, &r.MarkerID, &r.UserID, &r.Latitude, &r.Longitude,
			&r.NewLatitude, &r.NewLongitude, &r.Description, &r.CreatedAt, &r.Status, &r.DoesExist, &url); err != nil {
			return nil, err
		}
		// Check if the URL is not empty before appending
		if url != "" {
			if report, exists := reportMap[r.ReportID]; exists {
				report.PhotoURLs = append(report.PhotoURLs, url)
			} else {
				r.PhotoURLs = []string{url}
				reportMap[r.ReportID] = &r
			}
		} else {
			// Ensure that PhotoURLs is initialized even if there are no photos
			if _, exists := reportMap[r.ReportID]; !exists {
				r.PhotoURLs = []string{} // Initialize with an empty slice
				reportMap[r.ReportID] = &r
			}
		}
	}

	// Convert map to slice
	reports := make([]dto.MarkerReportResponse, 0, len(reportMap))
	for _, report := range reportMap {
		reports = append(reports, *report)
	}

	return reports, nil
}

func (s *ReportService) GetAllReportsBy(markerID int) ([]dto.MarkerReportResponse, error) {
	rows, err := s.DB.Queryx(getAllReportsByQuery, markerID)
	if err != nil {
		return nil, fmt.Errorf("error querying reports by marker ID: %w", err)
	}
	defer rows.Close()

	reportMap := make(map[int]*dto.MarkerReportResponse)
	for rows.Next() {
		var (
			r   dto.MarkerReportResponse
			url string
		)
		if err := rows.Scan(&r.ReportID, &r.MarkerID, &r.UserID, &r.Latitude, &r.Longitude,
			&r.NewLatitude, &r.NewLongitude, &r.Description, &r.CreatedAt, &r.Status, &r.Address, &url); err != nil {
			return nil, err
		}
		// Check if the URL is not empty before appending
		if url != "" {
			if report, exists := reportMap[r.ReportID]; exists {
				report.PhotoURLs = append(report.PhotoURLs, url)
			} else {
				r.PhotoURLs = []string{url}
				reportMap[r.ReportID] = &r
			}
		} else {
			// Ensure that PhotoURLs is initialized even if there are no photos
			if _, exists := reportMap[r.ReportID]; !exists {
				r.PhotoURLs = []string{} // Initialize with an empty slice
				reportMap[r.ReportID] = &r
			}
		}
	}

	// Convert map to slice
	reports := make([]dto.MarkerReportResponse, 0, len(reportMap))
	for _, report := range reportMap {
		reports = append(reports, *report)
	}

	return reports, nil
}

// CreateReport handles the logic for creating a report and uploading photos related to that report.
func (s *ReportService) CreateReport(report *dto.MarkerReportRequest, form *multipart.Form) error {
	// Process file uploads from the multipart form
	files := form.File["photos"]
	if len(files) == 0 {
		return ErrNoPhotos
	}

	// Begin a transaction for database operations
	tx, err := s.DB.Beginx()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBeginTransaction, err)
	}
	defer tx.Rollback()

	// Insert the main report record
	point := formatPoint(report.Latitude, report.Longitude)
	newPoint := formatPoint(report.NewLatitude, report.NewLongitude)
	res, err := tx.Exec(insertReportQuery, report.MarkerID, report.UserID, point, newPoint, report.Description, report.DoesExist)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInsertReport, err)
	}

	// Get the last inserted ID for the report
	reportID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrLastInsertID, err)
	}

	folder := fmt.Sprintf("reports/%d", reportID)

	// Create a cancellable context for the worker tasks.
	taskCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channel to collect errors
	errorChan := make(chan error, 1)

	var wg sync.WaitGroup
	wg.Add(len(files))

	// Process files concurrently
	for _, fileHeader := range files {
		fileHeader := fileHeader // capture loop variable

		go func() {
			defer wg.Done()

			// Upload the file to S3 with thumbnail
			fileURL, thumbnailURL, err := s.S3Service.UploadFileToS3WithContext(taskCtx, folder, fileHeader, true)
			if err != nil {
				select {
				case errorChan <- fmt.Errorf("S3 upload failed: %w", err):
				default:
				}
				return
			}

			// Generate blurhash
			file, _ := fileHeader.Open()
			defer file.Close()

			buf := new(bytes.Buffer)
			_, _ = io.Copy(buf, file)
			rawBytes := buf.Bytes()
			img, _, err := image.Decode(bytes.NewReader(rawBytes))
			if err != nil {
				select {
				case errorChan <- fmt.Errorf("image decoding failed: %w", err):
				default:
				}
				return
			}
			blurhashString := util.EncodeBlurHashImage(img, 6, 5)

			// Insert photo with thumbnail and blurhash
			if _, err := tx.Exec("INSERT INTO ReportPhotos (ReportID, PhotoURL, ThumbnailURL, Blurhash) VALUES (?, ?, ?, ?)",
				reportID, fileURL, thumbnailURL, blurhashString); err != nil {
				select {
				case errorChan <- err:
				default:
				}
				return
			}
		}()
	}

	// Wait for all jobs to finish
	wg.Wait()
	close(errorChan)

	// Check for errors
	if err, ok := <-errorChan; ok {
		return fmt.Errorf("encountered an error during file upload or DB operation: %v", err)
	}

	// Commit the transaction after all operations succeed
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", ErrCommitTransaction, err)
	}

	return nil
}

func (s *ReportService) ApproveReport(reportID, userID int) error {
	tx, err := s.DB.Beginx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Approve the report
	res, err := tx.Exec(approveReportQuery, reportID, userID, reportID, userID)
	if err != nil {
		return fmt.Errorf("error approving report: %w", err)
	}

	if count, _ := res.RowsAffected(); count == 0 {
		return errors.New("no report updated, either report does not exist or user is not the owner")
	}

	// Update the marker with report details
	if err := s.UpdateMarkerWithReportDetailsTx(tx, reportID); err != nil {
		return err
	}

	// Fetch report details
	var report struct {
		MarkerID int `db:"MarkerID"`
		UserID   int `db:"UserID"`
	}
	err = tx.Get(&report, getReportByMarkerUserIdsQuery, reportID)
	if err != nil {
		return fmt.Errorf("failed to fetch report details: %w", err)
	}

	// Determine comment text
	var commentText string
	if report.UserID == 0 {
		commentText = "🔉 익명 정보 제공 업데이트"
	} else {
		// Fetch username
		var username string
		err = tx.Get(&username, getUsernameByIdQuery, report.UserID)
		if err != nil {
			return fmt.Errorf("failed to fetch username: %w", err)
		}
		commentText = fmt.Sprintf("🔉 %s님께서 정보 제공", username)
	}

	// Add comment as admin
	commentService := NewMarkerCommentService(s.DB)
	_, err = commentService.CreateCommentTx(tx, report.MarkerID, 1, "k-pullup", commentText)
	if err != nil {
		return fmt.Errorf("failed to create comment: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	// Update location and invalidate cache
	s.UpdateDbLocation(reportID)
	s.CacheService.InvalidateFullMarkersCache()

	return nil
}

func (s *ReportService) DenyReport(reportID, userID int) error {
	res, err := s.DB.Exec(denyReportQuery, reportID, userID, reportID, userID)
	if err != nil {
		return fmt.Errorf("error denying report: %w", err)
	}
	// Check if the row was actually updated
	if count, _ := res.RowsAffected(); count == 0 {
		return fmt.Errorf("no report updated, either report does not exist or user is not the owner")
	}

	return nil
}

func (s *ReportService) UpdateMarkerWithReportDetailsTx(tx *sqlx.Tx, reportID int) error {

	if _, err := tx.Exec(updateMarkeryReportQuery, reportID); err != nil {
		return fmt.Errorf("error updating marker with report details: %w", err)
	}

	// TODO gotta blurhash

	// Move approved report photos to the Photos table
	if _, err := tx.Exec(updateReportPhotoQuery, reportID); err != nil {
		return fmt.Errorf("error transferring report photos to marker photos: %w", err)
	}

	// Check and remove the oldest photos if the limit is exceeded
	if _, err := tx.Exec(deleteExcessPhotosQuery, reportID); err != nil {
		return fmt.Errorf("error removing excess photos: %w", err)
	}

	if _, err := tx.Exec(deleteReportPhotosQuery, reportID); err != nil {
		return fmt.Errorf("error deleting report photos after moving: %w", err)
	}

	return nil
}

func (s *ReportService) DeleteReport(reportID, userID, markerID int) error {
	// Start a transaction
	tx, err := s.DB.Beginx()
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if the user is authorized to delete the report
	// Prepare to check if the user has the right to delete the report
	var authorized bool
	if err := tx.Get(&authorized, checkAuthQuery, reportID, userID, userID); err != nil {
		return fmt.Errorf("error checking authorization: %w", err)
	}

	if !authorized {
		return fmt.Errorf("user %d is not authorized to delete report %d", userID, reportID)
	}

	// Delete report photos first
	var photoURLs []string
	if err := tx.Select(&photoURLs, selectPhotosQuery, reportID); err != nil {
		return fmt.Errorf("error deleting report photos: %w", err)
	}

	// Delete photos from database
	if _, err := tx.Exec(deleteReportPhotosQuery, markerID); err != nil {
		tx.Rollback()
		return fmt.Errorf("deleting photos: %w", err)
	}

	// Then delete the report
	if _, err := tx.Exec(deleteReportQuery, reportID); err != nil {
		return fmt.Errorf("error deleting report: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}

	// Delete photos from S3 in a goroutine
	go func(photoURLs []string) {
		for _, photoURL := range photoURLs {
			if err := s.S3Service.DeleteDataFromS3(photoURL); err != nil {
				s.Logger.Error("Failed to delete photo from S3", zap.String("photoURL", photoURL), zap.Error(err))
			}
		}
	}(photoURLs)

	return nil
}

func (s *ReportService) UpdateDbLocation(reportID int) {
	go func(reportID int) {
		maxRetries := 3
		retryDelay := 5 * time.Second // Delay between retries

		// Fetch latitude and longitude from the database
		var location dto.Location
		// Fetch the marker ID associated with the report
		var markerID int64
		mErr := s.DB.Get(&markerID, getReportByIdQuery, reportID)
		if mErr != nil {
			s.Logger.Error("Failed to fetch marker ID for report", zap.Int("reportID", reportID), zap.Error(mErr))
			return
		}

		mErr = s.LocationService.DB.Get(&location, updateDbLocationQuery, markerID)
		if mErr != nil {
			s.Logger.Error("Failed to fetch location for marker", zap.Int64("markerID", markerID), zap.Error(mErr))
			return
		}

		var address string
		var err error

		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				s.Logger.Info("Retrying to fetch address for marker", zap.Int64("markerID", markerID), zap.Int("attempt", attempt))
				time.Sleep(retryDelay) // Wait before retrying
			}
			address, err = s.LocationService.FacilityService.FetchAddressFromMap(location.Latitude, location.Longitude)
			if err == nil && address != "" {
				break // Success, exit the retry loop
			}

			s.Logger.Warn("Attempt to fetch address failed",
				zap.Int("attempt", attempt),
				zap.Int64("markerID", markerID),
				zap.Error(err))
		}

		if err != nil || address == "" {
			address2, _ := s.LocationService.FacilityService.FetchRegionFromAPI(location.Latitude, location.Longitude)
			if address2 == "-2" {
				// delete the marker 북한 or 일본
				_, err = s.LocationService.DB.Exec(deleteMarkerQuery, markerID)
				if err != nil {
					s.Logger.Error("Failed to delete marker", zap.Int64("markerID", markerID), zap.Error(err))
				}
				return // no need to insert in failures
			}

			var errMsg string
			if err != nil {
				errMsg = fmt.Sprintf("Final attempt failed to fetch address for marker %d: %v", markerID, err)
			} else {
				errMsg = fmt.Sprintf("No address found for marker %d after %d attempts", markerID, maxRetries)
			}

			water, _ := s.LocationService.FacilityService.FetchRegionWaterInfo(location.Latitude, location.Longitude)
			if water {
				errMsg = fmt.Sprintf("The marker (%d) might be above on water", markerID)
			}

			url := fmt.Sprintf("%sd=%d&la=%f&lo=%f", s.LocationService.Config.ClientURL, markerID, location.Latitude, location.Longitude)

			if _, logErr := s.LocationService.DB.Exec(insertMarkerFailureQuery, markerID, errMsg, url); logErr != nil {
				s.Logger.Error("Failed to log address fetch failure for marker", zap.Int64("markerID", markerID), zap.Error(logErr))
			}
			return
		}

		standardizedAddress := standardizeAddress(address)

		// Update the marker's address in the database after successful fetch
		_, err = s.LocationService.DB.Exec(updateMarkerAddressByIdQuery, standardizedAddress, markerID)
		if err != nil {
			s.Logger.Error("Failed to update address for marker", zap.Int64("markerID", markerID), zap.Error(err))
		}

	}(reportID)
}

func (s *ReportService) GetPendingReports() ([]dto.MarkerReportResponse, error) {
	rows, err := s.DB.Queryx(getPendingReportsQuery)
	if err != nil {
		return nil, fmt.Errorf("error querying reports: %w", err)
	}
	defer rows.Close()

	reportMap := make(map[int]*dto.MarkerReportResponse)
	for rows.Next() {
		var (
			r   dto.MarkerReportResponse
			url string
		)
		if err := rows.Scan(&r.ReportID, &r.MarkerID, &r.UserID, &r.Latitude, &r.Longitude,
			&r.NewLatitude, &r.NewLongitude, &r.Description, &r.CreatedAt, &r.Status, &r.DoesExist, &url); err != nil {
			return nil, err
		}
		// Check if the URL is not empty before appending
		if url != "" {
			if report, exists := reportMap[r.ReportID]; exists {
				report.PhotoURLs = append(report.PhotoURLs, url)
			} else {
				r.PhotoURLs = []string{url}
				reportMap[r.ReportID] = &r
			}
		} else {
			// Ensure that PhotoURLs is initialized even if there are no photos
			if _, exists := reportMap[r.ReportID]; !exists {
				r.PhotoURLs = []string{} // Initialize with an empty slice
				reportMap[r.ReportID] = &r
			}
		}
	}

	// Convert map to slice
	reports := make([]dto.MarkerReportResponse, 0, len(reportMap))
	for _, report := range reportMap {
		reports = append(reports, *report)
	}

	return reports, nil
}
