package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/xml"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/model"
	"github.com/Alfex4936/chulbong-kr/protos"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/gammazero/workerpool"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/xid"

	gocache "github.com/eko/gocache/lib/v4/cache"

	ristretto_store "github.com/eko/gocache/store/ristretto/v4"

	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/jmoiron/sqlx"
)

const (
	// Simplified query to fetch only the marker IDs, latitudes, and longitudes
	getAllSimpleMarkersQuery = `
SELECT 
	MarkerID, 
	ST_X(Location) AS Latitude,
	ST_Y(Location) AS Longitude
FROM 
	Markers;`

	getAllSimpleMarkersPhotoExistenceQuery = `
SELECT 
    m.MarkerID, 
    ST_X(m.Location) AS Latitude,
    ST_Y(m.Location) AS Longitude,
    CASE 
        WHEN COUNT(p.PhotoID) > 0 THEN TRUE 
        ELSE FALSE 
    END AS HasPhoto
FROM 
    Markers m
LEFT JOIN 
    Photos p ON m.MarkerID = p.MarkerID
GROUP BY 
    m.MarkerID;`

	getAllNewMakersQuery = `
SELECT 
	MarkerID, 
	ST_X(Location) AS Latitude,
	ST_Y(Location) AS Longitude,
	Address,
	UserID
FROM 
	Markers
ORDER BY 
	CreatedAt DESC
LIMIT ? OFFSET ?;`

	// access_type: const, query_cost: 1.00
	getAmarkerQuery = `
SELECT
	M.MarkerID,
	M.UserID,
	ST_X(M.Location) AS Latitude,
	ST_Y(M.Location) AS Longitude,
	M.Description,
	COALESCE(U.Username, '탈퇴한 사용자') AS Username,
	M.CreatedAt,
	M.UpdatedAt,
	M.Address,
	COALESCE(D.DislikeCount, 0) AS DislikeCount,
	COALESCE(F.FavoriteCount, 0) AS FavoriteCount
FROM Markers M
LEFT JOIN Users U ON M.UserID = U.UserID
LEFT JOIN (
	SELECT
		MarkerID,
		COUNT(*) AS DislikeCount
	FROM MarkerDislikes
	WHERE MarkerID = ?
	GROUP BY MarkerID
) D ON M.MarkerID = D.MarkerID
LEFT JOIN (
	SELECT
		MarkerID,
		COUNT(*) AS FavoriteCount
	FROM Favorites
	WHERE MarkerID = ?
	GROUP BY MarkerID
) F ON M.MarkerID = F.MarkerID
WHERE M.MarkerID = ?`

	getAllPhotosForMarkerQuery = "SELECT * FROM Photos WHERE MarkerID = ? ORDER BY UploadedAt DESC"

	// Query to select markers created by a specific user with LIMIT and OFFSET for pagination
	getMarkersByUserQuery = `
SELECT 
    M.MarkerID,
    ST_X(M.Location) AS Latitude, 
    ST_Y(M.Location) AS Longitude, 
    M.Description,
    M.CreatedAt,
	M.Address
FROM 
    Markers M
WHERE 
    M.UserID = ?
ORDER BY 
    M.CreatedAt DESC
LIMIT ? OFFSET ?`

	getOneMarkerByMarkerIdQuery = `
	SELECT 
		M.MarkerID,
		ST_X(M.Location) AS Latitude, 
		ST_Y(M.Location) AS Longitude, 
		M.Description,
		M.CreatedAt,
		M.Address
	FROM 
		Markers M
	WHERE 
		M.MarkerID = ?`

	// Query to get the total count of markers for the user
	getTotalCountofMarkerQuery = "SELECT COUNT(DISTINCT Markers.MarkerID) FROM Markers WHERE Markers.UserID = ?"

	insertMarkerQuery = "INSERT INTO Markers (UserID, Location, Description, CreatedAt, UpdatedAt) VALUES (?, ST_PointFromText(?, 4326), ?, NOW(), NOW())"
	insertPhotoQuery  = "INSERT INTO Photos (MarkerID, PhotoURL, UploadedAt) VALUES (?, ?, NOW())"
	// insertPhotoWithThumbQuery = "INSERT INTO Photos (MarkerID, PhotoURL, ThumbnailURL, UploadedAt) VALUES (?, ?, ?, NOW())"
	insertPhotoWithThumbQuery = "INSERT INTO Photos (MarkerID, PhotoURL, ThumbnailURL, Blurhash, UploadedAt) VALUES (?, ?, ?, ?, NOW())"

	deleteMarkerQuery = "DELETE FROM Markers WHERE MarkerID = ?"

	insertMarkerFailureQuery = "INSERT INTO MarkerAddressFailures (MarkerID, ErrorMessage, URL) VALUES (?, ?, ?)"

	// UNION
	//     SELECT PhotoURL FROM ReportPhotos WHERE PhotoURL
	getAllPhotosQuery = "SELECT PhotoURL FROM Photos WHERE PhotoURL IS NOT NULL"

	updateMarkerQuery     = "UPDATE Markers SET Latitude = ?, Longitude = ?, Description = ?, UpdatedAt = NOW() WHERE MarkerID = ?"
	updateMarkerDescQuery = "UPDATE Markers SET Description = ?, UpdatedAt = NOW() WHERE MarkerID = ?"

	getAllMarkersByUserQuery = "SELECT UserID FROM Markers WHERE MarkerID = ?"
	getPhotosForMarkerQuery  = "SELECT PhotoURL FROM Photos WHERE MarkerID = ?"

	deletePhotoQuery = "DELETE FROM Photos WHERE MarkerID = ?"

	updateTimeMarkerQuery = "UPDATE Markers SET UpdatedAt = NOW() WHERE MarkerID = ?"

	findCloseMarkersAdminQuery = `
SELECT MarkerID, ST_X(Location) AS Latitude, ST_Y(Location) AS Longitude, Description, ST_Distance_Sphere(Location, ST_GeomFromText(?, 4326)) AS distance, Address
FROM Markers
WHERE ST_Distance_Sphere(Location, ST_GeomFromText(?, 4326)) <= ?
ORDER BY distance ASC`

	generateRSSQuery = "SELECT MarkerID, UpdatedAt, Address FROM Markers ORDER BY UpdatedAt DESC"

	// getNewTop10PicturesQuery      = "SELECT MarkerID, COALESCE(ThumbnailURL, PhotoURL) AS PhotoURL FROM Photos GROUP BY MarkerID, PhotoURL ORDER BY MAX(UploadedAt) DESC LIMIT 10"
	getNewTop10PicturesQuery      = "SELECT MarkerID, COALESCE(ThumbnailURL, PhotoURL) AS PhotoURL, COALESCE(Blurhash, '') AS Blurhash FROM Photos WHERE (MarkerID, UploadedAt) IN ( SELECT MarkerID, MAX(UploadedAt) FROM Photos GROUP BY MarkerID ) ORDER BY UploadedAt DESC LIMIT 10"
	getNewTop10PicturesExtraQuery = `
SELECT p.MarkerID, COALESCE(p.ThumbnailURL, p.PhotoURL) As PhotoURL, m.Address, ST_X(m.Location) AS Latitude, ST_Y(m.Location) AS Longitude
FROM Photos p
JOIN (
    SELECT MarkerID, MAX(UploadedAt) AS LatestUpload
    FROM Photos
    GROUP BY MarkerID
) sub ON p.MarkerID = sub.MarkerID AND p.UploadedAt = sub.LatestUpload
LEFT JOIN Markers m ON p.MarkerID = m.MarkerID
ORDER BY p.UploadedAt DESC
LIMIT 10`
)

// MarkerManageService is a service for marker management operations.
type MarkerManageService struct {
	DB *sqlx.DB

	MarkerLocationService *MarkerLocationService
	S3Service             *S3Service
	// ZincSearchService     *ZincSearchService
	BleveSearchService *BleveSearchService
	RedisService       *RedisService
	ChatService        *ChatService

	MapUtil           *util.MapUtil
	BadWordUtil       *util.BadWordUtil
	LocalCacheStorage *ristretto_store.RistrettoStore

	Logger *zap.Logger

	// markersLocalCache cache to store encoded marker data
	// markersLocalCache []byte // 400 kb is fine here
	// cacheMutex        sync.RWMutex

	byteCache  *gocache.Cache[[]byte]
	workerPool *workerpool.WorkerPool

	CacheService *MarkerCacheService

	GetMarkerStmt             *sqlx.Stmt
	GetAllPhotosForMarkerStmt *sqlx.Stmt
	GetNewTop10PicturesStmt   *sqlx.Stmt
	GenerateRSSQueryStmt      *sqlx.Stmt
}

type MarkerManageServiceParams struct {
	fx.In

	DB                    *sqlx.DB
	MarkerLocationService *MarkerLocationService
	S3Service             *S3Service
	// ZincSearchService     *ZincSearchService
	BleveSearchService *BleveSearchService
	RedisService       *RedisService
	ChatService        *ChatService
	MapUtil            *util.MapUtil
	BadWordUtil        *util.BadWordUtil
	LocalCacheStorage  *ristretto_store.RistrettoStore
	Logger             *zap.Logger
	CacheService       *MarkerCacheService
}

// NewMarkerManageService creates a new instance of MarkerManageService.
func NewMarkerManageService(p MarkerManageServiceParams) *MarkerManageService {
	byteCache := gocache.New[[]byte](p.LocalCacheStorage)

	// Prepare query parameters
	getMarkerStmt, _ := p.DB.Preparex(getAmarkerQuery)
	getAllPhotosForMarkerStmt, _ := p.DB.Preparex(getAllPhotosForMarkerQuery)
	getNewTop10PicturesStmt, _ := p.DB.Preparex(getNewTop10PicturesQuery)
	generateRSSQueryStmt, _ := p.DB.Preparex(generateRSSQuery)

	return &MarkerManageService{
		DB:                    p.DB,
		MarkerLocationService: p.MarkerLocationService,
		S3Service:             p.S3Service,
		// ZincSearchService:     p.ZincSearchService,
		BleveSearchService: p.BleveSearchService,
		RedisService:       p.RedisService,
		ChatService:        p.ChatService,
		MapUtil:            p.MapUtil,
		BadWordUtil:        p.BadWordUtil,
		Logger:             p.Logger,

		byteCache:  byteCache,
		workerPool: workerpool.New(30),

		GetMarkerStmt:             getMarkerStmt,
		GetAllPhotosForMarkerStmt: getAllPhotosForMarkerStmt,
		GetNewTop10PicturesStmt:   getNewTop10PicturesStmt,
		GenerateRSSQueryStmt:      generateRSSQueryStmt,

		CacheService: p.CacheService,
	}
}

func RegisterMarkerLifecycle(lifecycle fx.Lifecycle, service *MarkerManageService) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			rss, _ := service.GenerateRSS()
			saveRSSToFile(rss, "marker_rss.xml")
			return nil
		},
		OnStop: func(context.Context) error {
			_ = service.GetAllPhotosForMarkerStmt.Close()
			_ = service.GetMarkerStmt.Close()
			_ = service.GetNewTop10PicturesStmt.Close()
			_ = service.GenerateRSSQueryStmt.Close()
			service.workerPool.StopWait()
			return nil
		},
	})
}

func (s *MarkerManageService) GetCache() []byte {
	// ctx := context.Background()
	// value, _ := s.byteCache.Get(ctx, "allMarkers")

	var value []byte
	s.RedisService.GetCacheEntry(s.RedisService.RedisConfig.AllMarkersKey, &value)
	return value
}

func (s *MarkerManageService) SetCache(mjson []byte) {
	s.RedisService.SetCacheEntry(s.RedisService.RedisConfig.AllMarkersKey, mjson, time.Hour*24)
	// ctx := context.Background()
	// s.byteCache.Set(ctx, "allMarkers", mjson)
}

func (s *MarkerManageService) ClearCache() {
	s.RedisService.ResetCache(s.RedisService.RedisConfig.AllMarkersKey)
	// ctx := context.Background()
	// s.byteCache.Delete(ctx, "allMarkers")
}

// GetAllMarkers now returns a simplified list of markers
func (s *MarkerManageService) GetAllMarkers() ([]dto.MarkerSimple, error) {
	var markers []dto.MarkerSimple
	err := s.DB.Select(&markers, getAllSimpleMarkersPhotoExistenceQuery)
	if err != nil {
		return nil, fmt.Errorf("error fetching markers: %w", err)
	}

	// go s.MarkerLocationService.Redis.AddGeoMarkers(markers)

	return markers, nil
}

// GetAllNewMarkers returns a paginated, simplified list of the most recently added markers.
func (s *MarkerManageService) GetAllNewMarkers(page, pageSize int) ([]dto.MarkerNewResponse, error) {
	if page < 1 {
		page = 1 // Ensure page starts at 1
	}
	if pageSize < 1 {
		pageSize = 10 // Default pageSize to 10 if invalid
	}
	offset := (page - 1) * pageSize

	var markers []dto.MarkerNewResponse
	err := s.DB.Select(&markers, getAllNewMakersQuery, pageSize, offset)
	if err != nil {
		return nil, fmt.Errorf("error fetching markers: %w", err)
	}

	return markers, nil
}

// GetMarker retrieves a single marker and its associated photo by the marker's ID
func (s *MarkerManageService) GetMarker(markerID int) (*model.MarkerWithPhotos, error) {
	var markersWithUsernames dto.MarkersWithUsernames
	err := s.GetMarkerStmt.Get(&markersWithUsernames, markerID, markerID, markerID)
	if err != nil {
		return nil, err
	}

	// Fetch all photos for this marker by descending order of upload time
	var photos []model.Photo
	err = s.GetAllPhotosForMarkerStmt.Select(&photos, markerID)
	if err != nil {
		return nil, fmt.Errorf("error fetching photos: %w", err)
	}

	// Assemble the final structure
	markersWithPhotos := model.MarkerWithPhotos{
		Marker:        markersWithUsernames.Marker,
		Photos:        photos,
		Username:      markersWithUsernames.Username,
		DislikeCount:  markersWithUsernames.DislikeCount,
		FavoriteCount: markersWithUsernames.FavoriteCount,
	}

	// PublishMarkerUpdate(fmt.Sprintf("user: %s", markersWithPhotos.Username))

	return &markersWithPhotos, nil
}

// GetAllMarkersWithAddr fetches all markers and returns only those with an address not found or empty.
func (s *MarkerManageService) GetAllMarkersWithAddr() ([]dto.MarkerSimpleWithAddr, error) {
	var markers []dto.MarkerSimpleWithAddr
	err := s.DB.Select(&markers, getAllSimpleMarkersQuery)
	if err != nil {
		return nil, fmt.Errorf("error fetching markers: %w", err)
	}

	var filteredMarkers []dto.MarkerSimpleWithAddr
	for i := range markers {
		// func FetchAddressFromAPI(latitude float64, longitude float64) (string, error)
		address, err := s.MarkerLocationService.FacilityService.FetchAddressFromAPI(markers[i].Latitude, markers[i].Longitude)
		if err != nil {
			continue
		}

		// Update the address and include only if "Address not found" or empty
		if address == "Address not found" || address == "" {
			markers[i].Address = address
			filteredMarkers = append(filteredMarkers, markers[i])
		}
	}

	return filteredMarkers, nil
}

func (s *MarkerManageService) GetAllMarkersByUserWithPagination(userID, page, pageSize int) ([]dto.MarkerSimpleWithDescription, int, error) {
	offset := (page - 1) * pageSize

	markersWithDescription := make([]dto.MarkerSimpleWithDescription, 0)
	err := s.DB.Select(&markersWithDescription, getMarkersByUserQuery, userID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}

	var total int
	err = s.DB.Get(&total, getTotalCountofMarkerQuery, userID)
	if err != nil {
		return nil, 0, err
	}

	return markersWithDescription, total, nil
}

func (s *MarkerManageService) GetMarkerSimpleWithDescription(markerID int) (dto.MarkerSimpleWithDescription, error) {

	var markerWithDescription dto.MarkerSimpleWithDescription
	err := s.DB.Get(&markerWithDescription, getOneMarkerByMarkerIdQuery, markerID)
	if err != nil {
		return markerWithDescription, err
	}

	return markerWithDescription, nil
}

func (s *MarkerManageService) CheckMarkerValidity(latitude, longitude float64, description string) *fiber.Error {
	var wg sync.WaitGroup
	errorChan := make(chan *fiber.Error, 1) // Buffer size 1 since we only care about the first error
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure all paths cancel the context to prevent context leak

	checks := []func(){
		// South Korea check goroutine (403)
		func() {
			if inSKorea := s.MarkerLocationService.MapUtil.IsInSouthKoreaPrecisely(latitude, longitude); !inSKorea {
				select {
				case errorChan <- fiber.NewError(fiber.StatusForbidden, "operation is only allowed within South Korea"):
					cancel() // Cancel other goroutines
				case <-ctx.Done():
				}
			}
		},
		// Marker proximity check goroutine (409)
		func() {
			if nearby, _ := s.MarkerLocationService.IsMarkerNearby(latitude, longitude, 10); nearby {
				select {
				case errorChan <- fiber.NewError(fiber.StatusConflict, "there is a marker already nearby"):
					cancel() // Cancel other goroutines
				case <-ctx.Done():
				}
			}
		},
		// Bad words check goroutine (400)
		func() {
			if containsBadWord, _ := s.BadWordUtil.CheckForBadWordsUsingTrie(description); containsBadWord {
				select {
				case errorChan <- fiber.NewError(fiber.StatusBadRequest, "comment contains inappropriate content"):
					cancel() // Cancel other goroutines
				case <-ctx.Done():
				}
			}
		},
		// Restricted area check goroutine (422)
		func() {
			if name, restricted, _ := s.MarkerLocationService.IsMarkerInRestrictedArea(latitude, longitude); restricted {
				select {
				case errorChan <- fiber.NewError(fiber.StatusUnprocessableEntity, "marker is in restricted area: "+name):
					cancel() // Cancel other goroutines
				case <-ctx.Done():
				}
			}
		},

		// TODO: http link check
	}

	for _, check := range checks {
		wg.Add(1)
		go func(c func()) {
			defer wg.Done()
			c()
		}(check)
	}

	// Wait for all goroutines to finish in a separate goroutine to close the channel safely
	go func() {
		wg.Wait()
		close(errorChan)
	}()

	// Process the first error encountered
	if err := <-errorChan; err != nil {
		return err
	}

	return nil
}

// TODO: _, err = tx.Exec("SET TRANSACTION ISOLATION LEVEL SERIALIZABLE")
func (s *MarkerManageService) CreateMarkerWithPhotos(ctx context.Context, markerDto *dto.MarkerRequest, userID int, form *multipart.Form) (*dto.MarkerResponse, error) {
	s.ClearCache()

	// Begin a transaction for database operations
	tx, err := s.DB.Beginx()
	if err != nil {
		return nil, err
	}
	point := formatPoint(markerDto.Latitude, markerDto.Longitude)

	// Insert the marker into the database
	res, err := tx.Exec(
		insertMarkerQuery,
		userID, point, markerDto.Description,
	)

	if err != nil {
		tx.Rollback()
		return nil, err
	}

	markerID, _ := res.LastInsertId()
	folder := fmt.Sprintf("markers/%d", markerID)

	// Create a cancellable context for the worker tasks.
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channel to collect errors
	errorChan := make(chan error, 1) // Buffer of 1 to capture the first error

	// Process file uploads from the multipart form
	files := form.File["photos"]
	if len(files) > 5 {
		files = files[:5] // Limit to 5 files
	}

	var wg sync.WaitGroup
	wg.Add(len(files))

	// Submit the tasks to the worker pool
	for _, fileHeader := range files {
		fileHeader := fileHeader // capture loop variable

		s.workerPool.Submit(func() {
			defer wg.Done()

			// Use a per-task timeout to prevent hanging tasks.
			perTaskCtx, taskCancel := context.WithTimeout(taskCtx, 15*time.Second)
			defer taskCancel()

			// Upload the file to S3 using a context-aware method.
			fileURL, thumbnailURL, err := s.S3Service.UploadFileToS3WithContext(perTaskCtx, folder, fileHeader, true)
			if err != nil {
				select {
				case errorChan <- fmt.Errorf("S3 upload failed: %w", err):
				default:
				}
				return
			}

			file, _ := fileHeader.Open()
			defer file.Close()

			buf := new(bytes.Buffer)
			_, _ = io.Copy(buf, file)
			rawBytes := buf.Bytes()
			img, _, err := image.Decode(bytes.NewReader(rawBytes))
			if err != nil {
				select {
				case errorChan <- fmt.Errorf(" 이미지 디코딩 실패: %w", err):
				default:
				}
				return
			}
			blurhashString := util.EncodeBlurHashImage(img, 6, 5)

			// Insert photo into the database
			if _, err := tx.Exec(insertPhotoWithThumbQuery, markerID, fileURL, thumbnailURL, blurhashString); err != nil {
				select {
				case errorChan <- err:
				default:
				}
				return
			}
		})
	}

	// Wait for all jobs to finish
	wg.Wait()
	close(errorChan)

	// Check for errors
	if err, ok := <-errorChan; ok {
		tx.Rollback()
		return nil, fmt.Errorf("encountered an error during file upload or DB operation: %v", err)
	}

	// Commit the transaction after all operations succeed
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	go func(markerID int64, latitude, longitude float64, pics int) {
		maxRetries := 3
		retryDelay := 5 * time.Second // Delay between retries

		var address string
		var err error

		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				s.Logger.Warn("Retrying to fetch address", zap.Int64("markerID", markerID), zap.Int("attempt", attempt))
				time.Sleep(retryDelay) // Wait before retrying
			}

			address, err = s.MarkerLocationService.FacilityService.FetchAddressFromMap(latitude, longitude)
			if err == nil && address != "" {
				break // Success, exit the retry loop
			}

			s.Logger.Warn("Failed to fetch address", zap.Int("attempt", attempt), zap.Int64("markerID", markerID), zap.Error(err))
		}

		if err != nil || address == "" {
			address2, _ := s.MarkerLocationService.FacilityService.FetchRegionFromAPI(latitude, longitude)
			if address2 == "-2" {
				// delete the marker 북한 or 일본
				_, err = s.DB.Exec(deleteMarkerQuery, markerID)
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

			// water, _ := s.MarkerLocationService.FacilityService.FetchRegionWaterInfo(latitude, longitude)
			// if water {
			// 	errMsg = fmt.Sprintf("The marker (%d) might be above on water", markerID)
			// }

			url := fmt.Sprintf("%sd=%d&la=%f&lo=%f", s.MarkerLocationService.Config.ClientURL, markerID, markerDto.Latitude, markerDto.Longitude)

			if _, logErr := s.DB.Exec(insertMarkerFailureQuery, markerID, errMsg, url); logErr != nil {
				s.Logger.Error("Failed to log address fetch failure", zap.Int64("markerID", markerID), zap.Error(logErr))
			}
			return
		}

		// Standardize the address
		standardizedAddress := standardizeAddress(address)

		// Update the marker's address in the database after successful fetch
		_, err = s.DB.Exec(updateAddressQuery, standardizedAddress, markerID)
		if err != nil {
			s.Logger.Error("Failed to update address", zap.Int64("markerID", markerID), zap.Error(err))
		}

		err = s.BleveSearchService.InsertMarkerIndex(dto.MarkerIndexData{MarkerID: int(markerID), Address: address})
		if err != nil {
			s.Logger.Error("Failed to index address", zap.Int64("markerID", markerID), zap.Error(err))
		}

		if userID != 1 { // not for admin
			util.SendSlackNewMarkerNotification(markerID, address, markerDto.Description, latitude, longitude, pics)
		}

		regionCode := getRegionCode(standardizedAddress)
		if regionCode == "" {
			s.Logger.Error("Failed to determine region code", zap.Int64("markerID", markerID), zap.String("address", standardizedAddress))
		} else {
			message := dto.BroadcastMessage{
				Timestamp:    time.Now().UnixMilli(),
				UID:          xid.New().String(),
				Message:      fmt.Sprintf("새로운 철봉 (%d): %s", markerID, standardizedAddress),
				UserID:       "1",
				UserNickname: "k-pullup",
				RoomID:       regionCode,
			}

			// Call SaveMessageToRedis
			err := s.ChatService.SaveMessageToRedis(context.Background(), message)
			if err != nil {
				s.Logger.Error("Failed to save message to Redis", zap.Error(err))
			}
		}

		// userIDstr := strconv.Itoa(userID)
		// updateMsg := fmt.Sprintf("새로운 철봉이 [ %s ]에 등록되었습니다!", address)
		// metadata := notification.NotificationMarkerMetadata{
		// 	MarkerID:  markerID,
		// 	Latitude:  latitude,
		// 	Longitude: longitude,
		// 	Address:   address,
		// }

		// rawMetadata, _ := json.Marshal(metadata)
		// PostNotification(userIDstr, "NewMarker", "k-pullup!", updateMsg, rawMetadata)

		// TODO: update when frontend updates
		// if address != "" {
		// 	updateMsg := fmt.Sprintf("새로운 철봉이 등록되었습니다! %s", address)
		// 	PublishMarkerUpdate(updateMsg)
		// }

	}(markerID, markerDto.Latitude, markerDto.Longitude, len(files))

	// go s.MarkerLocationService.Redis.ResetAllCache(fmt.Sprintf("userMarkers:%d:page:*", userID))
	go s.CacheService.RemoveUserMarker(userID, int(markerID))
	go s.CacheService.InvalidateFullMarkersCache()

	// Construct and return the response
	return &dto.MarkerResponse{
		MarkerID:    int(markerID),
		Latitude:    markerDto.Latitude,
		Longitude:   markerDto.Longitude,
		Description: markerDto.Description,
		// PhotoURLs:   photoURLs,
	}, nil
}

func (s *MarkerManageService) FetchAllPhotoURLsFromDB() ([]string, error) {
	rows, err := s.DB.Query(getAllPhotosQuery)
	if err != nil {
		return nil, fmt.Errorf("error querying database: %w", err)
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		urls = append(urls, url)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return urls, nil
}

// UpdateMarker updates an existing marker's latitude, longitude, and description
func (s *MarkerManageService) UpdateMarker(marker *model.Marker) error {
	_, err := s.DB.Exec(updateMarkerQuery, marker.Latitude, marker.Longitude, marker.Description, marker.MarkerID)
	return err
}

func (s *MarkerManageService) UpdateMarkerDescriptionOnly(markerID int, description string) error {
	_, err := s.DB.Exec(updateMarkerDescQuery, description, markerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no marker found with markerID %d", markerID)
		}
		return fmt.Errorf("error updating a marker: %w", err)
	}

	return nil
}

func (s *MarkerManageService) DeleteMarker(userID, markerID int, userRole string) error {
	// Precheck user authorization and fetch photo URLs before transaction
	var ownerID sql.NullInt64
	var photoURLs []string
	err := s.DB.Get(&ownerID, getAllMarkersByUserQuery, markerID)
	if err != nil {
		return fmt.Errorf("checking marker ownership: %w", err)
	}

	if userRole != "admin" && int(ownerID.Int64) != userID {
		return fmt.Errorf("user %d is not authorized to delete marker %d", userID, markerID)
	}

	err = s.DB.Select(&photoURLs, getPhotosForMarkerQuery, markerID)
	if err != nil {
		return fmt.Errorf("fetching photos: %w", err)
	}

	// Start a transaction
	tx, err := s.DB.Beginx()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}

	// Delete photos from database
	if _, err := tx.Exec(deletePhotoQuery, markerID); err != nil {
		tx.Rollback()
		return fmt.Errorf("deleting photos: %w", err)
	}

	// Delete the marker
	if _, err := tx.Exec(deleteMarkerQuery, markerID); err != nil {
		tx.Rollback()
		return fmt.Errorf("deleting marker: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	// Delete photos from S3 in a goroutine
	go func(photoURLs []string) {
		for _, photoURL := range photoURLs {
			if err := s.S3Service.DeleteDataFromS3(photoURL); err != nil {
				s.Logger.Error("Failed to delete photo from S3", zap.String("photoURL", photoURL), zap.Error(err))
			}
		}
	}(photoURLs)

	s.ClearCache()
	s.BleveSearchService.DeleteMarkerIndex(markerID)

	return nil
}

func (s *MarkerManageService) UploadMarkerPhotoToS3(markerID int, files []*multipart.FileHeader) ([]string, error) {
	// Begin a transaction for database operations
	tx, err := s.DB.Beginx()
	if err != nil {
		return nil, err
	}

	defer tx.Rollback()

	folder := fmt.Sprintf("markers/%d", markerID)

	picUrls := make([]string, 0)
	// Process file uploads from the multipart form
	for _, file := range files {
		fileURL, thumbnailURL, err := s.S3Service.UploadFileToS3(folder, file, true)
		if err != nil {
			fmt.Printf("Failed to upload file to S3: %v\n", err)
			continue // Skip this file and continue with the next
		}
		picUrls = append(picUrls, fileURL)
		// Associate each photo with the marker in the database
		if _, err := tx.Exec(insertPhotoWithThumbQuery, markerID, fileURL, thumbnailURL); err != nil {
			// Attempt to delete the uploaded file from S3
			if delErr := s.S3Service.DeleteDataFromS3(fileURL); delErr != nil {
				fmt.Printf("Also failed to delete the file from S3: %v\n", delErr)
			}
			return nil, err
		}
	}

	// Update Marker's UpdatedAt field
	if _, err := tx.Exec(updateTimeMarkerQuery, markerID); err != nil {
		return nil, err
	}

	// If no errors, commit the transaction
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return picUrls, nil
}

func (s *MarkerManageService) CheckNearbyMarkersInDB() ([]dto.MarkerGroup, error) {
	markers, err := s.GetAllMarkers()
	if err != nil {
		return nil, fmt.Errorf("error fetching markers: %w", err)
	}

	var markerGroups []dto.MarkerGroup

	for _, marker := range markers {
		point := formatPoint(marker.Latitude, marker.Longitude)

		var nearbyMarkers []dto.MarkerWithDistance
		err := s.DB.Select(&nearbyMarkers, findCloseMarkersAdminQuery, point, point, 10)
		if err != nil {
			return nil, fmt.Errorf("error checking for nearby markers: %w", err)
		}

		// 필터링하여 자기 자신을 제외한 마커만 포함시키기
		var filteredNearbyMarkers []dto.MarkerWithDistance
		for _, nMarker := range nearbyMarkers {
			if nMarker.MarkerID != marker.MarkerID {
				filteredNearbyMarkers = append(filteredNearbyMarkers, nMarker)
			}
		}

		// 주변에 다른 마커들이 있는 경우에만 결과에 추가
		if len(filteredNearbyMarkers) > 0 {
			markerGroup := dto.MarkerGroup{
				CentralMarker: marker,
				NearbyMarkers: filteredNearbyMarkers,
			}
			markerGroups = append(markerGroups, markerGroup)
		}
	}

	return markerGroups, nil
}

func (s *MarkerManageService) GenerateRSS() (string, error) {
	var markers []dto.MarkerRSS
	err := s.GenerateRSSQueryStmt.Select(&markers)
	if err != nil {
		return "", fmt.Errorf("error fetching markers: %w", err)
	}

	return generateRSS(markers)
}

func (s *MarkerManageService) GetAllMarkersProto() ([]*protos.Marker, error) {
	var markers []*protos.Marker
	err := s.DB.Select(&markers, getAllSimpleMarkersQuery)
	if err != nil {
		return nil, fmt.Errorf("error fetching markers: %w", err)
	}

	return markers, nil
}

func (s *MarkerManageService) GetNewTop10Pictures() ([]dto.MarkerNewPicture, error) {
	var markers []dto.MarkerNewPicture
	err := s.GetNewTop10PicturesStmt.Select(&markers)
	if err != nil {
		return nil, fmt.Errorf("error fetching markers: %w", err)
	}

	return markers, nil
}

func (s *MarkerManageService) GetNewTop10PicturesWithExtra() ([]dto.MarkerNewPictureExtra, error) {
	var markers []dto.MarkerNewPictureExtra
	err := s.DB.Select(&markers, getNewTop10PicturesExtraQuery)
	if err != nil {
		return nil, fmt.Errorf("error fetching markers: %w", err)
	}

	var wg sync.WaitGroup

	for i := range markers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			weather, err := s.MarkerLocationService.FacilityService.FetchWeatherFromAddress(markers[i].Latitude, markers[i].Longitude)
			if err != nil {
				return
			}

			var sb strings.Builder
			sb.WriteString(weather.Temperature)
			sb.WriteString("도 ")
			sb.WriteString(weather.Desc)
			sb.WriteString(" (강수량 ")
			sb.WriteString(weather.Rainfall)
			sb.WriteString("%)")

			markers[i].Weather = sb.String()
		}(i)
	}

	wg.Wait()

	return markers, nil
}

func generateRSS(markers []dto.MarkerRSS) (string, error) {
	items := []dto.RssItem{}
	for _, marker := range markers {
		item := dto.RssItem{
			Title:       fmt.Sprintf("Marker %d", marker.MarkerID),
			Link:        fmt.Sprintf("https://www.k-pullup.com/pullup/%d", marker.MarkerID),
			Description: marker.Address,
			PubDate:     marker.UpdatedAt.Format(time.RFC1123Z),
		}
		items = append(items, item)
	}

	rss := dto.RSS{
		Version: "2.0",
		Channel: dto.RssChannel{
			Title:         "k-pullup Markers",
			Link:          "https://www.k-pullup.com",
			Description:   fmt.Sprintf("Public pull-up bars in South Korea (%d)", len(markers)),
			LastBuildDate: time.Now().Format(time.RFC1123Z),
			Items:         items,
		},
	}

	output, err := xml.MarshalIndent(rss, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshalling RSS: %w", err)
	}
	return `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + string(output), nil
}

func saveRSSToFile(content, filePath string) error {
	return os.WriteFile(filePath, []byte(content), 0644)
}

// 대한민국의 행정 구역
// https://ko.wikipedia.org/wiki/%EB%8C%80%ED%95%9C%EB%AF%BC%EA%B5%AD%EC%9D%98_%ED%96%89%EC%A0%95_%EA%B5%AC%EC%97%AD
func standardizeProvinceForDB(province string) string {
	provinceMap := map[string]string{
		// 1
		"서울":    "서울특별시",
		"서울특별시": "서울특별시",
		"서울시":   "서울특별시",

		// 2
		"부산":    "부산광역시",
		"부산광역시": "부산광역시",
		"부산시":   "부산광역시",

		// 3
		"대구":    "대구광역시",
		"대구광역시": "대구광역시",
		"대구시":   "대구광역시",

		// 4
		"인천":    "인천광역시",
		"인천광역시": "인천광역시",
		"인천시":   "인천광역시",

		// 5
		"광주":    "광주광역시",
		"광주광역시": "광주광역시",
		"광주시":   "광주광역시",

		// 6
		"대전":    "대전광역시",
		"대전광역시": "대전광역시",
		"대전시":   "대전광역시",

		// 7
		"울산":    "울산광역시",
		"울산광역시": "울산광역시",
		"울산시":   "울산광역시",
		// 8
		"세종":      "세종특별자치시",
		"세종특별자치시": "세종특별자치시",
		"세종시":     "세종특별자치시",

		// 9
		"경기":  "경기도",
		"경기도": "경기도",

		// 10
		"강원":      "강원특별자치도",
		"강원도":     "강원특별자치도",
		"강원특별자치도": "강원특별자치도",

		// 11
		"충북":   "충청북도",
		"충청북도": "충청북도",

		// 12
		"충남":   "충청남도",
		"충청남도": "충청남도",

		// 13
		"전북":      "전북특별자치도",
		"전북특별자치도": "전북특별자치도",

		// 14
		"전남":   "전라남도",
		"전라남도": "전라남도",

		// 15
		"경북":   "경상북도",
		"경상북도": "경상북도",
		// 16
		"경남":   "경상남도",
		"경상남도": "경상남도",

		// 17
		"제주도":     "제주특별자치도",
		"제주":      "제주특별자치도",
		"제주특별자치도": "제주특별자치도",
	}

	if standardized, exists := provinceMap[province]; exists {
		return standardized
	}
	return province
}

// standardizeAddress standardizes the first part of the address.
func standardizeAddress(address string) string {
	addressParts := strings.Fields(address)
	if len(addressParts) > 0 {
		province := addressParts[0]
		standardizedProvince := standardizeProvinceForDB(province)
		if province != standardizedProvince {
			addressParts[0] = standardizedProvince
		}
	}
	return strings.Join(addressParts, " ")
}

type Region struct {
	Name string
	Code string
}

var regions = []Region{
	{Name: "제주특별자치도", Code: "jj"},
	{Name: "전라남도", Code: "jn"},
	{Name: "전라북도", Code: "jb"},
	{Name: "경상남도", Code: "gn"},
	{Name: "경상북도", Code: "gb"},
	{Name: "대구광역시", Code: "dg"},
	{Name: "울산광역시", Code: "us"},
	{Name: "충청북도", Code: "cb"},
	{Name: "충청남도", Code: "cn"},
	{Name: "대전광역시", Code: "dj"},
	{Name: "강원특별자치도", Code: "gw"},
	{Name: "경기도", Code: "gg"},
	{Name: "서울특별시", Code: "so"},
	{Name: "인천광역시", Code: "ic"},
	{Name: "부산광역시", Code: "bs"},
}

func getRegionCode(address string) string {
	for _, region := range regions {
		if strings.HasPrefix(address, region.Name) {
			return region.Code
		}
	}
	return ""
}
