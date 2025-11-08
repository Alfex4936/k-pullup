package facade

import (
	"bytes"
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"image"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/model"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// AdminFacadeService provides a simplified interface to various admin-related services.

var (
	imagePool = sync.Pool{
		New: func() any {
			return &bytes.Buffer{}
		},
	}
)

type AdminFacadeService struct {
	MarkerManage   *service.MarkerManageService
	S3Service      *service.S3Service
	ChatService    *service.ChatService
	MarkerFacility *service.MarkerFacilityService
	RedisService   *service.RedisService

	HTTPClient *http.Client

	Logger *zap.Logger
}

type AdminFacadeParams struct {
	fx.In

	MarkerManage   *service.MarkerManageService
	S3Service      *service.S3Service
	ChatService    *service.ChatService
	MarkerFacility *service.MarkerFacilityService
	RedisService   *service.RedisService

	HTTPClient *http.Client
	Logger     *zap.Logger
}

func NewAdminFacadeService(
	p AdminFacadeParams,
) *AdminFacadeService {
	return &AdminFacadeService{
		MarkerManage:   p.MarkerManage,
		S3Service:      p.S3Service,
		ChatService:    p.ChatService,
		MarkerFacility: p.MarkerFacility,
		RedisService:   p.RedisService,
		HTTPClient:     p.HTTPClient,
		Logger:         p.Logger,
	}
}

func (afs *AdminFacadeService) FetchAllPhotoURLsFromDB() ([]string, error) {
	return afs.MarkerManage.FetchAllPhotoURLsFromDB()
}

func (afs *AdminFacadeService) ListAllObjectsInS3() ([]map[string]interface{}, error) {
	return afs.S3Service.ListAllObjectsInS3()
}

func (afs *AdminFacadeService) FindUnreferencedS3Objects(dbURLs []string, s3Keys []string) []string {
	return afs.S3Service.FindUnreferencedS3Objects(dbURLs, s3Keys)
}

func (afs *AdminFacadeService) DeleteDataFromS3(dataURL string) error {
	return afs.S3Service.DeleteDataFromS3(dataURL)
}

func (afs *AdminFacadeService) BanUser(markerID, userID string, duration time.Duration) error {
	return afs.ChatService.BanUser(markerID, userID, duration)
}

func (afs *AdminFacadeService) FetchLatestMarkers(thresholdDate time.Time) ([]service.DataItem, error) {
	return afs.MarkerFacility.FetchLatestMarkers(thresholdDate)
}

func (afs *AdminFacadeService) FetchRoadViewPicDate(latitude, longitude float64) (time.Time, error) {
	return afs.MarkerFacility.FetchRoadViewPicDate(latitude, longitude)
}

func (afs *AdminFacadeService) CheckMarkerValidity(latitude, longitude float64, description string) *fiber.Error {
	return afs.MarkerManage.CheckMarkerValidity(latitude, longitude, description)
}

func (afs *AdminFacadeService) CreateMarkerWithPhotos(ctx context.Context, markerDto *dto.MarkerRequest, userID int, form *multipart.Form) (*dto.MarkerResponse, error) {
	return afs.MarkerManage.CreateMarkerWithPhotos(ctx, markerDto, userID, form)
}

func (afs *AdminFacadeService) SetMarkerFacilities(markerID int, facilities []dto.FacilityQuantity) error {
	return afs.MarkerFacility.SetMarkerFacilities(markerID, facilities)
}

func (afs *AdminFacadeService) ResetMarkerCache() {
	afs.MarkerManage.ClearCache()
}

func (afs *AdminFacadeService) GetUniqueVisitorsDB(date string) (int, error) {
	var count int

	const query = "SELECT COUNT(*) FROM visitors WHERE visit_date = ?"

	err := afs.MarkerManage.DB.Get(&count, query, date)
	return count, err
}

func (afs *AdminFacadeService) CreateNotice(title, content string, authorID int) (int, error) {
	res, err := afs.MarkerManage.DB.Exec(`
        INSERT INTO Notices (Title, Content, AuthorID, Published)
        VALUES (?, ?, ?, 1)`, // Default to published
		title, content, authorID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert notice: %w", err)
	}

	noticeID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return int(noticeID), nil
}

// PublishNotice sets Published=1, Unpublish sets Published=0
func (afs *AdminFacadeService) PublishNotice(noticeID int, publish bool) error {
	pubVal := 0
	if publish {
		pubVal = 1
	}

	_, err := afs.MarkerManage.DB.Exec(`
        UPDATE Notices 
        SET Published = ? 
        WHERE NoticeID = ?
    `, pubVal, noticeID)
	if err != nil {
		return fmt.Errorf("failed to update notice: %w", err)
	}
	return nil
}

func (afs *AdminFacadeService) DeleteNotice(noticeID int) error {
	_, err := afs.MarkerManage.DB.Exec(`
        DELETE FROM Notices 
        WHERE NoticeID = ?
    `, noticeID)
	if err != nil {
		return fmt.Errorf("failed to delete notice: %w", err)
	}
	return nil
}

func (afs *AdminFacadeService) ListNotices() ([]model.Notice, error) {
	notices := []model.Notice{}
	err := afs.MarkerManage.DB.Select(&notices,
		`SELECT NoticeID, Title, Content, CreatedAt, UpdatedAt 
         FROM Notices
         ORDER BY CreatedAt DESC`)
	if err != nil {
		return nil, err
	}
	return notices, nil
}

// DeleteMarkerPhoto deletes the photo (and its S3 objects) at the given index (ordered by UploadedAt)
// for a given marker. The photo is selected using an index-based query (using an index on MarkerID, UploadedAt).
func (afs *AdminFacadeService) DeleteMarkerPhoto(ctx context.Context, markerID, photoIdx int) error {
	// Begin a transaction to ensure atomicity of the DB operations.
	tx, err := afs.MarkerManage.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	// Ensure a rollback if anything fails.
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Query for the photo at the given marker and index.
	// The index on (MarkerID, UploadedAt) makes this lookup efficient.
	query := `
        SELECT PhotoID, PhotoURL, ThumbnailURL
        FROM Photos
        WHERE MarkerID = ?
        ORDER BY UploadedAt ASC
        LIMIT 1 OFFSET ?
    `
	var photoID int
	var photoURL string
	// ThumbnailURL is stored in case you want to delete it separately.
	// (Note: your S3Service.DeleteDataFromS3 may already delete the thumbnail if the key does not contain "_thumb".)
	var thumbURL sql.NullString

	if err = tx.QueryRowContext(ctx, query, markerID, photoIdx).Scan(&photoID, &photoURL, &thumbURL); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no photo found for marker %d at index %d", markerID, photoIdx)
		}
		return fmt.Errorf("querying photo: %w", err)
	}

	// Delete the photo file from S3.
	// The S3 service will handle deletion of the thumbnail if appropriate.
	if err = afs.S3Service.DeleteDataFromS3(photoURL); err != nil {
		return fmt.Errorf("failed to delete photo from S3: %w", err)
	}

	// (Optional) to explicitly delete the thumbnail from S3 and it isn’t handled automatically,
	// if thumbURL.Valid && thumbURL.String != "" {
	//     if err = afs.S3Service.DeleteDataFromS3(thumbURL.String); err != nil {
	//         return fmt.Errorf("failed to delete thumbnail from S3: %w", err)
	//     }
	// }

	// Delete the photo record from the database.
	delQuery := `DELETE FROM Photos WHERE PhotoID = ?`
	if _, err = tx.ExecContext(ctx, delQuery, photoID); err != nil {
		return fmt.Errorf("failed to delete photo record: %w", err)
	}

	// Commit the transaction.
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (afs *AdminFacadeService) OptimizeImage(srcURL string, width, quality int, acceptHeader string) ([]byte, string, error) {
	// Validate the input.
	if srcURL == "" {
		return nil, "", fmt.Errorf("missing url parameter")
	}

	// Determine output format.
	var ext string
	if strings.Contains(acceptHeader, "image/webp") {
		ext = ".webp"
	} else {
		ext = strings.ToLower(path.Ext(srcURL))
		// If the extension isn't one of the supported types, default to JPEG.
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".gif" && ext != ".webp" {
			ext = ".jpeg"
		}
	}

	// Build a unique cache key based on the source URL and parameters.
	hash := md5.Sum([]byte(srcURL))
	hashStr := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("optimized_image:%s:%d:%d:%s", hashStr, width, quality, ext)

	// Check Redis cache for an existing optimized image.
	var cached []byte
	if err := afs.RedisService.GetCacheEntry(cacheKey, &cached); err == nil && len(cached) > 0 {
		var contentType string
		switch ext {
		case ".png":
			contentType = "image/png"
		case ".gif":
			contentType = "image/gif"
		case ".webp":
			contentType = "image/webp"
		default:
			contentType = "image/jpeg"
		}
		return cached, contentType, nil
	}

	// Fetch the source image.
	resp, err := afs.HTTPClient.Get(srcURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to fetch image")
	}
	defer resp.Body.Close()

	// Decode the image.
	srcImg, err := imaging.Decode(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image: %w", err)
	}

	// Resize the image if a width is provided (height is auto-calculated).
	var dstImg *image.NRGBA
	if width > 0 {
		dstImg = imaging.Resize(srcImg, width, 0, imaging.Lanczos)
	} else {
		// If the image is already in the desired format, avoid cloning.
		if nrgba, ok := srcImg.(*image.NRGBA); ok {
			dstImg = nrgba
		} else {
			dstImg = imaging.Clone(srcImg)
		}
	}

	// Acquire a buffer from the pool.
	buf := imagePool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset() // Ensure the buffer is cleared before returning it.
		imagePool.Put(buf)
	}()

	// Encode the optimized image into the chosen format.
	var contentType string
	switch ext {
	case ".png":
		contentType = "image/png"
		err = imaging.Encode(buf, dstImg, imaging.PNG)
	case ".gif":
		contentType = "image/gif"
		err = imaging.Encode(buf, dstImg, imaging.GIF)
	case ".webp":
		contentType = "image/webp"
		err = webp.Encode(buf, dstImg, &webp.Options{Quality: float32(quality)})
	default: // .jpeg or .jpg
		contentType = "image/jpeg"
		err = imaging.Encode(buf, dstImg, imaging.JPEG, imaging.JPEGQuality(quality))
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to encode image: %w", err)
	}

	resultBytes := buf.Bytes()

	// Cache the optimized image in Redis with an expiration (e.g., 24 hours).
	cacheExpiration := 24 * time.Hour
	if err := afs.RedisService.SetCacheEntry(cacheKey, resultBytes, cacheExpiration); err != nil {
		// Log the error but do not fail the request.
		afs.Logger.Error("failed to set cache entry", zap.Error(err))
	}

	return resultBytes, contentType, nil
}

// GetActiveUsersInRoom returns active users in a chat room (stub implementation)
func (afs *AdminFacadeService) GetActiveUsersInRoom(roomID int) ([]string, error) {
	// TODO: Implement actual logic
	return []string{}, nil
}

// GetUserCountInRoom returns the count of users in a chat room (stub implementation)
func (afs *AdminFacadeService) GetUserCountInRoom(roomID int) (int, error) {
	// TODO: Implement actual logic
	return 0, nil
}

// OptimizeImageWithContext optimizes an image with context (stub implementation)
func (afs *AdminFacadeService) OptimizeImageWithContext(ctx context.Context, file *multipart.FileHeader, quality int) ([]byte, string, error) {
	// TODO: Implement actual logic with context
	return nil, "", fmt.Errorf("OptimizeImageWithContext not implemented")
}

// GetUsersWithWarnings returns users with warning counts (stub implementation)
func (afs *AdminFacadeService) GetUsersWithWarnings() ([]dto.UserWarningResponse, error) {
	// TODO: Implement actual logic
	return []dto.UserWarningResponse{}, nil
}

// UpdateUserWarning updates a user's warning count (stub implementation)
func (afs *AdminFacadeService) UpdateUserWarning(userID int, action int, reason string) (*dto.UserWarningResponse, error) {
	// TODO: Implement actual logic
	return &dto.UserWarningResponse{
		UserID:       userID,
		Username:     "stub",
		Email:        "stub@example.com",
		WarningCount: 0,
	}, nil
}

// GetUserCommentCreationStatus returns whether a user can create comments (stub implementation)
func (afs *AdminFacadeService) GetUserCommentCreationStatus(userID int) (bool, error) {
	// TODO: Implement actual logic
	return true, nil
}
