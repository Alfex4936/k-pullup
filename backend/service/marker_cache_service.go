package service

import (
	"context"
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
)

// control redis cache related to markers

type MarkerCacheService struct {
	MarkerWeatherCache *gocache.Cache[[]byte]
	RedisService       *RedisService

	LocalCacheStorage *ristretto_store.RistrettoStore
}

func NewMarkerCacheService(

	localCacheStorage *ristretto_store.RistrettoStore,
	redisService *RedisService,
) *MarkerCacheService {
	byteCache := gocache.New[[]byte](localCacheStorage)

	return &MarkerCacheService{
		RedisService:       redisService,
		MarkerWeatherCache: byteCache,
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

func (s *MarkerCacheService) GetAllMarkers() ([]byte, error) {
	// Retrieve the cached markers from Redis as a byte array
	ctx := context.Background()
	getCmd := s.RedisService.Core.Client.B().Get().Key("all_markers").Build()

	result, err := s.RedisService.Core.Client.Do(ctx, getCmd).AsBytes()
	if err != nil {
		return nil, err // Cache miss or error
	}
	
	// Check for null or empty values
	if len(result) == 0 || string(result) == "null" {
		// Invalidate this problematic cache entry
		s.InvalidateFullMarkersCache()
		return nil, fmt.Errorf("invalid cache data")
	}
	
	return result, nil
}

// Set the full cache (all markers as a byte array)
func (s *MarkerCacheService) SetFullMarkersCache(markersJSON []byte) error {
	// Validate that we're not storing empty or null data
	if len(markersJSON) == 0 || string(markersJSON) == "null" {
		return fmt.Errorf("attempted to cache empty or null markers data")
	}

	ctx := context.Background()
	setCmd := s.RedisService.Core.Client.B().Set().Key("all_markers").Value(rueidis.BinaryString(markersJSON)).Ex(time.Hour * 24).Build()
	return s.RedisService.Core.Client.Do(ctx, setCmd).Error()
}

// Invalidate full cache
func (s *MarkerCacheService) InvalidateFullMarkersCache() error {
	return s.RedisService.ResetCache("all_markers")
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
	err := s.RedisService.AddToSet(fmt.Sprintf("user_fav:%d", userID), strconv.Itoa(marker.MarkerID))
	if err != nil {
		return err
	}

	// Cache the individual marker as part of the favorites
	markerJSON, err := sonic.Marshal(marker)
	if err != nil {
		return err
	}
	return s.RedisService.SetCacheEntry(fmt.Sprintf("user_fav_marker:%d:%d", userID, marker.MarkerID), markerJSON, time.Hour*24)
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
			err := s.RedisService.AddToSet(fmt.Sprintf("user_fav:%d", userID), strconv.Itoa(fav.MarkerID))
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
			err = s.RedisService.SetCacheEntry(fmt.Sprintf("user_fav_marker:%d:%d", userID, fav.MarkerID), markerJSON, time.Hour*24)
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
	if err := s.RedisService.AddToSet(fmt.Sprintf("user_fav:%d", userID), strconv.Itoa(fav.MarkerID)); err != nil {
		return fmt.Errorf("failed to add to set: %w", err)
	}

	// Cache marker details
	markerJSON, err := sonic.Marshal(fav)
	if err != nil {
		return fmt.Errorf("failed to marshal marker data: %w", err)
	}

	if err := s.RedisService.SetCacheEntry(fmt.Sprintf("user_fav_marker:%d:%d", userID, fav.MarkerID), markerJSON, 24*time.Hour); err != nil {
		return fmt.Errorf("failed to set cache entry: %w", err)
	}

	return nil
}

// GetUserFavorites retrieves the list of a user's favorite markers from the cache
func (s *MarkerCacheService) GetUserFavorites(userID int) ([]dto.MarkerSimpleWithDescription, error) {
	// Retrieve the list of favorite marker IDs from Redis set
	markerIDs, err := s.RedisService.GetMembersOfSet(fmt.Sprintf("user_fav:%d", userID))
	if err != nil {
		return nil, err
	}

	var favorites []dto.MarkerSimpleWithDescription

	// Retrieve the individual markers from cache
	for _, markerID := range markerIDs {
		var markerData []byte
		err := s.RedisService.GetCacheEntry(fmt.Sprintf("user_fav_marker:%d:%s", userID, markerID), &markerData)
		if err != nil || len(markerData) == 0 {
			continue // Skip any missing or invalid cache entries
		}
		var marker dto.MarkerSimpleWithDescription
		if err := sonic.Unmarshal(markerData, &marker); err == nil {
			favorites = append(favorites, marker)
		}
	}

	return favorites, nil
}

func (s *MarkerCacheService) RemoveMarkerFromFavorites(userID int, markerID int) {
	// Remove the specific marker from the user's favorite list
	s.RedisService.RemoveFromSet(fmt.Sprintf("user_fav:%d", userID), strconv.Itoa(markerID))
	s.RedisService.ResetCache(fmt.Sprintf("user_fav_marker:%d:%d", userID, markerID))
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
	s.RedisService.ResetAllCache(fmt.Sprintf("user_markers:%d:page:*", userID)) // TODO: Only invalidate the affected page if possible
}

// user_profile
// GetUserProfileCache retrieves the cached user profile from Redis as byte data.
func (s *MarkerCacheService) GetUserProfileCache(userID int) ([]byte, error) {
	// Construct the Redis key for the user profile
	userProfileKey := fmt.Sprintf("user_profile:%d", userID)

	// Retrieve the cached byte data from Redis
	var userData []byte
	err := s.RedisService.GetCacheEntry(userProfileKey, &userData)
	if err != nil || len(userData) == 0 {
		return nil, err // Cache miss or error
	}

	return userData, nil
}

// SetUserProfileCache caches the user profile as byte data in Redis with a specified TTL.
func (s *MarkerCacheService) SetUserProfileCache(userID int, userProfileData []byte) error {
	// Construct the Redis key for the user profile
	userProfileKey := fmt.Sprintf("user_profile:%d", userID)

	// Set the cache entry in Redis with the specified TTL
	return s.RedisService.SetCacheEntry(userProfileKey, userProfileData, 3*time.Hour)
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
	// user added markers
	s.RedisService.ResetAllCache(fmt.Sprintf("userMarkers:%d:page:*", userID))

	// facilities
	s.RedisService.ResetCache(fmt.Sprintf("facilities:%d", markerID))

	// user fav
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
	ctx := context.Background()
	getCmd := s.RedisService.Core.Client.B().Get().Key(cacheKey).Build()
	return s.RedisService.Core.Client.Do(ctx, getCmd).AsBytes()
}

// SetCloseMarkersCache caches the close markers response in Redis with a specified TTL
func (s *MarkerCacheService) SetCloseMarkersCache(cacheKey string, data []byte, ttl time.Duration) error {
	ctx := context.Background()
	setCmd := s.RedisService.Core.Client.B().Set().Key(cacheKey).Value(rueidis.BinaryString(data)).Ex(ttl).Build()
	return s.RedisService.Core.Client.Do(ctx, setCmd).Error()
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

// generateCacheKey generates a unique cache key based on latitude and longitude.
func generateCacheKey(latitude, longitude float64) string {
	return fmt.Sprintf("wcong:%f:%f", latitude, longitude)
}
