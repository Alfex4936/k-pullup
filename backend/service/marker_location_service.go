package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Alfex4936/chulbong-kr/config"
	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/jmoiron/sqlx"
	"github.com/rs/xid"
	"go.uber.org/fx"
)

const (
	earthRadius = 6371000

	existNearbyMarkerQuery = `
SELECT EXISTS (
    SELECT 1 
    FROM Markers
    WHERE ST_Within(Location, ST_Buffer(ST_GeomFromText(?, 4326), ?))
) AS Nearby;
`

	isInRestrictedAreaQuery = `
SELECT Name 
FROM RestrictedAreas 
WHERE MBRContains(Polygon, ST_GeomFromText(?, 4326))
AND ST_Contains(Polygon, ST_GeomFromText(?, 4326))
LIMIT 1;
`

	// Using the optimized query with bounding box
	findClosestMarkersQuery = `
SELECT MarkerID, 
       ST_X(Location) AS Latitude, 
       ST_Y(Location) AS Longitude, 
       Description, 
       ST_Distance_Sphere(Location, ST_GeomFromText(?, 4326)) AS distance, 
       Address
FROM Markers
WHERE MBRContains(
    ST_SRID(
        ST_GeomFromText(
            CONCAT('POLYGON((',
                   ?, ' ', ?, ',', 
                   ?, ' ', ?, ',', 
                   ?, ' ', ?, ',', 
                   ?, ' ', ?, ',', 
                   ?, ' ', ?, 
                   '))')), 
        4326), 
    Location)
  AND ST_Distance_Sphere(Location, ST_GeomFromText(?, 4326)) <= ?
ORDER BY distance ASC`

	findClosestMarkersWithThumbnailQuery = `
SELECT m.MarkerID, 
       ST_X(m.Location) AS Latitude, 
       ST_Y(m.Location) AS Longitude, 
       m.Description, 
       ST_Distance_Sphere(m.Location, ST_GeomFromText(?, 4326)) AS Distance, 
       m.Address,
	   COALESCE(p.ThumbnailURL, p.PhotoURL) AS Thumbnail
FROM Markers m
LEFT JOIN (
    SELECT p1.MarkerID, p1.PhotoURL, p1.ThumbnailURL
    FROM Photos p1
    WHERE p1.UploadedAt = (
        SELECT MAX(p2.UploadedAt) 
        FROM Photos p2 
        WHERE p1.MarkerID = p2.MarkerID
    )
) p ON m.MarkerID = p.MarkerID
WHERE MBRContains(
    ST_SRID(
        ST_GeomFromText(
            CONCAT('POLYGON((',
                   ?, ' ', ?, ',', 
                   ?, ' ', ?, ',', 
                   ?, ' ', ?, ',', 
                   ?, ' ', ?, ',', 
                   ?, ' ', ?, 
                   '))')), 
        4326), 
    Location)
AND ST_Distance_Sphere(Location, ST_GeomFromText(?, 4326)) <= ?
GROUP BY m.MarkerID
ORDER BY distance ASC
LIMIT ? OFFSET ?`
)

type PooledMarkers struct {
	Markers []dto.MarkerWithDistanceAndPhoto
	pool    *sync.Pool
}

// Method to release the markers back to the pool
func (pm *PooledMarkers) Release() {
	// Clear the slice before putting it back
	pm.Markers = pm.Markers[:0]
	pm.pool.Put(pm)
}

var markerPool = sync.Pool{
	New: func() interface{} {
		return &PooledMarkers{
			Markers: make([]dto.MarkerWithDistanceAndPhoto, 0, 100),
		}
	},
}

type MarkerLocationService struct {
	DB                   *sqlx.DB
	Config               *config.AppConfig
	KakaoConfig          *config.KakaoConfig
	Redis                *RedisService
	MapUtil              *util.MapUtil
	FacilityService      *MarkerFacilityService
	FindCloseMarkersStmt *sqlx.Stmt
}

func NewMarkerLocationService(
	db *sqlx.DB,
	config *config.AppConfig,
	kakaoConfig *config.KakaoConfig,
	redis *RedisService,
	mapUtil *util.MapUtil,
	facilityService *MarkerFacilityService,
) *MarkerLocationService {
	findCloseMarkersStmt, _ := db.Preparex(findClosestMarkersWithThumbnailQuery)
	return &MarkerLocationService{
		DB:                   db,
		Config:               config,
		KakaoConfig:          kakaoConfig,
		Redis:                redis,
		MapUtil:              mapUtil,
		FacilityService:      facilityService,
		FindCloseMarkersStmt: findCloseMarkersStmt,
	}
}

func RegisterMarkerLocationLifecycle(lifecycle fx.Lifecycle, service *MarkerLocationService) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return nil
		},
		OnStop: func(context.Context) error {
			service.FindCloseMarkersStmt.Close()
			return nil
		},
	})
}

// meters_per_degree = 40075000 / 360 / 1000
// IsMarkerNearby checks if there's a marker within n meters of the given latitude and longitude
func (s *MarkerLocationService) IsMarkerNearby(lat, long float64, bufferDistanceMeters int) (bool, error) {
	point := formatPoint(lat, long)

	// Execute the query
	var nearby bool
	err := s.DB.Get(&nearby, existNearbyMarkerQuery, point, bufferDistanceMeters)
	if err != nil {
		return false, fmt.Errorf("error checking for nearby markers: %w", err)
	}

	return nearby, nil
}

// IsMarkerNearby checks if there's a marker within n meters of the given latitude and longitude
func (s *MarkerLocationService) IsMarkerInRestrictedArea(lat, long float64) (string, bool, error) {
	point := formatPoint(lat, long)

	var name string
	err := s.DB.Get(&name, isInRestrictedAreaQuery, point, point)
	if err != nil {
		if err == sql.ErrNoRows {
			// No rows were found
			return "", false, nil
		}
		// Some other error occurred
		return "", false, fmt.Errorf("error checking for nearby markers: %w", err)
	}

	// Row was found, return the name and true
	return name, true, nil
}

// FindClosestNMarkersWithinDistance
func (s *MarkerLocationService) FindClosestNMarkersWithinDistance(lat, long float64, distance, pageSize, offset int) ([]dto.MarkerWithDistanceAndPhoto, int, error) {
	// Calculate bounding box more efficiently
	radLat := lat * util.RadiansPerDegree

	// Calculate angular distance
	radDist := float64(distance) / earthRadius

	// Calculate deltas in degrees
	deltaLat := radDist * util.RadiansToDegrees
	deltaLon := deltaLat / math.Cos(radLat)

	// Calculate bounds directly
	minLat := lat - deltaLat
	maxLat := lat + deltaLat
	minLon := long - deltaLon
	maxLon := long + deltaLon

	point := formatPoint(lat, long)

	pooledMarkers := markerPool.Get().(*PooledMarkers)

	err := s.FindCloseMarkersStmt.Select(
		&pooledMarkers.Markers,
		point,
		minLon, minLat,
		maxLon, minLat,
		maxLon, maxLat,
		minLon, maxLat,
		minLon, minLat,
		point,
		distance,
		pageSize, // LIMIT
		offset,   // OFFSET
	)
	if err != nil {
		pooledMarkers.Release()
		return nil, 0, errors.New("error fetching nearby markers")
	}

	// Attach the pool reference for later use
	pooledMarkers.pool = &markerPool

	return pooledMarkers.Markers, len(pooledMarkers.Markers), nil
}

func (s *MarkerLocationService) FindRankedMarkersInCurrentArea(lat, long float64, distance, limit int) ([]dto.MarkerWithDistanceAndPhoto, error) {
	// Predefine capacity for slices based on known limits to avoid multiple allocations
	nearbyMarkers, total, err := s.FindClosestNMarkersWithinDistance(lat, long, distance, limit, 0)
	if err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil // Return nil to signify no markers, reducing slice allocation
	}

	markerIDs := make([]string, len(nearbyMarkers))
	for i, marker := range nearbyMarkers {
		markerIDs[i] = strconv.Itoa(marker.MarkerID)
	}

	ctx := context.Background()
	floatMin := float64(MinClickRank)

	result, _ := s.Redis.Core.Client.Do(ctx, s.Redis.Core.Client.B().Zmscore().Key("marker_clicks").Member(markerIDs...).Build()).AsFloatSlice()
	rankedMarkers := make([]dto.MarkerWithDistanceAndPhoto, 0, len(result))
	for i, score := range result {
		if score > floatMin { // Include markers with score > minScore
			nearbyMarkers[i].Distance = score
			rankedMarkers = append(rankedMarkers, nearbyMarkers[i])
		}
	}

	if len(rankedMarkers) == 0 {
		return nil, nil // Return nil to signify no ranked markers
	}

	// The sorting logic remains unchanged, but it's necessary for ranking
	sort.SliceStable(rankedMarkers, func(i, j int) bool {
		return rankedMarkers[i].Distance > rankedMarkers[j].Distance
	})

	// Applying limit after sorting
	if limit > len(rankedMarkers) {
		limit = len(rankedMarkers)
	}

	return rankedMarkers[:limit], nil
}

// GoogleGeoResponse struct to parse the Google Maps Geocoding API response
type GoogleGeoResponse struct {
	Results []struct {
		Geometry struct {
			Location struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"location"`
		} `json:"geometry"`
	} `json:"results"`
	Status string `json:"status"`
}

// GeocodeAddress queries the Google Maps Geocoding API to get latitude and longitude for a given address.
func GeocodeAddress(address, apiKey string) (float64, float64, error) {
	var baseURL = "https://maps.googleapis.com/maps/api/geocode/json"

	// Prepare the request URL with query parameters
	params := url.Values{}
	params.Add("address", address)
	params.Add("key", apiKey)

	// Create an HTTP client with a timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Build the complete URL
	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// Make the request
	resp, err := client.Get(requestURL)
	if err != nil {
		return 0, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Decode the JSON response
	var geoResp GoogleGeoResponse
	if err := json.NewDecoder(resp.Body).Decode(&geoResp); err != nil {
		return 0, 0, fmt.Errorf("error decoding response: %w", err)
	}

	if geoResp.Status != "OK" || len(geoResp.Results) == 0 {
		return 0, 0, errors.New("no results found")
	}

	// Extract latitude and longitude
	lat := geoResp.Results[0].Geometry.Location.Lat
	lng := geoResp.Results[0].Geometry.Location.Lng

	return lat, lng, nil
}

func (s *MarkerLocationService) SaveOfflineMap(lat, lng float64) (string, error) {
	if !s.MapUtil.IsInSouthKoreaPrecisely(lat, lng) {
		return "", errors.New("only allowed in South Korea")
	}

	// 0. Get Address of lat/lng
	address, _ := s.FacilityService.FetchAddressFromAPI(lat, lng)
	if address == "-2" {
		return "", errors.New("address not found")
	}
	if address == "-1" {
		address = "대한민국 철봉 지도"
	}

	// 1. Convert them into WCONGNAMUL
	mapWcon := util.ConvertWGS84ToWCONGNAMUL(lat, lng)

	// 2. Get the static map image (base_map_blah.png)
	// temporarily download from fmt.Sprintf("%s&MX=%f%MY=%f", KAKAO_STATIC, map_wcon.X, map_wcon.Y)
	tempDir, err := os.MkdirTemp("", "chulbongkr-*") // Use "" for the system default temp directory
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory")
	}
	// defer os.RemoveAll(tempDir)

	baseImageFile := fmt.Sprintf("base_map-%s.png", xid.New().String())
	baseImageFilePath := path.Join(tempDir, baseImageFile)
	util.DownloadFile(fmt.Sprintf("%s&MX=%f&MY=%f", s.KakaoConfig.KakaoStaticMap, mapWcon.X, mapWcon.Y), baseImageFilePath)

	// 3. Load all close markers nearby map lat/lng
	// Predefine capacity for slices based on known limits to avoid multiple allocations
	// 1280*720 500m
	nearbyMarkers, total, err := s.FindClosestNMarkersWithinDistance(lat, lng, 500, 15, 0) // meter, pageSize, offset
	if err != nil {
		return "", errors.New("failed to find nearby markers")
	}
	if total == 0 {
		return "", nil // Return nil to signify no markers in the area, reducing slice allocation
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(nearbyMarkers))

	// temporarily download each
	tempImagePath := path.Join(tempDir, "marker_images")

	os.Mkdir(tempImagePath, os.FileMode(0755))

	for i, marker := range nearbyMarkers {
		wg.Add(1)
		go func(i int, marker dto.MarkerWithDistanceAndPhoto) {
			defer wg.Done()
			markerWcon := util.ConvertWGS84ToWCONGNAMUL(marker.Latitude, marker.Longitude)
			markerFile := path.Join(tempImagePath, fmt.Sprintf("marker-%d.png", i))
			err := util.DownloadFile(fmt.Sprintf("%s&MX=%f&MY=%f&CX=%f&CY=%f", s.KakaoConfig.KakaoStaticMap, mapWcon.X, mapWcon.Y, markerWcon.X, markerWcon.Y), markerFile)
			if err != nil {
				errs <- fmt.Errorf("failed to download marker %d: %w", i, err)
				return
			}
		}(i, marker)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return "", err
		}
	}

	// there will be len(nearbyMarkers) + 1 (base map) png files. but doesn't matter if some missing

	// 4. Overlay them
	resultImagePath, err := util.OverlayImages(baseImageFilePath, tempImagePath)
	if err != nil {
		return "", errors.New("failed to overlay images")
	}

	// 5. Make PDF
	downloadPath, err := util.GenerateMapPDF(resultImagePath, tempDir, address, nearbyMarkers[0].MarkerID)
	if err != nil {
		return "", fmt.Errorf("failed to make pdf file: " + err.Error())
	}

	// Schedule cleanup for 5mins later
	// go func() {
	// 	time.Sleep(5 * time.Minute)
	// 	os.RemoveAll(tempDir)
	// }()

	return downloadPath, nil
}

// SaveOfflineMap2 draws markers with go rather than download images
func (s *MarkerLocationService) SaveOfflineMap2(lat, lng float64) (string, string, error) {
	if !s.MapUtil.IsInSouthKoreaPrecisely(lat, lng) {
		return "", "", errors.New("only allowed in South Korea")
	}

	// 0. Fetch address - no formatting needed if simple
	address, err := s.FacilityService.FetchAddressFromAPI(lat, lng)
	if err != nil {
		return "", "", err
	}
	if address == "-2" {
		return "", "", errors.New("address not found")
	}
	if address == "-1" {
		// Use a default address if API error occurred
		address = "대한민국 철봉 지도"
	}

	// 1. Convert them into WCONGNAMUL
	mapWcon := util.ConvertWGS84ToWCONGNAMUL(lat, lng)

	// 2. Get the static map image (base_map_blah.png)
	// temporarily download from fmt.Sprintf("%s&MX=%f%MY=%f", KAKAO_STATIC, map_wcon.X, map_wcon.Y)
	tempDir, err := os.MkdirTemp("", "chulbongkr-*") // Use "" for the system default temp directory
	if err != nil {
		return "", "", errors.New("failed to create temp directory")
	}
	// defer os.RemoveAll(tempDir)

	// Build URL using strconv for performance
	// Example: s.KakaoConfig.KakaoStaticMap + "&MX=" + strconv.FormatFloat(mapWcon.X,'f',6,64) + "&MY=" ...
	var urlBuilder strings.Builder
	urlBuilder.Grow(len(s.KakaoConfig.KakaoStaticMap) + 50) // pre-allocate capacity
	urlBuilder.WriteString(s.KakaoConfig.KakaoStaticMap)
	urlBuilder.WriteString("&MX=")
	urlBuilder.WriteString(strconv.FormatFloat(mapWcon.X, 'f', 6, 64))
	urlBuilder.WriteString("&MY=")
	urlBuilder.WriteString(strconv.FormatFloat(mapWcon.Y, 'f', 6, 64))
	baseMapURL := urlBuilder.String()

	// Generate a short random name instead of full UUID to save cycles and memory
	// (You could also use a global buffer or a simple counter)
	baseImageFile := "base_map-" + xid.New().String() + ".png"
	baseImageFilePath := path.Join(tempDir, baseImageFile)

	util.DownloadFile(baseMapURL, baseImageFilePath)

	// 3. Load all close markers nearby map lat/lng
	// Predefine capacity for slices based on known limits to avoid multiple allocations
	// 1280*720 500m
	nearbyMarkers, total, err := s.FindClosestNMarkersWithinDistance(lat, lng, 700, 30, 0) // meter, pageSize, offset
	if err != nil {
		return "", "", errors.New("failed to find nearby markers")
	}
	if total == 0 {
		// TODO: consider returning just base image file as pdf
		return "", "", nil // Return nil to signify no markers in the area, reducing slice allocation
	}

	// Convert coordinates in-place
	markers := make([]util.WCONGNAMULCoord, 0, total)
	for i := range nearbyMarkers {
		coord := util.ConvertWGS84ToWCONGNAMUL(nearbyMarkers[i].Latitude, nearbyMarkers[i].Longitude)
		markers = append(markers, coord)
	}

	// 4. Place them
	resultImagePath, err := util.PlaceMarkersOnImage(baseImageFilePath, markers, mapWcon.X, mapWcon.Y)
	if err != nil {
		return "", "", errors.New("failed to overlay images")
	}

	os.Remove(baseImageFilePath) // Remove base image file

	// 5. Make PDF
	downloadPath, err := util.GenerateMapPDF(resultImagePath, tempDir, address, nearbyMarkers[0].MarkerID)
	if err != nil {
		return "", "", fmt.Errorf("failed to make pdf file: %w", err)
	}

	return downloadPath, tempDir, nil
}

// Simple pagination helper function
func paginateMarkers(markers []dto.MarkerWithDistanceAndPhoto, pageSize, offset int) []dto.MarkerWithDistanceAndPhoto {
	if offset >= len(markers) {
		// Calculate the starting index of the last possible page
		lastPageOffset := len(markers) - pageSize
		if lastPageOffset < 0 {
			lastPageOffset = 0
		}
		return markers[lastPageOffset:]
	}
	end := offset + pageSize
	if end > len(markers) {
		end = len(markers)
	}
	return markers[offset:end]
}

func (s *MarkerLocationService) TestDynamic(latitude, longitude, zoomScale float64, width, height int64) {
	nearbyMarkers, total, err := s.FindClosestNMarkersWithinDistance(latitude, longitude, 700, 30, 0) // meter, pageSize, offset
	if err != nil {
		return
	}
	if total == 0 {
		return
	}

	markers := make([]util.WCONGNAMULCoord, len(nearbyMarkers))
	for i, marker := range nearbyMarkers {
		markers[i] = util.ConvertWGS84ToWCONGNAMUL(marker.Latitude, marker.Longitude)
	}
	mapWcon := util.ConvertWGS84ToWCONGNAMUL(latitude, longitude)
	baseImageFile := fmt.Sprintf("base_map-%s.png", xid.New().String())
	baseImageFilePath := path.Join("./tests", baseImageFile)

	static := fmt.Sprintf("https://spi.maps.daum.net/map2/map/imageservice?IW=%d&IH=%d&SCALE=%f&service=open", width, height, zoomScale)
	util.DownloadFile(fmt.Sprintf("%s&MX=%f&MY=%f", static, mapWcon.X, mapWcon.Y), baseImageFilePath)

	resultImagePath, _ := util.PlaceMarkersOnImageDynamic(baseImageFilePath, markers, mapWcon.X, mapWcon.Y, zoomScale)
	fmt.Println(resultImagePath)
}

func formatPointOld(lat, long float64) string {
	var sb strings.Builder
	sb.WriteString("POINT(")
	sb.WriteString(strconv.FormatFloat(lat, 'f', 6, 64))
	sb.WriteString(" ")
	sb.WriteString(strconv.FormatFloat(long, 'f', 6, 64))
	sb.WriteString(")")
	return sb.String()
}

/*
Execution Time:
The optimized function is ~33% faster than the original implementation, reducing processing time from ~2565 ns/op to ~1728 ns/op.

Memory Usage:
The optimized version uses ~69% less memory per call (520 B reduced to 160 B).
This is significant in memory-intensive or high-throughput scenarios.

Allocations:
The optimized version performs 80% fewer allocations (25 reduced to 5), which will reduce pressure on the garbage collector (GC), improving performance in scenarios with frequent calls.
*/
func formatPoint(lat, long float64) string {
	// Pre-allocate enough space
	// "POINT(" + lat + " " + long + ")"
	// worst case ~ "POINT(-180.000000 -180.000000)" fits well in 64 bytes.
	var buf [64]byte
	b := buf[:0]

	b = append(b, "POINT("...)
	b = strconv.AppendFloat(b, lat, 'f', 6, 64)
	b = append(b, ' ')
	b = strconv.AppendFloat(b, long, 'f', 6, 64)
	b = append(b, ')')

	return string(b)
}
