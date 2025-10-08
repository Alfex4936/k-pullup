package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Alfex4936/chulbong-kr/config"
	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/util"

	sonic "github.com/bytedance/sonic"
	"github.com/redis/rueidis"
)

type RedisClient struct {
	Mu     sync.RWMutex
	Client rueidis.Client
}

func (src *RedisClient) Reconnect(newClient rueidis.Client) {
	src.Mu.Lock()
	defer src.Mu.Unlock()
	src.Client.Close()
	src.Client = newClient
}

type RedisService struct {
	RedisConfig *config.RedisConfig
	Core        *RedisClient
}

// NewRedisService creates a new instance of RedisService with the provided configuration and Redis client.
func NewRedisService(redisConfig *config.RedisConfig, redis *RedisClient) *RedisService {
	return &RedisService{
		RedisConfig: redisConfig,
		Core:        redis,
	}
}

// TODO: cannot use Generic as Fx doesn't support it directly maybe
// SetCacheEntry sets a cache entry with the given key and value, with an expiration time.
// func (s *RedisService[T])...
func (s *RedisService) SetCacheEntry(key string, value interface{}, expiration time.Duration) error {
	jsonValue, err := sonic.Marshal(value)
	if err != nil {
		return err
	}

	ctx := context.Background()
	setCmd := s.Core.Client.B().Set().Key(key).Value(rueidis.BinaryString(jsonValue)).Ex(expiration).Build()
	return s.Core.Client.Do(ctx, setCmd).Error()
}

// TODO: cannot use Generic as Fx doesn't support it directly maybe
// GetCacheEntry retrieves a cache entry by its key and unmarshals it into the provided type.
func (s *RedisService) GetCacheEntry(key string, target interface{}) error {
	ctx := context.Background()
	getCmd := s.Core.Client.B().Get().Key(key).Build()
	resp, err := s.Core.Client.Do(ctx, getCmd).ToString()

	if err != nil {
		return err
	}
	if resp != "" {
		// Use StringToBytes to avoid unnecessary allocation
		err = sonic.Unmarshal(util.StringToBytes(resp), target)
	}
	return err
}

func (s *RedisService) Delete(key string) error {
	return s.ResetCache(key)
}

// ResetCache invalidates cache entries by deleting the specified key
func (s *RedisService) ResetCache(key string) error {
	ctx := context.Background()

	// Build and execute the DEL command using the client
	delCmd := s.Core.Client.B().Del().Key(key).Build()

	// Execute the DELETE command
	if err := s.Core.Client.Do(ctx, delCmd).Error(); err != nil {
		return err
	}

	return nil
}

// ResetAllCache invalidates cache entries by deleting all keys matching a given pattern.
func (s *RedisService) ResetAllCache(pattern string) error {
	ctx := context.Background()

	var cursor uint64
	for {
		// Build the SCAN command with the current cursor
		scanCmd := s.Core.Client.B().Scan().Cursor(cursor).Match(pattern).Count(10).Build()

		// Execute the SCAN command to find keys matching the pattern
		scanEntry, err := s.Core.Client.Do(ctx, scanCmd).AsScanEntry()
		if err != nil {
			return err
		}

		// Use the ScanEntry for cursor and keys directly
		cursor = scanEntry.Cursor
		keys := scanEntry.Elements

		// Delete keys using individual DEL commands
		for _, key := range keys {
			delCmd := s.Core.Client.B().Del().Key(key).Build()
			if err := s.Core.Client.Do(ctx, delCmd).Error(); err != nil {
				return err
			}
		}

		// If the cursor returned by SCAN is 0, iterated through all the keys
		if cursor == 0 {
			break
		}
	}

	return nil
}

// GetMembersOfSet retrieves all members of a Redis set
func (s *RedisService) GetMembersOfSet(key string) ([]string, error) {
	ctx := context.Background()
	membersCmd := s.Core.Client.B().Smembers().Key(key).Build()

	// Execute the SMEMBERS command and return the list of members
	members, err := s.Core.Client.Do(ctx, membersCmd).AsStrSlice()
	if err != nil {
		return nil, err
	}

	return members, nil
}

// AddToSet adds a member to a Redis set
func (s *RedisService) AddToSet(key string, member string) error {
	ctx := context.Background()
	addCmd := s.Core.Client.B().Sadd().Key(key).Member(member).Build()
	return s.Core.Client.Do(ctx, addCmd).Error()
}

// RemoveFromSet removes a member from a Redis set
func (s *RedisService) RemoveFromSet(key string, member string) error {
	ctx := context.Background()
	removeCmd := s.Core.Client.B().Srem().Key(key).Member(member).Build()
	return s.Core.Client.Do(ctx, removeCmd).Error()
}

// REDIS GEO
func (s *RedisService) AddGeoMarker(key string, lat float64, lon float64) error {
	ctx := context.Background()
	geoAddCmd := s.Core.Client.B().Geoadd().Key("geo:markers").LongitudeLatitudeMember().LongitudeLatitudeMember(lon, lat, key).Build()
	return s.Core.Client.Do(ctx, geoAddCmd).Error()
}

func (s *RedisService) AddGeoMarkers(markers []dto.MarkerSimple) error {
	ctx := context.Background()

	// Check the number of existing members
	zcardCmd := s.Core.Client.B().Zcard().Key("geo:markers").Build()
	count, err := s.Core.Client.Do(ctx, zcardCmd).AsInt64()
	if err != nil {
		return err
	}

	// If there are already 10 or more members, do not add new markers
	if count >= 10 {
		//  s.FindNearbyMarkersMeter(37.568166, 126.974102, 5000)

		return nil
	}

	// Add markers
	for _, marker := range markers {
		geoAddCmd := s.Core.Client.B().Geoadd().Key("geo:markers").LongitudeLatitudeMember().LongitudeLatitudeMember(marker.Longitude, marker.Latitude, strconv.Itoa(marker.MarkerID)).Build()
		if err := s.Core.Client.Do(ctx, geoAddCmd).Error(); err != nil {
			continue
		}
	}

	return nil
}

func (s *RedisService) RemoveGeoMarker(key string) error {
	ctx := context.Background()
	zremCmd := s.Core.Client.B().Zrem().Key("geo:markers").Member(key).Build()
	return s.Core.Client.Do(ctx, zremCmd).Error()
}

func (s *RedisService) RemoveGeoAllMarkers() error {
	ctx := context.Background()
	delCmd := s.Core.Client.B().Del().Key("geo:markers").Build()
	return s.Core.Client.Do(ctx, delCmd).Error()
}

func (s *RedisService) FindNearbyMarkersKM(lat float64, lon float64, radius float64) ([]rueidis.GeoLocation, error) {
	ctx := context.Background()

	// Manually build the GEOSEARCH command with the correct casing
	cmd := s.Core.Client.B().Arbitrary("GEOSEARCH").
		Keys("geo:markers").
		Args("FROMLONLAT", fmt.Sprintf("%f", lon), fmt.Sprintf("%f", lat), "BYRADIUS", fmt.Sprintf("%f", radius), "KM", "ASC", "WITHCOORD", "WITHDIST").
		Build()

	// Execute the command
	result, err := s.Core.Client.Do(ctx, cmd).AsGeosearch()
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *RedisService) FindNearbyMarkersMeter(lat float64, lon float64, radius float64) ([]rueidis.GeoLocation, error) {
	ctx := context.Background()
	// Manually build the GEOSEARCH command with the correct casing
	cmd := s.Core.Client.B().Arbitrary("GEOSEARCH").
		Keys("geo:markers").
		Args("FROMLONLAT", fmt.Sprintf("%f", lon), fmt.Sprintf("%f", lat), "BYRADIUS", fmt.Sprintf("%f", radius), "M", "ASC", "WITHCOORD", "WITHDIST").
		Build()

	// Execute the command
	result, err := s.Core.Client.Do(ctx, cmd).AsGeosearch()
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetMarkerCreateCount gets the current marker creation count for a user on a specific date
func (s *RedisService) GetMarkerCreateCount(userID int, date string) (int64, error) {
	ctx := context.Background()
	key := fmt.Sprintf("marker_limit:%d:%s", userID, date)

	getCmd := s.Core.Client.B().Get().Key(key).Build()
	resp, err := s.Core.Client.Do(ctx, getCmd).ToString()

	if err != nil {
		if rueidis.IsRedisNil(err) {
			return 0, nil // Key doesn't exist, count is 0
		}
		return 0, err
	}

	if resp == "" {
		return 0, nil
	}

	count, err := strconv.ParseInt(resp, 10, 64)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// IncrementMarkerCreateCount increments the marker creation count for a user on a specific date
// and sets expiration if it's the first increment of the day
func (s *RedisService) IncrementMarkerCreateCount(userID int, date string) (int64, error) {
	ctx := context.Background()
	key := fmt.Sprintf("marker_limit:%d:%s", userID, date)

	// Use INCR command to atomically increment
	incrCmd := s.Core.Client.B().Incr().Key(key).Build()
	newCount, err := s.Core.Client.Do(ctx, incrCmd).AsInt64()
	if err != nil {
		return 0, err
	}

	// If this is the first increment (count = 1), set expiration to end of day
	if newCount == 1 {
		// Set expiration to 24 hours from now
		expireCmd := s.Core.Client.B().Expire().Key(key).Seconds(24 * 60 * 60).Build()
		if err := s.Core.Client.Do(ctx, expireCmd).Error(); err != nil {
			// Log error but don't fail the operation
			return newCount, nil
		}
	}

	return newCount, nil
}

// GetRemainingMarkerCreates returns how many markers a user can still create today
func (s *RedisService) GetRemainingMarkerCreates(userID int, date string) (int, error) {
	currentCount, err := s.GetMarkerCreateCount(userID, date)
	if err != nil {
		return 0, err
	}

	remaining := 10 - int(currentCount)
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// GetCommentCreateCount gets the current comment creation count for a user on a specific date
func (s *RedisService) GetCommentCreateCount(userID int, date string) (int64, error) {
	ctx := context.Background()
	key := fmt.Sprintf("comment_limit:%d:%s", userID, date)

	getCmd := s.Core.Client.B().Get().Key(key).Build()
	resp, err := s.Core.Client.Do(ctx, getCmd).ToString()

	if err != nil {
		if rueidis.IsRedisNil(err) {
			return 0, nil // Key doesn't exist, count is 0
		}
		return 0, err
	}

	if resp == "" {
		return 0, nil
	}

	count, err := strconv.ParseInt(resp, 10, 64)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// IncrementCommentCreateCount increments the comment creation count for a user on a specific date
// and sets expiration if it's the first increment of the day
func (s *RedisService) IncrementCommentCreateCount(userID int, date string) (int64, error) {
	ctx := context.Background()
	key := fmt.Sprintf("comment_limit:%d:%s", userID, date)

	// Use INCR command to atomically increment
	incrCmd := s.Core.Client.B().Incr().Key(key).Build()
	newCount, err := s.Core.Client.Do(ctx, incrCmd).AsInt64()
	if err != nil {
		return 0, err
	}

	// If this is the first increment (count = 1), set expiration to end of day
	if newCount == 1 {
		// Set expiration to 24 hours from now
		expireCmd := s.Core.Client.B().Expire().Key(key).Seconds(24 * 60 * 60).Build()
		if err := s.Core.Client.Do(ctx, expireCmd).Error(); err != nil {
			// Log error but don't fail the operation
			return newCount, nil
		}
	}

	return newCount, nil
}

// GetRemainingCommentCreates returns how many comments a user can still create today
func (s *RedisService) GetRemainingCommentCreates(userID int, date string) (int, error) {
	currentCount, err := s.GetCommentCreateCount(userID, date)
	if err != nil {
		return 0, err
	}

	remaining := 15 - int(currentCount)
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}
