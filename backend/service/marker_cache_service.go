package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/dto/kakao"
	"github.com/Alfex4936/chulbong-kr/model"
	sonic "github.com/bytedance/sonic"
	gocache "github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
	"github.com/redis/rueidis"
	"go.uber.org/fx"
	"golang.org/x/sync/singleflight"
)

// control redis cache related to markers

const (
	cacheOpTimeout    = 3 * time.Second
	markersL1TTL      = 5 * time.Minute
	markersL2TTL      = 12 * time.Hour
	markersVersionKey = "markers:ver"
	facilityTTL       = 12 * time.Hour
	userMarkersTTL    = 6 * time.Hour
	favoritesTTL      = 12 * time.Hour
	userProfileTTL    = 3 * time.Hour
	kakaoCacheTTL     = time.Hour
)

var errCacheMiss = errors.New("cache miss")

type MarkerCacheService struct {
	MarkerWeatherCache *gocache.Cache[[]byte]
	RedisService       *RedisService

	LocalCacheStorage *ristretto_store.RistrettoStore
	l1Cache           *gocache.Cache[[]byte]
	sfGroup           singleflight.Group
}

func NewMarkerCacheService(

	localCacheStorage *ristretto_store.RistrettoStore,
	redisService *RedisService,
) *MarkerCacheService {
	weatherCache := gocache.New[[]byte](localCacheStorage)
	l1Cache := gocache.New[[]byte](localCacheStorage)

	return &MarkerCacheService{
		RedisService:       redisService,
		MarkerWeatherCache: weatherCache,
		LocalCacheStorage:  localCacheStorage,
		l1Cache:            l1Cache,
	}
}

func RegisterMarkerCacheService(lifecycle fx.Lifecycle, service *MarkerCacheService) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return nil
		},
		OnStop: func(context.Context) error {
			return nil
		},
	})
}

// ----------------------------------------------------------------
// func

func markersAllKey(version uint64) string {
	return fmt.Sprintf("markers:all:v%d", version)
}

func (s *MarkerCacheService) withCacheTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), cacheOpTimeout)
	}
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, cacheOpTimeout)
}

func (s *MarkerCacheService) getMarkersVersion(ctx context.Context) (uint64, error) {
	newGetCmd := func() rueidis.Completed {
		return s.RedisService.Core.Client.B().Get().Key(markersVersionKey).Build()
	}

	if val, err := s.RedisService.Core.Client.Do(ctx, newGetCmd()).AsInt64(); err == nil && val > 0 {
		return uint64(val), nil
	}

	seed := time.Now().Unix()
	seedStr := strconv.FormatInt(seed, 10)
	setCmd := s.RedisService.Core.Client.B().Set().Key(markersVersionKey).Value(rueidis.BinaryString([]byte(seedStr))).Nx().Build()
	_ = s.RedisService.Core.Client.Do(ctx, setCmd).Error()

	val, err := s.RedisService.Core.Client.Do(ctx, newGetCmd()).AsInt64()
	if err != nil {
		return 0, err
	}
	if val <= 0 {
		return 0, fmt.Errorf("invalid markers version %d", val)
	}
	return uint64(val), nil
}

func (s *MarkerCacheService) bumpMarkersVersion(ctx context.Context) (uint64, error) {
	incr := s.RedisService.Core.Client.B().Incr().Key(markersVersionKey).Build()
	newVer, err := s.RedisService.Core.Client.Do(ctx, incr).AsInt64()
	if err != nil {
		return 0, err
	}
	return uint64(newVer), nil
}

func (s *MarkerCacheService) setL1(ctx context.Context, key string, value []byte) error {
	if len(value) == 0 {
		return nil
	}
	return s.l1Cache.Set(ctx, key, value, store.WithExpiration(markersL1TTL))
}

func (s *MarkerCacheService) setAllMarkers(ctx context.Context, version uint64, cacheKey string, data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		return fmt.Errorf("attempted to cache empty or null markers data")
	}

	setCmd := s.RedisService.Core.Client.B().Set().Key(cacheKey).Value(rueidis.BinaryString(data)).Ex(markersL2TTL).Build()
	if err := s.RedisService.Core.Client.Do(ctx, setCmd).Error(); err != nil {
		return err
	}
	_ = s.setL1(ctx, cacheKey, data)
	return nil
}

func (s *MarkerCacheService) getBytesWithL1(ctx context.Context, key string) ([]byte, bool, error) {
	ctx, cancel := s.withCacheTimeout(ctx)
	defer cancel()

	if cached, err := s.l1Cache.Get(ctx, key); err == nil && len(cached) > 0 {
		return cached, true, nil
	}

	getCmd := s.RedisService.Core.Client.B().Get().Key(key).Build()
	val, err := s.RedisService.Core.Client.Do(ctx, getCmd).AsBytes()
	if err != nil {
		return nil, false, err
	}
	if len(val) == 0 {
		return nil, false, errCacheMiss
	}
	_ = s.setL1(ctx, key, val)
	return val, false, nil
}

func (s *MarkerCacheService) setBytes(ctx context.Context, key string, ttl time.Duration, value []byte) error {
	if len(value) == 0 {
		return nil
	}

	ctx, cancel := s.withCacheTimeout(ctx)
	defer cancel()

	setCmd := s.RedisService.Core.Client.B().Set().Key(key).Value(rueidis.BinaryString(value)).Ex(ttl).Build()
	if err := s.RedisService.Core.Client.Do(ctx, setCmd).Error(); err != nil {
		return err
	}
	_ = s.setL1(ctx, key, value)
	return nil
}

// GetOrLoadAllMarkers returns cached markers if present or uses loader to hydrate caches.
// It returns the data, a flag indicating whether it was served from cache, and an error.
func (s *MarkerCacheService) GetOrLoadAllMarkers(ctx context.Context, loader func(context.Context) ([]byte, error)) ([]byte, bool, error) {
	ctx, cancel := s.withCacheTimeout(ctx)
	defer cancel()

	version, err := s.getMarkersVersion(ctx)
	if err != nil {
		return nil, false, err
	}

	cacheKey := markersAllKey(version)

	if cached, err := s.l1Cache.Get(ctx, cacheKey); err == nil && len(cached) > 0 {
		return cached, true, nil
	}

	type result struct {
		data      []byte
		fromCache bool
	}

	resAny, loadErr, _ := s.sfGroup.Do(cacheKey, func() (interface{}, error) {
		cacheCtx, cacheCancel := s.withCacheTimeout(ctx)
		defer cacheCancel()

		if cached, err := s.RedisService.Core.Client.Do(cacheCtx, s.RedisService.Core.Client.B().Get().Key(cacheKey).Build()).AsBytes(); err == nil && len(cached) > 0 && string(cached) != "null" {
			_ = s.setL1(cacheCtx, cacheKey, cached)
			return result{data: cached, fromCache: true}, nil
		}

		if loader == nil {
			return nil, errCacheMiss
		}

		fresh, err := loader(ctx)
		if err != nil {
			return nil, err
		}

		if err := s.setAllMarkers(ctx, version, cacheKey, fresh); err != nil {
			return nil, err
		}

		return result{data: fresh, fromCache: false}, nil
	})

	if loadErr != nil {
		return nil, false, loadErr
	}

	res := resAny.(result)
	return res.data, res.fromCache, nil
}

// GetAllMarkers is kept for backward-compatibility; it returns cached data if present.
func (s *MarkerCacheService) GetAllMarkers() ([]byte, error) {
	data, _, err := s.GetOrLoadAllMarkers(context.Background(), nil)
	return data, err
}

// SetFullMarkersCache stores the full marker list under a fresh version (or bumps version if data is empty).
func (s *MarkerCacheService) SetFullMarkersCache(markersJSON []byte) error {
	ctx, cancel := s.withCacheTimeout(context.Background())
	defer cancel()

	if len(markersJSON) == 0 || string(markersJSON) == "null" {
		_, err := s.bumpMarkersVersion(ctx)
		return err
	}

	newVer, err := s.bumpMarkersVersion(ctx)
	if err != nil {
		return err
	}

	return s.setAllMarkers(ctx, newVer, markersAllKey(newVer), markersJSON)
}

// Invalidate full cache by bumping version so readers move to a fresh key.
func (s *MarkerCacheService) InvalidateFullMarkersCache() error {
	ctx, cancel := s.withCacheTimeout(context.Background())
	defer cancel()

	_, err := s.bumpMarkersVersion(ctx)
	return err
}

// Set individual marker in Redis
func (s *MarkerCacheService) SetMarkerCache(markerID int, marker dto.MarkerSimple) error {
	markerJSON, err := sonic.Marshal(marker)
	if err != nil {
		return err
	}

	ctx := context.Background()
	setCmd := s.RedisService.Core.Client.B().Set().Key(fmt.Sprintf("marker:%d", markerID)).Value(rueidis.BinaryString(markerJSON)).Nx().Ex(time.Hour * 24).Build()
	return s.RedisService.Core.Client.Do(ctx, setCmd).Error()
}

// Remove an individual marker from cache
func (s *MarkerCacheService) RemoveMarkerCache(markerID int) error {
	return s.RedisService.ResetCache(fmt.Sprintf("marker:%d", markerID))
}

func (s *MarkerCacheService) AddMarker(markerID int, marker dto.MarkerSimple) error {
	// Cache the individual marker
	if err := s.SetMarkerCache(markerID, marker); err != nil {
		return err
	}

	// Add the marker ID to the Redis set
	if err := s.AddMarkerIDToSet(markerID); err != nil {
		return err
	}

	return nil
}

func (s *MarkerCacheService) UpdateMarker(markerID int, marker dto.MarkerSimple) error {
	// Update the individual marker cache
	if err := s.SetMarkerCache(markerID, marker); err != nil {
		return err
	}

	// Invalidate the full markers cache
	return s.InvalidateFullMarkersCache()
}

func (s *MarkerCacheService) RemoveMarker(markerID int) {
	// Remove the individual marker cache
	s.RemoveMarkerCache(markerID)

	// Remove the marker ID from the Redis set
	s.RemoveMarkerIDFromSet(markerID)

	// Invalidate the full markers cache
	s.InvalidateFullMarkersCache()
}

// AddMarkerIDToSet adds a marker ID to the Redis set "all_markers_set"
func (s *MarkerCacheService) AddMarkerIDToSet(markerID int) error {
	ctx := context.Background()
	addCmd := s.RedisService.Core.Client.B().Sadd().Key("all_markers_set").Member(fmt.Sprintf("%d", markerID)).Build()
	return s.RedisService.Core.Client.Do(ctx, addCmd).Error()
}

// RemoveMarkerIDFromSet removes a marker ID from the Redis set "all_markers_set"
func (s *MarkerCacheService) RemoveMarkerIDFromSet(markerID int) error {
	ctx := context.Background()
	removeCmd := s.RedisService.Core.Client.B().Srem().Key("all_markers_set").Member(fmt.Sprintf("%d", markerID)).Build()
	return s.RedisService.Core.Client.Do(ctx, removeCmd).Error()
}

// Retrieve marker IDs from Redis set
func (s *MarkerCacheService) GetAllMarkerIDs() ([]string, error) {
	ctx := context.Background()
	markerIDsCmd := s.RedisService.Core.Client.B().Smembers().Key("all_markers_set").Build()
	markerIDs, err := s.RedisService.Core.Client.Do(ctx, markerIDsCmd).AsStrSlice()
	if err != nil {
		return nil, err
	}
	return markerIDs, nil
}

// user_fav
// AddMarkerToFavorites adds a marker to the user's favorites cache
func (s *MarkerCacheService) AddMarkerToFavorites(userID int, marker dto.MarkerSimpleWithDescription) error {
	// Add the marker ID to the user's favorite set
	err := s.RedisService.AddToSet(s.userFavSetKey(userID), strconv.Itoa(marker.MarkerID))
	if err != nil {
		return err
	}

	// Cache the individual marker as part of the favorites
	markerJSON, err := sonic.Marshal(marker)
	if err != nil {
		return err
	}
	return s.setBytes(context.Background(), s.userFavMarkerKey(userID, marker.MarkerID), favoritesTTL, markerJSON)
}

// AddFavoritesToCache adds all favorites to the user's cache concurrently
func (s *MarkerCacheService) AddFavoritesToCache(userID int, favorites []dto.MarkerSimpleWithDescription) error {
	// Prepare to concurrently add marker IDs to the user's favorite set
	var wg sync.WaitGroup
	errChan := make(chan error, len(favorites))

	// Concurrently cache marker IDs and marker data
	for _, fav := range favorites {
		wg.Add(1)
		go func(fav dto.MarkerSimpleWithDescription) {
			defer wg.Done()

			// Add marker ID to the user's favorite set
			err := s.RedisService.AddToSet(s.userFavSetKey(userID), strconv.Itoa(fav.MarkerID))
			if err != nil {
				errChan <- err
				return
			}

			// Cache the individual marker as part of the favorites
			markerJSON, err := sonic.Marshal(fav)
			if err != nil {
				errChan <- err
				return
			}
			err = s.setBytes(context.Background(), s.userFavMarkerKey(userID, fav.MarkerID), favoritesTTL, markerJSON)
			if err != nil {
				errChan <- err
			}
		}(fav)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Return the first error encountered, if any
	if len(errChan) > 0 {
		return <-errChan
	}
	return nil
}

func (s *MarkerCacheService) AddSingleFavoriteToCache(userID int, fav dto.MarkerSimpleWithDescription) error {
	// Add marker ID to set
	if err := s.RedisService.AddToSet(s.userFavSetKey(userID), strconv.Itoa(fav.MarkerID)); err != nil {
		return fmt.Errorf("failed to add to set: %w", err)
	}

	// Cache marker details
	markerJSON, err := sonic.Marshal(fav)
	if err != nil {
		return fmt.Errorf("failed to marshal marker data: %w", err)
	}

	if err := s.setBytes(context.Background(), s.userFavMarkerKey(userID, fav.MarkerID), favoritesTTL, markerJSON); err != nil {
		return fmt.Errorf("failed to set cache entry: %w", err)
	}

	return nil
}

// GetUserFavorites retrieves the list of a user's favorite markers from the cache
func (s *MarkerCacheService) GetUserFavorites(userID int) ([]dto.MarkerSimpleWithDescription, error) {
	// Retrieve the list of favorite marker IDs from Redis set
	markerIDs, err := s.RedisService.GetMembersOfSet(s.userFavSetKey(userID))
	if err != nil {
		return nil, err
	}

	var favorites []dto.MarkerSimpleWithDescription

	// Retrieve the individual markers from cache
	for _, markerID := range markerIDs {
		data, _, err := s.getBytesWithL1(context.Background(), s.userFavMarkerKey(userID, atoiSafe(markerID)))
		if err != nil || len(data) == 0 {
			continue // Skip any missing or invalid cache entries
		}
		var marker dto.MarkerSimpleWithDescription
		if err := sonic.Unmarshal(data, &marker); err == nil {
			favorites = append(favorites, marker)
		}
	}

	return favorites, nil
}

func (s *MarkerCacheService) RemoveMarkerFromFavorites(userID int, markerID int) {
	// Remove the specific marker from the user's favorite list
	ctx, cancel := s.withCacheTimeout(context.Background())
	defer cancel()

	remCmd := s.RedisService.Core.Client.B().Srem().Key(s.userFavSetKey(userID)).Member(strconv.Itoa(markerID)).Build()
	_ = s.RedisService.Core.Client.Do(ctx, remCmd).Error()
	delCmd := s.RedisService.Core.Client.B().Del().Key(s.userFavMarkerKey(userID, markerID)).Build()
	_ = s.RedisService.Core.Client.Do(ctx, delCmd).Error()
}

// facilities
// AddFacilitiesCache adds facilities data for a specific marker to the cache
func (s *MarkerCacheService) AddFacilitiesCache(markerID int, facilities []model.Facility) error {
	// Cache the facilities data
	facilitiesJSON, err := sonic.Marshal(facilities)
	if err != nil {
		return err
	}
	return s.RedisService.SetCacheEntry(fmt.Sprintf("facilities:%d", markerID), facilitiesJSON, time.Hour*24)
}

// GetFacilitiesCache retrieves the facilities data for a specific marker from the cache
func (s *MarkerCacheService) GetFacilitiesCache(markerID int) (*[]model.Facility, error) {
	var facilitiesData []byte
	err := s.RedisService.GetCacheEntry(fmt.Sprintf("facilities:%d", markerID), &facilitiesData)
	if err != nil || len(facilitiesData) == 0 {
		return nil, err
	}

	var facilities []model.Facility
	if err := sonic.Unmarshal(facilitiesData, &facilities); err != nil {
		return nil, err
	}

	return &facilities, nil
}

func (s *MarkerCacheService) InvalidateFacilities(markerID int) {
	s.RedisService.ResetCache(fmt.Sprintf("facilities:%d", markerID))
}

// user_markers

// AddUserMarkersPageCache caches a page of markers the user has created
func (s *MarkerCacheService) AddUserMarkersPageCache(userID int, page int, markers []dto.MarkerSimpleWithDescription) error {
	// Cache the list of markers on a specific page for the user
	markersJSON, err := sonic.Marshal(markers)
	if err != nil {
		return err
	}
	return s.RedisService.SetCacheEntry(fmt.Sprintf("user_markers:%d:page:%d", userID, page), markersJSON, time.Hour*24)
}

// GetUserMarkersPageCache retrieves a page of markers created by the user from the cache
func (s *MarkerCacheService) GetUserMarkersPageCache(userID int, page int) ([]dto.MarkerSimpleWithDescription, error) {
	var markersData []byte
	err := s.RedisService.GetCacheEntry(fmt.Sprintf("user_markers:%d:page:%d", userID, page), &markersData)
	if err != nil || len(markersData) == 0 {
		return nil, err
	}

	var markers []dto.MarkerSimpleWithDescription
	if err := sonic.Unmarshal(markersData, &markers); err != nil {
		return nil, err
	}

	return markers, nil
}

func (s *MarkerCacheService) RemoveUserMarker(userID, markerID int) {
	s.RedisService.ResetAllCache(userMarkersPattern(userID)) // TODO: Only invalidate the affected page if possible
}

// user_profile
// GetUserProfileCache retrieves the cached user profile from Redis as byte data.
func (s *MarkerCacheService) GetUserProfileCache(userID int) ([]byte, error) {
	// Construct the Redis key for the user profile
	userProfileKey := fmt.Sprintf("user_profile:%d", userID)

	// Retrieve the cached byte data from Redis
	data, _, err := s.getBytesWithL1(context.Background(), userProfileKey)
	if err != nil || len(data) == 0 {
		return nil, err // Cache miss or error
	}

	return data, nil
}

// SetUserProfileCache caches the user profile as byte data in Redis with a specified TTL.
func (s *MarkerCacheService) SetUserProfileCache(userID int, userProfileData []byte) error {
	// Construct the Redis key for the user profile
	userProfileKey := fmt.Sprintf("user_profile:%d", userID)

	// Set the cache entry in Redis with the specified TTL
	return s.setBytes(context.Background(), userProfileKey, userProfileTTL, userProfileData)
}

// ResetUserProfileCache invalidates the user profile cache.
func (s *MarkerCacheService) ResetUserProfileCache(userID int) error {
	// Construct the Redis key for the user profile
	userProfileKey := fmt.Sprintf("user_profile:%d", userID)

	// Remove the cached user profile from Redis
	return s.RedisService.ResetCache(userProfileKey)
}

// --
func (s *MarkerCacheService) InvalidateAllMarkersCache(markerID, userID int, username string) {
	_ = s.InvalidateFullMarkersCache()
	// user added markers
	s.RedisService.ResetAllCache(userMarkersPattern(userID))

	// facilities
	s.RedisService.ResetCache(facilityKey(markerID))

	// user fav
	s.RedisService.ResetCache(s.userFavSetKey(userID))
	s.RedisService.ResetCache(s.userFavMarkerKey(userID, markerID))
	// legacy username-based key
	s.RedisService.ResetCache(fmt.Sprintf("%s:%d:%s", s.RedisService.RedisConfig.UserFavKey, userID, username))

}

func (s *MarkerCacheService) SetWcongCache(latitude, longitude float64, coord *kakao.WeatherRequest) {
	key := generateCacheKey(latitude, longitude)
	data, err := sonic.Marshal(coord)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.MarkerWeatherCache.Set(ctx, key, data, store.WithExpiration(time.Minute*15))
}

func (s *MarkerCacheService) GetWcongCache(latitude, longitude float64) (*kakao.WeatherRequest, error) {
	key := generateCacheKey(latitude, longitude)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	value, err := s.MarkerWeatherCache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil // cache miss
	}

	var coord *kakao.WeatherRequest
	err = sonic.Unmarshal(value, &coord)
	if err != nil {
		return nil, err
	}

	return coord, nil
}

// close
// GetUserProfileCache retrieves the cached user profile from Redis as byte data.
func (s *MarkerCacheService) GetCloseMarkersCache(cacheKey string) ([]byte, error) {
	data, _, err := s.getBytesWithL1(context.Background(), cacheKey)
	return data, err
}

// SetCloseMarkersCache caches the close markers response in Redis with a specified TTL
func (s *MarkerCacheService) SetCloseMarkersCache(cacheKey string, data []byte, ttl time.Duration) error {
	return s.setBytes(context.Background(), cacheKey, ttl, data)
}

// kakaochat bot
func (s *MarkerCacheService) GetKakaoRecentMarkersCache(response interface{}) error {
	return s.RedisService.GetCacheEntry(s.RedisService.RedisConfig.KakaoRecentMarkersKey, response)
}

func (s *MarkerCacheService) SetKakaoRecentMarkersCache(json interface{}) error {
	return s.RedisService.SetCacheEntry(s.RedisService.RedisConfig.KakaoRecentMarkersKey, json, 1*time.Hour)
}

func (s *MarkerCacheService) GetKakaoMarkerSearchCache(utterance string, obj interface{}) error {
	return s.RedisService.GetCacheEntry(s.RedisService.RedisConfig.KakaoSearchMarkersKey+utterance, obj)
}

func (s *MarkerCacheService) SetKakaoMarkerSearchCache(utterance string, json interface{}) {
	s.RedisService.SetCacheEntry(s.RedisService.RedisConfig.KakaoSearchMarkersKey+utterance, json, 1*time.Hour)
}

// func GetStoryCacheKey(markerID int, page int) string {
//     return fmt.Sprintf("stories:%d:page:%d", markerID, page)
// }

// func GetStoryCachePattern(markerID int) string {
//     return fmt.Sprintf("stories:%d:*", markerID)
// }

// HELPERS

func atoiSafe(value string) int {
	v, _ := strconv.Atoi(value)
	return v
}

func facilityKey(markerID int) string {
	return fmt.Sprintf("facilities:%d", markerID)
}

func userMarkersPageKey(userID int, page int) string {
	return fmt.Sprintf("user_markers:%d:page:%d", userID, page)
}

func userMarkersPattern(userID int) string {
	return fmt.Sprintf("user_markers:%d:page:*", userID)
}

func (s *MarkerCacheService) userFavSetKey(userID int) string {
	return fmt.Sprintf("%s:%d", s.RedisService.RedisConfig.UserFavKey, userID)
}

func (s *MarkerCacheService) userFavMarkerKey(userID int, markerID int) string {
	return fmt.Sprintf("%s_marker:%d:%d", s.RedisService.RedisConfig.UserFavKey, userID, markerID)
}

// generateCacheKey generates a unique cache key based on latitude and longitude.
func generateCacheKey(latitude, longitude float64) string {
	return fmt.Sprintf("wcong:%f:%f", latitude, longitude)
}
