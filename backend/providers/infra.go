package providers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/dgraph-io/ristretto"
	ristretto_store "github.com/eko/gocache/store/ristretto/v4"
	"github.com/jmoiron/sqlx"
	"github.com/redis/rueidis"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"go.uber.org/zap"

	_ "github.com/go-sql-driver/mysql"
)

// NewHTTPClient creates a new HTTP client with timeout
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second, // Set a timeout to avoid hanging requests indefinitely
	}
}

// NewDatabase sets up the database connection
func NewDatabase() (*sqlx.DB, error) {
	dbUsername := os.Getenv("DB_USERNAME")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", dbUsername, dbPassword, dbHost, dbPort, dbName)
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("error connecting to the database: %v", err)
	}
	return db, nil
}

// NewRedis creates a new Redis client with lifecycle management
func NewRedis(lifecycle fx.Lifecycle, logger *zap.Logger) (*service.RedisClient, error) {
	// Initialize redis
	rdb, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress:       []string{viper.GetString("REDIS_HOST") + ":" + viper.GetString("REDIS_PORT")},
		Username:          viper.GetString("REDIS_USERNAME"),
		Password:          viper.GetString("REDIS_PASSWORD"),
		DisableCache:      true, // dragonfly doesn't support CACHING command
		SelectDB:          0,
		ForceSingleClient: true,
		TLSConfig:         &tls.Config{InsecureSkipVerify: true},
	})
	if err != nil {
		logger.Fatal("Error connecting to Redis", zap.Error(err))
	}

	if viper.GetString("DEPLOYMENT") == "production" {
		// Flush the Redis database to clear all keys
		err := rdb.Do(context.Background(), rdb.B().Flushall().Build()).Error()
		if err != nil {
			logger.Fatal("Error executing FLUSHALL SYNC", zap.Error(err))
		}
	}

	safeClient := &service.RedisClient{Client: rdb}

	// Register lifecycle hooks for Redis
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				ticker := time.NewTicker(30 * time.Minute)
				defer ticker.Stop()

				for range ticker.C {
					safeClient.Mu.RLock()
					err := pingRedis(safeClient.Client)
					safeClient.Mu.RUnlock()

					if err != nil {
						logger.Info("Redis ping failed, attempting to reconnect...")
						newClient, err := reconnectRedis(logger)
						if err != nil {
							logger.Fatal("Failed to reconnect", zap.Error(err))
						}
						safeClient.Reconnect(newClient)
					}
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			rdb.Close()
			return nil
		},
	})

	return safeClient, nil
}

// NewGoCacheLocalStorage initializes a new Ristretto cache store with appropriate settings.
func NewGoCacheLocalStorage() (*ristretto_store.RistrettoStore, error) {
	estimatedItems := 10000 // Estimated number of items to cache
	approxItemSize := 200   // Approximate size of each item in bytes
	maxCost := estimatedItems * approxItemSize

	ristrettoCache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,            // 10 million counters for better hit ratio
		MaxCost:     int64(maxCost), // Maximum cost of cache (in bytes)
		BufferItems: 64,             // Number of keys per Get buffer
	})
	if err != nil {
		return nil, err
	}

	ristrettoStore := ristretto_store.NewRistretto(ristrettoCache)
	return ristrettoStore, nil
}

// pingRedis checks Redis connection health
func pingRedis(rdb rueidis.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return rdb.Do(ctx, rdb.B().Ping().Build()).Error()
}

// reconnectRedis attempts to reconnect to Redis
func reconnectRedis(logger *zap.Logger) (rueidis.Client, error) {
	var newRdb rueidis.Client
	for i := 0; i < 3; i++ {
		time.Sleep(time.Duration(i+1) * time.Second)
		var err error
		newRdb, err = rueidis.NewClient(rueidis.ClientOption{
			InitAddress:  []string{viper.GetString("REDIS_HOST") + ":" + viper.GetString("REDIS_PORT")},
			Username:     viper.GetString("REDIS_USERNAME"),
			Password:     viper.GetString("REDIS_PASSWORD"),
			DisableCache: true,
			TLSConfig:    &tls.Config{InsecureSkipVerify: true},
		})
		if err == nil {
			return newRdb, nil
		}
		logger.Warn("Attempt to reconnect failed", zap.Int("attempt", i+1), zap.Error(err))
	}

	return nil, fmt.Errorf("failed to reconnect to Redis after attempts")
}
