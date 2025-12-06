package marker

import (
	"time"

	"github.com/Alfex4936/chulbong-kr/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
)

// RegisterMarkerRoutes wires public and authenticated marker routes.
func RegisterMarkerRoutes(api fiber.Router, handler *MarkerHandler, authMiddleware *middleware.AuthMiddleware) {
	publicGroup := api.Group("/markers")
	publicGroup.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			handler.logger.Error("Panic recovered in public marker API",
				zap.Any("error", e),
				zap.String("url", c.Path()),
				zap.String("method", c.Method()),
			)
		},
	}))

	{
		// Public routes with recover middleware
		publicGroup.Get("", handler.HandleGetAllMarkersLocal)
		// publicGroup.Get("2", handler.HandleGetAllMarkersLocalMsgp)
		// publicGroup.Get("-proto", handler.HandleGetAllMarkersProto)
		publicGroup.Get("/new", handler.HandleGetAllNewMarkers)
		publicGroup.Get("/user/:username", handler.HandleGetMarkersByUsername)
		publicGroup.Get("/:markerId/details", authMiddleware.VerifySoft, handler.HandleGetMarker)
		publicGroup.Get("/:markerID/facilities", handler.HandleGetFacilities)
		publicGroup.Get("/close", handler.HandleFindCloseMarkers)
		publicGroup.Get("/ranking", handler.HandleGetMarkerRanking)
		publicGroup.Get("/unique-ranking", handler.HandleGetUniqueVisitorCount)
		// publicGroup.Get("/unique-ranking/all", handler.HandleGetAllUniqueVisitorCount)
		publicGroup.Get("/area-ranking", handler.HandleGetCurrentAreaMarkerRanking)
		publicGroup.Get("/convert", handler.HandleConvertWGS84ToWCONGNAMUL)
		publicGroup.Get("/location-check", handler.HandleIsInSouthKorea)
		publicGroup.Get("/weather", handler.HandleGetWeatherByWGS84)
		publicGroup.Get("/verify", handler.HandleVerifyMarker)
		publicGroup.Get("/new-markers", handler.HandleGetNewMarkers)
		publicGroup.Get("/:markerID/reactions", authMiddleware.VerifySoft, handler.HandleGetMarkerReactions)
		publicGroup.Get("/save-offline", limiter.New(limiter.Config{
			KeyGenerator: func(c *fiber.Ctx) string {
				return "login-" + handler.deps.MarkerFacadeService.ChatUtil.GetUserIP(c)
			},
			Max:               5,
			Expiration:        1 * time.Minute,
			LimiterMiddleware: limiter.SlidingWindow{},
			LimitReached: func(c *fiber.Ctx) error {
				c.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
				c.Status(429).SendString("Too many requests, please try again later.")
				return nil
			},
			SkipFailedRequests: false,
		}), handler.HandleSaveOfflineMap2)
		publicGroup.Get("/rss", handler.HandleRSS)
		publicGroup.Get("/roadview-date", handler.HandleGetRoadViewPicDate)
		publicGroup.Get("/new-pictures", handler.HandleGet10NewPictures)
		publicGroup.Get("/stories", handler.HandleGetAllStories)
		publicGroup.Get("/:markerID/stories", authMiddleware.VerifySoft, handler.HandleGetStories)
	}

	// Admin routes (still directly on api router)
	api.Post("/markers/upload", authMiddleware.CheckAdmin, handler.HandleUploadMarkerPhotoToS3)

	markerGroup := api.Group("/markers")
	markerGroup.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			handler.logger.Error("Panic recovered in authenticated marker API",
				zap.Any("error", e),
				zap.String("url", c.Path()),
				zap.String("method", c.Method()),
			)
		},
	}))

	{
		markerGroup.Use(authMiddleware.Verify)

		markerGroup.Get("/my", handler.HandleGetUserMarkers)
		markerGroup.Get("/:markerID/dislike-status", handler.HandleCheckDislikeStatus)

		markerGroup.Post("", handler.HandleCreateMarkerWithPhotos)
		markerGroup.Post("/new", handler.HandleCreateMarkerWithPhotos)

		markerGroup.Post("/facilities", handler.HandleSetMarkerFacilities)
		markerGroup.Post("/:markerID/dislike", handler.HandleLeaveDislike)
		markerGroup.Post("/:markerID/reactions", handler.HandleSetMarkerReaction)
		markerGroup.Post("/:markerID/favorites", handler.HandleAddFavorite)

		markerGroup.Put("/:markerID", handler.HandleUpdateMarker)

		markerGroup.Delete("/:markerID", handler.HandleDeleteMarker)
		markerGroup.Delete("/:markerID/photos/:photoID", handler.HandleDeleteMarkerPhoto)
		markerGroup.Delete("/:markerID/dislike", handler.HandleUndoDislike)
		markerGroup.Delete("/:markerID/reactions", handler.HandleRemoveMarkerReaction)
		markerGroup.Delete("/:markerID/favorites", handler.HandleRemoveFavorite)

		// Story routes
		markerGroup.Post("/:markerID/stories", handler.HandleAddStory)
		markerGroup.Delete("/:markerID/stories/:storyID", handler.HandleDeleteStory)
		markerGroup.Post("/stories/:storyID/reactions", handler.HandleAddReaction)
		markerGroup.Delete("/stories/:storyID/reactions", handler.HandleRemoveReaction)
		markerGroup.Post("/stories/:storyID/report", handler.HandleReportStory)
	}
}
