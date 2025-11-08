package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/Alfex4936/chulbong-kr/handler"
	"github.com/Alfex4936/chulbong-kr/middleware"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/ansrivas/fiberprometheus/v2"
	sonic "github.com/bytedance/sonic"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/django/v3"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/valyala/fasthttp/reuseport"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// NewFiberApp creates and configures the Fiber application
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

	// Set GOMAXPROCS to 1
	// setting GOMAXPROCS=1 can simplify thread scheduling, reduce contention, and improve cache locality on each core.
	runtime.GOMAXPROCS(1)

	app := fiber.New(fiber.Config{
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

// RegisterHooks sets up lifecycle hooks for starting and stopping the Fiber server
func RegisterHooks(lc fx.Lifecycle,
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

// logRuntimeMetrics logs runtime metrics periodically
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
