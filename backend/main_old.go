package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/Alfex4936/chulbong-kr/config"
	configfx "github.com/Alfex4936/chulbong-kr/configfx"
	servicefx "github.com/Alfex4936/chulbong-kr/di"
	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/handler"
	"github.com/Alfex4936/chulbong-kr/middleware"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/Alfex4936/tzf"
	"github.com/ansrivas/fiberprometheus/v2"
	"github.com/blevesearch/bleve/v2"
	_ "github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	_ "github.com/blevesearch/bleve/v2/analysis/char/html"
	_ "github.com/blevesearch/bleve/v2/analysis/lang/cjk"
	_ "github.com/blevesearch/bleve/v2/analysis/token/edgengram"
	_ "github.com/blevesearch/bleve/v2/analysis/token/ngram"
	_ "github.com/blevesearch/bleve/v2/analysis/token/unicodenorm"
	_ "github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	_ "github.com/blevesearch/bleve/v2/index/upsidedown/store/boltdb"
	"github.com/dgraph-io/ristretto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/spf13/viper"
	"github.com/valyala/fasthttp/reuseport"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/joho/godotenv/autoload"
	"github.com/redis/rueidis"

	ristretto_store "github.com/eko/gocache/store/ristretto/v4"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/django/v3"

	sonic "github.com/bytedance/sonic"

	amqp "github.com/rabbitmq/amqp091-go"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

type BleveSearchService struct {
	Shards []bleve.Index    // Store individual shard indexes
	Alias  bleve.IndexAlias // Keep the alias for querying
}

func logRuntimeMetrics(logger *zap.Logger) {
	var memStats runtime.MemStats

	for {
		// Pause before logging again
		time.Sleep(10 * time.Minute)

		// Capture current memory stats
		runtime.ReadMemStats(&memStats)

		// Log runtime statistics
		logger.Info("Runtime metrics",
			zap.Int("goroutines", runtime.NumGoroutine()),                     // Number of goroutines
			zap.Uint64("alloc", memStats.Alloc),                               // Allocated memory
			zap.Uint64("total_alloc", memStats.TotalAlloc),                    // Total allocated memory
			zap.Uint64("sys", memStats.Sys),                                   // System memory
			zap.Uint64("heap_alloc", memStats.HeapAlloc),                      // Heap memory allocated
			zap.Uint64("heap_sys", memStats.HeapSys),                          // Heap memory in use
			zap.Uint64("heap_idle", memStats.HeapIdle),                        // Heap memory idle
			zap.Uint64("heap_inuse", memStats.HeapInuse),                      // Heap memory in use
			zap.Uint64("heap_released", memStats.HeapReleased),                // Heap memory released
			zap.Uint64("heap_objects", memStats.HeapObjects),                  // Number of heap objects
			zap.Uint64("stack_inuse", memStats.StackInuse),                    // Stack memory in use
			zap.Uint64("stack_sys", memStats.StackSys),                        // Stack memory system
			zap.Uint64("gc_sys", memStats.GCSys),                              // GC system memory
			zap.Uint64("next_gc", memStats.NextGC),                            // Next GC will happen after this amount of heap allocation
			zap.Uint32("gc_cpu_fraction", uint32(memStats.GCCPUFraction*100)), // GC CPU fraction
			zap.Uint64("last_gc", memStats.LastGC),                            // Last GC time in nanoseconds
		)
	}
}

func resolveDomainToIPs(domain string) ([]string, error) {
	ips, err := net.LookupIP(domain)
	if err != nil {
		return nil, err
	}

	var ipStrings []string
	for _, ip := range ips {
		ipStrings = append(ipStrings, ip.String())
	}
	return ipStrings, nil
}

func refreshHandler(
	appConfigFunc func() *config.AppConfig,
	kakaoConfigFunc func() *config.KakaoConfig,
	redisConfigFuc func() *config.RedisConfig,
	zincSearchConfigFunc func() *config.ZincSearchConfig,
	s3ConfigFunc func() *config.S3Config,
	smtpConfigFunc func() *config.SmtpConfig,
	tossConfigFunc func() *config.TossPayConfig,
	oauthConfigFunc func() *config.OAuthConfig,
) func(c *fiber.Ctx) error {
	viper.AutomaticEnv()
	// Load environment variables from a .env file if not in production
	if viper.GetString("DEPLOYMENT") != "production" {
		viper.SetConfigFile(".env")
		_ = viper.ReadInConfig()
	}

	return func(c *fiber.Ctx) error {
		configName := c.Params("config")
		switch configName {
		case "app":
			appConfigFunc()
		case "kakao":
			kakaoConfigFunc()
		case "redis":
			redisConfigFuc()
		case "zincsearch":
			zincSearchConfigFunc()
		case "s3":
			s3ConfigFunc()
		case "smtp":
			smtpConfigFunc()
		case "toss":
			tossConfigFunc()
		case "oauth":
			oauthConfigFunc()
		default:
			// Load all configs
			appConfigFunc()
			kakaoConfigFunc()
			redisConfigFuc()
			zincSearchConfigFunc()
			s3ConfigFunc()
			smtpConfigFunc()
			tossConfigFunc()
			oauthConfigFunc()
		}

		log.Printf("🐔 TEST_VAL = %v", viper.GetString("TEST_VALUE"))

		return c.SendString("Configuration refreshed")
	}
}

// ඞ Fiber app constructor
func NewFiberApp(
	logger *zap.Logger,
	chatUtil *util.ChatUtil,
	wsConfig websocket.Config,
	markerHandler *handler.MarkerHandler,
	userHandler *handler.UserHandler,
	searchHandler *handler.SearchHandler,
	adminHandler *handler.AdminHandler,
	authHandler *handler.AuthHandler,
	chatHandler *handler.ChatHandler,
	commentHandler *handler.CommentHandler,
	notificatinHandler *handler.NotificationHandler,
	kakaobotHandler *handler.KakaoBotHandler,
	authMiddleware *middleware.AuthMiddleware,
	zapMiddleware *middleware.LogMiddleware,
	prometheusRegistry prometheus.Registerer,
) *fiber.App {

	// Load the Caffe model
	// net := gocv.NewCascadeClassifier() // gocv.NewScalar(104, 177, 123, 0)
	// net.Load("fonts/haarcascade_frontalface_default.xml")

	// net := gocv.ReadNetFromCaffe("fonts/deploy.mobilnet.prototxt", "fonts/mobilenet_iter_73000.caffemodel") // gocv.NewScalar(104, 177, 123, 0)
	// net := gocv.ReadNetFromCaffe("fonts/deploy.prototxt", "fonts/res10_300x300_ssd_iter_140000.caffemodel") // gocv.NewScalar(104, 177, 123, 0)
	// net := gocv.ReadNet("fonts/opencv_face_detector_uint8.pb", "fonts/opencv_face_detector.pbtxt")
	// net := gocv.ReadNetFromONNX("fonts/yolov8n-face.onnx")
	// defer net.Close()
	// net.SetPreferableBackend(gocv.NetBackendType(gocv.NetBackendDefault))
	// net.SetPreferableTarget(gocv.NetTargetType(gocv.NetTargetCPU))

	// faceDetection(net)
	// faceDetectionXML(net)
	// faceDetectionPigo()

	// modelPath := "fonts/yolov8n-face.onnx"
	// imgPath := "fonts/1.jpg"
	// savePath := "fonts/face.png" // Path where the edited image will be saved

	// yolo := NewYOLOv8Face(modelPath, 0.45, 0.5)
	// img := gocv.IMRead(imgPath, gocv.IMReadColor)
	// defer img.Close()

	// boxes, scores, _, kpts := yolo.detect(img)
	// yolo.drawDetections(&img, boxes, scores, kpts)

	// // Save the edited image
	// if ok := gocv.IMWrite(savePath, img); !ok {
	// 	fmt.Println("Error saving image")
	// }

	// ips, err := resolveDomainToIPs("k-pullup.com")
	// if err != nil {
	// 	fmt.Println("Error resolving domain:", err)
	// 	os.Exit(1)
	// }

	// Set GOMAXPROCS to 1
	// setting GOMAXPROCS=1 can simplify thread scheduling, reduce contention, and improve cache locality on each core.
	runtime.GOMAXPROCS(1)

	app := fiber.New(fiber.Config{
		// TrustedProxies: ips,
		// ProxyHeader:    fiber.HeaderXForwardedFor,

		Immutable:     false,
		Prefork:       false,
		CaseSensitive: true,
		StrictRouting: true,
		ServerHeader:  "nginx",
		BodyLimit:     30 * 1024 * 1024, // limit to 30 MB
		IdleTimeout:   60 * time.Second,
		ReadTimeout:   10 * time.Second,
		WriteTimeout:  10 * time.Second,
		JSONEncoder:   sonic.Marshal,
		JSONDecoder:   sonic.Unmarshal,
		AppName:       "k-pullup",
		Concurrency:   512 * 1024,
		Views:         django.New("./view", ".django"),
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			// Initial status code defaults to 500
			code := fiber.StatusInternalServerError

			// Retrieve the custom status code if it's a *fiber.Error
			var e *fiber.Error
			if errors.As(err, &e) {
				code = e.Code
			}

			// Define a user-friendly error response
			errorResponse := fiber.Map{
				"success": false,
				"message": "Something went wrong on our end. Please try again later.",
			}

			// Customize the message for known error codes
			switch code {
			case fiber.StatusNotFound: // 404
				errorResponse["message"] = "The requested resource could not be found."
			case fiber.StatusInternalServerError: // 500
				errorResponse["message"] = "An unexpected error occurred. We're working to fix the problem. Please try again later."
				// TODO: Optionally add a reference code
				// errorResponse["reference_code"] = "REF123456"
			}

			// Send a JSON response with the error message and status code
			return ctx.Status(code).JSON(errorResponse)
		},
	})

	// Set up middlewares
	fp := fiberprometheus.NewWithRegistry(prometheusRegistry, "go-service", "http", "", nil)
	fp.SetSkipPaths([]string{"/"})
	fp.RegisterAt(app, "/metrics")
	app.Use(fp.Middleware)

	middleware.SetupMiddlewares(app, logger, chatUtil, authMiddleware, zapMiddleware)

	// Set up routes
	api := app.Group("/api/v1")
	handler.RegisterMarkerRoutes(api, markerHandler, authMiddleware)
	handler.RegisterReportRoutes(api, markerHandler, authMiddleware)
	handler.RegisterUserRoutes(api, userHandler, authMiddleware)
	handler.RegisterSearchRoutes(api, searchHandler)
	handler.RegisterAdminRoutes(api, adminHandler, authMiddleware)
	handler.RegisterAuthRoutes(api, authHandler, authMiddleware)
	handler.RegisterChatRoutes(app, wsConfig, chatHandler) // not /api/v1/
	handler.RegisterCommentRoutes(api, commentHandler, authMiddleware)
	handler.RegisterNotificationRoutes(app, wsConfig, notificatinHandler, authMiddleware) // not /api/v1/
	handler.RegisterKakaoBotRoutes(api, kakaobotHandler, authMiddleware)

	return app
}

// Provides a new logger instance.
func NewLogger() (*zap.Logger, error) {
	return zap.NewProduction()
}

func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second, // Set a timeout to avoid hanging requests indefinitely
	}
}

func NewGeminiClient() (*genai.Client, error) {
	return genai.NewClient(context.Background(), option.WithAPIKey(viper.GetString("GEMINI_API_KEY")))
}

func NewLavinMqClient() (*amqp.Connection, error) {
	return amqp.Dial(viper.GetString("LAVINMQ_HOST"))
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

func NewRedis(lifecycle fx.Lifecycle, logger *zap.Logger) (*service.RedisClient, error) {
	// Initialize redis
	rdb, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress:       []string{viper.GetString("REDIS_HOST") + ":" + viper.GetString("REDIS_PORT")},
		Username:          viper.GetString("REDIS_USERNAME"),
		Password:          viper.GetString("REDIS_PASSWORD"),
		DisableCache:      true, // dragonfly doesn't support CACHING command
		SelectDB:          0,
		ForceSingleClient: true,
		// PoolSize:    10 * runtime.GOMAXPROCS(0),
		// MaxRetries:  5,
		// PipelineMultiplex: 2, // Default is typically sufficient
		// BlockingPoolSize:  5,
		TLSConfig: &tls.Config{InsecureSkipVerify: true},
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

func NewStationData() (map[string]dto.KoreaStation, error) {
	stationMap := make(map[string]dto.KoreaStation)

	file, err := os.Open("./resource/stations.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Decode the JSON data
	decoder := sonic.ConfigDefault.NewDecoder(file)
	var data struct {
		Data []struct {
			BldnNm string `json:"bldn_nm"`
			Lat    string `json:"lat"`
			Lot    string `json:"lot"`
		} `json:"DATA"`
	}
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}

	// Populate the stationMap
	for _, item := range data.Data {
		lat, err := strconv.ParseFloat(item.Lat, 64)
		if err != nil {
			continue
		}
		lon, err := strconv.ParseFloat(item.Lot, 64)
		if err != nil {
			continue
		}
		name := item.BldnNm

		// If the station name contains extra information in parentheses, strip it
		if idx := strings.Index(name, "("); idx != -1 {
			name = name[:idx]
		}

		// Ensure the station name ends with "역"
		if !strings.HasSuffix(name, "역") {
			name = name + "역"
		}

		stationMap[name] = dto.KoreaStation{
			Name:      name,
			Latitude:  lat,
			Longitude: lon,
		}
	}

	return stationMap, nil
}

func NewWsConfig() websocket.Config {
	return websocket.Config{
		// Set the handshake timeout to a reasonable duration to prevent slowloris attacks.
		HandshakeTimeout: 5 * time.Second,

		// TODO: PRODUCTION
		Origins: []string{"https://test.k-pullup.com", "https://www.k-pullup.com", "https://m.k-pullup.com", "https://local.k-pullup.com:5173"},

		EnableCompression: true,

		RecoverHandler: func(c *websocket.Conn) {
			// Custom recover logic. By default, it logs the error and stack trace.
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "WebSocket panic: %v\n", r)
				debug.PrintStack()
				c.WriteMessage(websocket.CloseMessage, []byte{})
				c.Close()
			}
		},
	}
}

// Load timezone finder
func NewTimeZoneFinder(logger *zap.Logger) (tzf.F, error) {
	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		logger.Fatal("Error loading timezone finder", zap.Error(err))
		return &tzf.DefaultFinder{}, err
	}
	return finder, nil
}

// Create a new Bleve index service with sharding
func NewBleveIndex() ([]bleve.Index, bleve.Index, error) {
	var shards []bleve.Index
	searchShardHandler := bleve.NewIndexAlias()

	for i := 0; i < 3; i++ {
		indexShardName := fmt.Sprintf("markers_shard_%d.bleve", i)
		index, err := bleve.Open(indexShardName)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open shard %d: %v", i, err)
		}
		shards = append(shards, index) // Store each shard
		searchShardHandler.Add(index)  // Add to alias for querying
	}

	return shards, searchShardHandler, nil
}

// NewRegistry creates and returns a new Prometheus registry and registers some standard collectors.
func NewRegistry() prometheus.Registerer {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// No need to create a new registry, just use the default one
	requestDurationHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Histogram for HTTP request durations.",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // Use optimized buckets
	})

	// Register only if not already registered
	if err := prometheus.DefaultRegisterer.Register(requestDurationHistogram); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			requestDurationHistogram = existing.ExistingCollector.(prometheus.Histogram)
		} else {
			panic(err) // Real error, not just a duplicate metric
		}
	}

	return reg
}

// NewLoginCounter creates a custom Prometheus counter for tracking login events.
func NewLoginCounter(registry prometheus.Registerer) prometheus.Counter {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "login_total",
		Help: "Total number of successful login requests",
	})
	registry.MustRegister(counter)
	return counter
}

// MAIN Fx
// @title k-pullup API
// @description This is the API documentation for the k-pullup service.
// @version 1.0
// @license MIT
// @license.url https://raw.githubusercontent.com/Alfex4936/chulbong-kr/refs/heads/main/LICENSE
// @contact.name API Support
// @contact.email chulbong.kr@gmail.com
// @contact.url https://github.com/Alfex4936
func main() {
	viper.AutomaticEnv() // Automatically read from environment variables
	if viper.GetString("DEPLOYMENT") != "production" {
		viper.SetConfigFile(".env")
		if err := viper.ReadInConfig(); err != nil {
		}
	}

	// Load environment variables from a .env file if not in production
	if os.Getenv("DEPLOYMENT") != "production" {
		godotenv.Overload()
	}

	// Create an Fx application with provided dependencies and lifecycle hooks
	fx.New(
		servicefx.FxMarkerModule,
		servicefx.FxExternalModle,
		servicefx.FxChatModule,
		servicefx.FxUtilModule,
		servicefx.FxUserModule,
		servicefx.FxAPIModule,
		servicefx.FxFacadeModule,

		configfx.FxConfigModule,

		fx.Provide(

			NewHTTPClient,
			NewLogger,
			NewDatabase,
			NewRedis,
			NewWsConfig,
			NewStationData,
			NewTimeZoneFinder,
			NewBleveIndex,
			NewGoCacheLocalStorage,

			NewRegistry,
			NewLoginCounter,
			// NewGeminiClient,
			// NewLavinMqClient,

			middleware.NewAuthMiddleware,
			middleware.NewLogMiddleware,

			NewFiberApp,
		),
		fx.Invoke(
			registerHooks,
			util.RegisterBadWordUtilLifecycle,
			service.RegisterSchedulerLifecycle,
			util.RegisterPdfInitLifecycle,
			service.RegisterMarkerLifecycle,
			service.RegisterMarkerLocationLifecycle,
			service.RegisterAuthLifecycle,
			service.RegisteBleveLifecycle,
			service.RegisterTokenServiceLifecycle,
		), // func(diGraph fx.DotGraph) {
		// logger.Debug("➡️", diGraph)
		// }

	).Run()
}

// registerHooks sets up lifecycle hooks for starting and stopping the Fiber server
func registerHooks(lc fx.Lifecycle,
	app *fiber.App, db *sqlx.DB, logger *zap.Logger,
	rankService *service.MarkerRankService,
	redisService *service.RedisService,
	safeClient *service.RedisClient,
) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			// Default to the port from the .env file
			serverPort := os.Getenv("SERVER_PORT")

			// Check if a command-line argument was provided for the port
			if len(os.Args) > 1 {
				serverPort = os.Args[1]
			}

			// Set a default port if not set by .env or command-line argument
			if serverPort == "" {
				serverPort = "8080" // Default port if none provided
			}

			serverAddr := fmt.Sprintf("0.0.0.0:%s", serverPort)

			logger.Info("💖 Starting Fiber v2 server...")

			go func() {
				if os.Getenv("DEPLOYMENT") == "production" {
					// Send Slack notification
					go util.SendDeploymentSuccessNotification(app.Config().AppName, "fly.io")
					// Random ranking
					go rankService.ResetAndRandomizeClickRanking()

					// Start logging runtime metrics
					go logRuntimeMetrics(logger)
				} else {
					logger.Info("There are APIs available in chulbong-kr", zap.Int("API count", countAPIs(app)))
				}

				// util.SendSlackReportNotification("test")

				ln, err := reuseport.Listen("tcp4", serverAddr)
				if err != nil {
					log.Fatalf("Error while setting up listener: %s", err)
				}

				if err := app.Listener(ln); err != nil {
					logger.Fatal("Failed to start Fiber v2", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			logger.Info("=== Shutting down Fiber v2 server...")
			if err := db.Close(); err != nil {
				logger.Error("Failed to close database connection", zap.Error(err))
			}
			safeClient.Mu.Lock()
			defer safeClient.Mu.Unlock()
			if safeClient.Client != nil {
				safeClient.Client.Close()
			}

			logger.Sync()
			return app.Shutdown()
		},
	})
}

// countAPIs counts the number of APIs in a Fiber app
func countAPIs(app *fiber.App) int {
	numAPIs := 0
	for _, route := range app.GetRoutes(true) {
		// Check if the route is for an API (skip middleware routes)
		if route.Path[len(route.Path)-1] != '*' {
			numAPIs++
		}
	}
	return numAPIs
}

func pingRedis(rdb rueidis.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return rdb.Do(ctx, rdb.B().Ping().Build()).Error()
}

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

	return nil, errors.New("failed to reconnect to Redis after attempts")
}
