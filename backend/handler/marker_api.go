package handler

import (
	"context"
	"errors"
	"math"
	"mime/multipart"
	"os"
	"strconv"
	"strings"
	"time"

	sonic "github.com/bytedance/sonic"
	"go.uber.org/zap"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/facade"
	"github.com/Alfex4936/chulbong-kr/middleware"
	"github.com/Alfex4936/chulbong-kr/protos"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	"google.golang.org/protobuf/proto"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

type MarkerHandler struct {
	MarkerFacadeService *facade.MarkerFacadeService

	CacheService *service.MarkerCacheService

	AuthMiddleware *middleware.AuthMiddleware

	logger *zap.Logger
}

// NewMarkerHandler creates a new MarkerHandler with dependencies injected
func NewMarkerHandler(
	authMiddleware *middleware.AuthMiddleware,
	facade *facade.MarkerFacadeService,
	c *service.MarkerCacheService,
	logger *zap.Logger,
) *MarkerHandler {
	return &MarkerHandler{
		MarkerFacadeService: facade,

		AuthMiddleware: authMiddleware,
		CacheService:   c,

		logger: logger,
	}
}

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
		publicGroup.Get("/save-offline", limiter.New(limiter.Config{
			KeyGenerator: func(c *fiber.Ctx) string {
				return "login-" + handler.MarkerFacadeService.ChatUtil.GetUserIP(c)
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
	// api.Post("/markers/refresh", authMiddleware.CheckAdmin, handler.HandleRefreshMarkerCache)

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
		// markerGroup.Get("/:markerId", handlers.GetMarker)

		markerGroup.Post("", handler.HandleCreateMarkerWithPhotos)
		markerGroup.Post("/new", handler.HandleCreateMarkerWithPhotos)

		markerGroup.Post("/facilities", handler.HandleSetMarkerFacilities)
		markerGroup.Post("/:markerID/dislike", handler.HandleLeaveDislike)
		markerGroup.Post("/:markerID/favorites", handler.HandleAddFavorite)

		markerGroup.Put("/:markerID", handler.HandleUpdateMarker)

		markerGroup.Delete("/:markerID", handler.HandleDeleteMarker)
		markerGroup.Delete("/:markerID/dislike", handler.HandleUndoDislike)
		markerGroup.Delete("/:markerID/favorites", handler.HandleRemoveFavorite)

		// Story routes
		markerGroup.Post("/:markerID/stories", handler.HandleAddStory)
		markerGroup.Delete("/:markerID/stories/:storyID", handler.HandleDeleteStory)
		markerGroup.Post("/stories/:storyID/reactions", handler.HandleAddReaction)
		markerGroup.Delete("/stories/:storyID/reactions", handler.HandleRemoveReaction)
		markerGroup.Post("/stories/:storyID/report", handler.HandleReportStory)
	}
}

func (h *MarkerHandler) HandleGetAllMarkersProto(c *fiber.Ctx) error {
	markers, err := h.MarkerFacadeService.GetAllMarkersProto()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	markerList := &protos.MarkerList{
		Markers: markers,
	}

	data, err := proto.Marshal(markerList)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}

	c.Type("application/protobuf")
	return c.Send(data)
}

// HandleGet10NewPictures retrieves the 10 most recently added marker pictures.
//
// @Summary Get 10 new marker pictures
// @Description Fetches a list of the 10 most recently added pictures associated with markers.
// @ID get-10-new-marker-pictures
// @Tags markers-data
// @Accept json
// @Produce json
// @Success 200 {array} dto.MarkerNewPicture "List of 10 new marker pictures"
// @Failure 500 {object} map[string]string "Failed to fetch markers"
// @Router /api/v1/markers/new-pictures [get]
func (h *MarkerHandler) HandleGet10NewPictures(c *fiber.Ctx) error {
	markers, err := h.MarkerFacadeService.GetNew10Pictures()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to fetch markers " + err.Error(),
		})
	}
	return c.JSON(markers)
}

// HandleGetAllMarkersLocal retrieves all markers.
//
// @Summary Get all markers
// @Description Retrieves a full list of markers from the cache or database.
// @ID get-all-markers
// @Tags markers-data
// @Accept json
// @Produce json
// @Security
// @Success 200 {array} dto.MarkerSimple "List of all markers"
// @Failure 500 {object} map[string]string "Internal server error when retrieving markers"
// @Router /api/v1/markers [get]
func (h *MarkerHandler) HandleGetAllMarkersLocal(c *fiber.Ctx) error {
	// Check the Referer header and redirect if it matches the specific URL pattern
	// if !strings.HasSuffix(c.Get("Referer"), ".k-pullup.com") || c.Get("Referer") != "https://www.k-pullup.com/" {
	// 	return c.Redirect("https://k-pullup.com", fiber.StatusFound) // Use HTTP 302 for standard redirection
	// }
	c.Set("Content-type", "application/json")

	// Attempt to fetch cached data first
	cached, err := h.CacheService.GetAllMarkers() // from Redis, on error will proceed to fetch from DB
	if err == nil && len(cached) > 0 && string(cached) != "null" {
		// Cache hit, return the cached byte array
		c.Append("X-Cache", "hit")
		return c.Send(cached)
	}

	// Cache miss: Fetch markers from DB
	markers, err := h.MarkerFacadeService.GetAllMarkers() // Fetch from DB
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get markers"})
	}

	if len(markers) == 0 {
		// Return empty array instead of null
		return c.Send([]byte("[]"))
	}

	// Marshal the markers to JSON for caching and response
	markersJSON, err := sonic.ConfigFastest.Marshal(markers)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encode markers"})
	}

	// Cache the full list of markers
	// TODO: Stale-While-Revalidate?
	// Verify we're not caching "null" or empty data
	if len(markersJSON) > 0 && string(markersJSON) != "null" {
		// Cache the full list of markers
		err = h.CacheService.SetFullMarkersCache(markersJSON)
		if err != nil {
			h.logger.Error("Failed to cache full markers", zap.Error(err))
		}
	}

	return c.Send(markersJSON)
}

func (h *MarkerHandler) HandleGetAllMarkersLocalMsgp(c *fiber.Ctx) error {
	cached := h.MarkerFacadeService.GetMarkerCache()
	c.Set("Content-type", "application/json")

	if cached != nil || len(cached) != 0 {
		// If cache is not empty, directly return the cached binary data as JSON
		c.Append("X-Cache", "hit")
		return c.Send(cached)
	}

	// Fetch markers if cache is empty
	markers, err := h.MarkerFacadeService.GetAllMarkers() // []dto.MarkerSimple, err
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get markers"})
	}

	// Marshal the markers to JSON for caching and response
	markerSlice := dto.MarkerSimpleSlice(markers)

	markersBin, err := markerSlice.MarshalMsg(nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encode markers"})
	}

	// Update cache
	h.MarkerFacadeService.SetMarkerCache(markersBin)

	return c.Send(markersBin)
}

// HandleGetAllNewMarkers retrieves a paginated list of newly added markers.
//
// @Summary Get newly added markers
// @Description Fetches a paginated list of markers that were recently added.
// @ID get-new-markers
// @Tags markers-data, pagination
// @Accept json
// @Produce json
// @Security
// @Param page query int false "Page number (default: 1)"
// @Param pageSize query int false "Number of markers per page (default: 10)"
// @Success 200 {array} dto.MarkerNewResponse "List of newly added markers"
// @Failure 400 {object} map[string]string "Invalid pagination parameters"
// @Failure 500 {object} map[string]string "Internal server error when fetching markers"
// @Router /api/v1/markers/new [get]
func (h *MarkerHandler) HandleGetAllNewMarkers(c *fiber.Ctx) error {
	pagination, err := util.ParsePaginationParams(c, &util.PaginationConfig{
		DefaultPage:       1,
		DefaultPageSize:   10,
		PageParamName:     "page",
		PageSizeParamName: "pageSize",
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid pagination parameters"})
	}

	// Call the service to get markers
	markers, err := h.MarkerFacadeService.GetAllNewMarkers(pagination.Page, pagination.PageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not fetch markers: " + err.Error()})
	}

	return c.JSON(markers)
}

// HandleGetMarker retrieves details of a specific marker.
//
// @Summary Get marker details
// @Description Fetches detailed information about a specific marker, including user-specific interactions if available.
// @ID get-marker-details
// @Tags markers-data
// @Accept json
// @Produce json
// @Security
// @Param markerId path int true "Marker ID"
// @Success 200 {object} model.MarkerWithPhotos "Marker details including photos"
// @Failure 400 {object} map[string]string "Invalid Marker ID"
// @Failure 404 {object} map[string]string "Marker not found"
// @Router /api/v1/markers/{markerId}/details [get]
func (h *MarkerHandler) HandleGetMarker(c *fiber.Ctx) error {
	markerID, err := strconv.Atoi(c.Params("markerId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid Marker ID"})
	}

	userID, userOK := c.Locals("userID").(int)
	chulbong, _ := c.Locals("chulbong").(bool)

	marker, err := h.MarkerFacadeService.GetMarker(markerID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Marker not found"})
	}

	if userOK {
		// Checking dislikes and favorites only if user is authenticated
		marker.Disliked, _ = h.MarkerFacadeService.CheckUserDislike(userID, markerID)
		marker.Favorited, _ = h.MarkerFacadeService.CheckUserFavorite(userID, markerID)

		// Check ownership. If marker.UserID is nil, chulbong remains as set earlier.
		if !chulbong && marker.UserID != nil {
			marker.IsChulbong = *marker.UserID == userID
		} else {
			marker.IsChulbong = chulbong
		}
	}

	go h.MarkerFacadeService.BufferClickEvent(markerID)
	// go h.MarkerFacadeService.SaveUniqueVisitor(c.Params("markerId"), c)
	return c.JSON(marker)
}

// ADMIN
func (h *MarkerHandler) HandleGetAllMarkersWithAddr(c *fiber.Ctx) error {
	markersWithPhotos, err := h.MarkerFacadeService.GetAllMarkersWithAddr()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(markersWithPhotos)
}

// HandleCreateMarkerWithPhotos creates a new marker with optional photos.
//
// @Summary Create a new marker
// @Description Creates a marker with latitude, longitude, description, and optional photos.
// @ID create-marker-with-photos
// @Tags markers
// @Accept multipart/form-data
// @Produce json
// @Param latitude formData number true "Latitude of the marker"
// @Param longitude formData number true "Longitude of the marker"
// @Param description formData string false "Description of the marker"
// @Param photos formData file false "Marker photos (multiple allowed)"
// @Security ApiKeyAuth
// @Success 201 {object} dto.MarkerResponse "Marker created successfully"
// @Failure 400 {object} map[string]string "Invalid request parameters or form data"
// @Failure 409 {object} map[string]string "Error during file upload"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/markers [post]
func (h *MarkerHandler) HandleCreateMarkerWithPhotos(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parse the multipart form
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to parse form"})
	}

	// Check if latitude and longitude are provided
	latitude, longitude, err := GetLatLngFromForm(form)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to parse latitude/longitude"})
	}

	description := GetDescriptionFromForm(form)

	// check first
	if fErr := h.MarkerFacadeService.CheckMarkerValidity(latitude, longitude, description); fErr != nil {
		return c.Status(fErr.Code).JSON(fiber.Map{"error": fErr.Message})
	}

	description = util.RemoveURLs(description)

	// no errors
	userID := c.Locals("userID").(int)

	marker, err := h.MarkerFacadeService.CreateMarkerWithPhotos(ctx, &dto.MarkerRequest{
		Latitude:    latitude,
		Longitude:   longitude,
		Description: description,
	}, userID, form)
	if err != nil {
		if strings.Contains(err.Error(), "an error during file") {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "an error during file upload"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error happened, try again later"})
	}

	return c.Status(fiber.StatusCreated).JSON(marker)
}

// HandleUpdateMarker updates the description of an existing marker.
//
// @Summary Update marker description
// @Description Allows the authenticated user to update the description of a specific marker.
// @ID update-marker
// @Tags markers
// @Accept application/x-www-form-urlencoded
// @Produce json
// @Param markerID path int true "Marker ID"
// @Param description formData string true "New description for the marker"
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Updated marker description" example: {"description": "New marker description"}
// @Failure 400 {object} map[string]string "Description contains profanity or invalid parameters"
// @Failure 500 {object} map[string]string "Failed to update marker description"
// @Router /api/v1/markers/{markerID} [put]
func (h *MarkerHandler) HandleUpdateMarker(c *fiber.Ctx) error {
	markerID, _ := strconv.Atoi(c.Params("markerID"))
	description := c.FormValue("description")

	if profanity, _ := h.MarkerFacadeService.CheckBadWord(description); profanity {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Description contains profanity"})
	}

	if err := h.MarkerFacadeService.UpdateMarkerDescriptionOnly(markerID, description); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"description": description})
}

// HandleDeleteMarker deletes a marker if the user is the owner or an admin.
//
// @Summary Delete a marker
// @Description Allows the authenticated owner or an admin to delete a specific marker.
// @ID delete-marker
// @Tags markers
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Security ApiKeyAuth
// @Success 200 "Marker deleted successfully"
// @Failure 400 {object} map[string]string "Invalid marker ID"
// @Failure 403 {object} map[string]string "User is not authorized to delete this marker"
// @Failure 500 {object} map[string]string "Failed to delete marker"
// @Router /api/v1/markers/{markerID} [delete]
func (h *MarkerHandler) HandleDeleteMarker(c *fiber.Ctx) error {
	// Auth
	userID := c.Locals("userID").(int)
	userRole := c.Locals("role").(string)

	// Get MarkerID from the URL parameter
	markerIDParam := c.Params("markerID")
	markerID, err := strconv.Atoi(markerIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid marker ID"})
	}

	// Call the service function to delete the marker, now passing userID as well
	err = h.MarkerFacadeService.DeleteMarker(userID, markerID, userRole)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete marker"})
	}

	h.MarkerFacadeService.RemoveMarkerClick(markerID)

	h.CacheService.SetFullMarkersCache(nil)
	h.CacheService.InvalidateFacilities(markerID)
	h.CacheService.RemoveUserMarker(userID, markerID)

	return c.SendStatus(fiber.StatusOK)
}

// UploadMarkerPhotoToS3Handler to upload a file to S3
func (h *MarkerHandler) HandleUploadMarkerPhotoToS3(c *fiber.Ctx) error {
	// Parse the multipart form
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to parse form"})
	}

	markerIDstr, markerIDExists := form.Value["markerId"]
	if !markerIDExists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to parse form"})
	}

	markerID, err := strconv.Atoi(markerIDstr[0])
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to parse form"})
	}

	files := form.File["photos"]

	urls, err := h.MarkerFacadeService.UploadMarkerPhotoToS3(markerID, files)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to upload photos"})
	}

	return c.JSON(fiber.Map{"urls": urls})
}

// HandleLeaveDislike registers a dislike for a specific marker by the authenticated user.
//
// @Summary Leave a dislike on a marker
// @Description Allows the authenticated user to dislike a marker.
// @ID leave-marker-dislike
// @Tags markers
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Security ApiKeyAuth
// @Success 200 "Dislike successfully registered"
// @Failure 400 {object} map[string]string "Invalid marker ID"
// @Failure 500 {object} map[string]string "Failed to leave dislike"
// @Router /api/v1/markers/{markerID}/dislike [post]
func (h *MarkerHandler) HandleLeaveDislike(c *fiber.Ctx) error {
	// Auth
	userID := c.Locals("userID").(int)

	// Get MarkerID from the URL parameter
	markerIDParam := c.Params("markerID")
	markerID, err := strconv.Atoi(markerIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid marker ID"})
	}

	// Call the service function to leave a dislike, passing userID and markerID
	err = h.MarkerFacadeService.LeaveDislike(userID, markerID)
	if err != nil {
		// Handle specific error cases here, for example, a duplicate dislike
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to leave dislike: " + err.Error()})
	}

	return c.SendStatus(fiber.StatusOK)
}

// HandleUndoDislike removes a dislike from a marker.
//
// @Summary Undo marker dislike
// @Description Allows the authenticated user to remove their dislike from a marker.
// @ID undo-marker-dislike
// @Tags markers
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Security ApiKeyAuth
// @Success 200 "Dislike removed successfully"
// @Failure 400 {object} map[string]string "Invalid marker ID"
// @Failure 500 {object} map[string]string "Failed to undo dislike"
// @Router /api/v1/markers/{markerID}/dislike [delete]
func (h *MarkerHandler) HandleUndoDislike(c *fiber.Ctx) error {
	// Auth
	userID := c.Locals("userID").(int)

	// Get MarkerID from the URL parameter
	markerIDParam := c.Params("markerID")
	markerID, err := strconv.Atoi(markerIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid marker ID"})
	}

	// Call the service function to undo a dislike
	err = h.MarkerFacadeService.UndoDislike(userID, markerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to undo dislike: " + err.Error()})
	}

	return c.SendStatus(fiber.StatusOK)
}

// HandleGetUserMarkers retrieves a paginated list of markers created by the authenticated user.
//
// @Summary Get user's markers
// @Description Fetches a paginated list of markers created by the currently authenticated user.
// @ID get-user-markers
// @Tags markers, pagination
// @Accept json
// @Produce json
// @Param page query int false "Page number (default: 1)"
// @Param pageSize query int false "Number of markers per page (default: 5)"
// @Security ApiKeyAuth
// @Success 200 {object} dto.UserMarkers "List of user's markers with pagination"
// @Failure 400 {object} map[string]string "User not authenticated or invalid pagination parameters"
// @Failure 500 {object} map[string]string "Failed to get markers"
// @Router /api/v1/markers/my [get]
func (h *MarkerHandler) HandleGetUserMarkers(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(int)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "User not authenticated"})
	}

	pagination, err := util.ParsePaginationParams(c, &util.PaginationConfig{
		DefaultPage:       1,
		DefaultPageSize:   5,
		PageParamName:     "page",
		PageSizeParamName: "pageSize",
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid pagination parameters"})
	}

	page := pagination.Page
	pageSize := pagination.PageSize

	// Try to get markers from cache
	cachedMarkers, err := h.CacheService.GetUserMarkersPageCache(userID, page)
	if err == nil && len(cachedMarkers) > 0 {
		// If cache hit, calculate total markers and total pages and return the cached response
		totalMarkers := len(cachedMarkers)
		totalPages := int(math.Ceil(float64(totalMarkers) / float64(pageSize)))

		// Prepare the response from the cached data
		response := dto.UserMarkers{
			MarkersWithPhotos: cachedMarkers,
			CurrentPage:       page,
			TotalPages:        totalPages,
			TotalMarkers:      totalMarkers,
		}

		// Return cached response
		c.Append("X-Cache", "hit")
		return c.JSON(response)
	}

	// If no cache, fetch markers from database
	markersWithPhotos, total, err := h.MarkerFacadeService.GetAllMarkersByUserWithPagination(userID, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get markers"})
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	// Prepare the response
	response := dto.UserMarkers{
		MarkersWithPhotos: markersWithPhotos,
		CurrentPage:       page,
		TotalPages:        totalPages,
		TotalMarkers:      total,
	}

	// Cache the response for future requests
	go h.CacheService.AddUserMarkersPageCache(userID, page, markersWithPhotos)

	// Return the response
	return c.JSON(response)
}

// HandleCheckDislikeStatus checks if the authenticated user has disliked a specific marker.
//
// @Summary Check dislike status for a marker
// @Description Returns whether the authenticated user has disliked the given marker.
// @ID check-dislike-status
// @Tags markers-data
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Security ApiKeyAuth
// @Success 200 {object} map[string]bool "Dislike status of the marker" example: {"disliked": true}
// @Failure 400 {object} map[string]string "Invalid marker ID"
// @Failure 500 {object} map[string]string "Error checking dislike status"
// @Router /api/v1/markers/{markerID}/dislike-status [get]
func (h *MarkerHandler) HandleCheckDislikeStatus(c *fiber.Ctx) error {
	userID := c.Locals("userID").(int)
	markerID, err := strconv.Atoi(c.Params("markerID"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid marker ID"})
	}

	disliked, err := h.MarkerFacadeService.CheckUserDislike(userID, markerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "error checking dislike status"})
	}

	return c.JSON(fiber.Map{"disliked": disliked})
}

// HandleAddFavorite adds a marker to the user's favorites.
//
// @Summary Add a marker to favorites
// @Description Allows the authenticated user to mark a specific marker as a favorite.
// @ID add-marker-favorite
// @Tags markers
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Favorite added successfully"
// @Failure 400 {object} map[string]string "Invalid marker ID"
// @Failure 403 {object} map[string]string "Maximum number of favorites reached"
// @Failure 500 {object} map[string]string "Failed to add favorite"
// @Router /api/v1/markers/{markerID}/favorites [post]
func (h *MarkerHandler) HandleAddFavorite(c *fiber.Ctx) error {
	userData, err := h.MarkerFacadeService.GetUserFromContext(c)
	if err != nil {
		return err // fiber err
	}

	// Extracting marker ID from request parameters or body
	markerIDParam := c.Params("markerID")
	markerID, err := strconv.Atoi(markerIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid marker ID",
		})
	}

	// Add favorite in the database
	err = h.MarkerFacadeService.AddFavorite(userData.UserID, markerID)
	if err != nil {
		// Respond differently based on the type of error
		if err.Error() == "maximum number of favorites reached" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	go func() {
		marker, markerErr := h.MarkerFacadeService.GetMarkerSimpleWithDescription(markerID)
		if markerErr != nil {
			//log.Printf("Failed to get one to cache: %v", markerErr)
			return
		}
		h.CacheService.AddSingleFavoriteToCache(userData.UserID, marker)
	}()

	// Successfully added the favorite
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Favorite added successfully",
	})
}

// HandleRemoveFavorite removes a marker from the user's favorites.
//
// @Summary Remove marker from favorites
// @Description Allows the authenticated user to remove a marker from their favorites.
// @ID remove-marker-favorite
// @Tags markers
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Security ApiKeyAuth
// @Success 204 "Favorite removed successfully (No Content)"
// @Failure 400 {object} map[string]string "Invalid marker ID or user ID not found"
// @Failure 500 {object} map[string]string "Failed to remove favorite"
// @Router /api/v1/markers/{markerID}/favorites [delete]
func (h *MarkerHandler) HandleRemoveFavorite(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(int)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "User ID not found"})
	}

	markerID, err := strconv.Atoi(c.Params("markerID"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid marker ID"})
	}

	err = h.MarkerFacadeService.RemoveFavorite(userID, markerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to remove marker favorite"})
	}

	go h.CacheService.RemoveMarkerFromFavorites(userID, markerID)

	return c.SendStatus(fiber.StatusNoContent) // 204 No Content is appropriate for a DELETE success with no response body
}

// HandleGetFacilities retrieves facilities for a specific marker.
//
// @Summary Get facilities by marker ID
// @Description Fetches a list of facilities available at a given marker location.
// @ID get-facilities
// @Tags markers-data
// @Accept json
// @Produce json
// @Security
// @Param markerID path int true "Marker ID"
// @Success 200 {array} model.Facility "List of facilities at the marker"
// @Failure 400 {object} map[string]string "Invalid Marker ID"
// @Failure 500 {object} map[string]string "Failed to retrieve facilities"
// @Router /api/v1/markers/{markerID}/facilities [get]
func (h *MarkerHandler) HandleGetFacilities(c *fiber.Ctx) error {
	markerID, err := strconv.Atoi(c.Params("markerID"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid Marker ID"})
	}

	// Attempt to retrieve from cache first
	cachedFacilities, cacheErr := h.CacheService.GetFacilitiesCache(markerID)
	if cacheErr == nil && cachedFacilities != nil {
		c.Append("X-Cache", "hit")
		return c.JSON(cachedFacilities)
	}

	facilities, err := h.MarkerFacadeService.GetFacilitiesByMarkerID(markerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve facilities"})
	}

	// Cache the result for future requests
	go h.CacheService.AddFacilitiesCache(markerID, facilities)

	return c.JSON(facilities)
}

// HandleSetMarkerFacilities sets facilities for a specific marker.
//
// @Summary Set marker facilities
// @Description Assigns a list of facilities to a given marker.
// @ID set-marker-facilities
// @Tags markers
// @Accept json
// @Produce json
// @Param request body dto.FacilityRequest true "Marker ID and facilities"
// @Security ApiKeyAuth
// @Success 200 "Facilities set successfully"
// @Failure 400 {object} map[string]string "Invalid request body"
// @Failure 500 {object} map[string]string "Failed to set facilities for marker"
// @Router /api/v1/markers/facilities [post]
func (h *MarkerHandler) HandleSetMarkerFacilities(c *fiber.Ctx) error {
	req := new(dto.FacilityRequest)
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse request"})
	}

	if err := h.MarkerFacadeService.SetMarkerFacilities(req.MarkerID, req.Facilities); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to set facilities for marker"})
	}

	return c.SendStatus(fiber.StatusOK)
}

// UpdateMarkersAddressesHandler handles the request to update all markers' addresses.
func (h *MarkerHandler) HandleUpdateMarkersAddresses(c *fiber.Ctx) error {
	updatedMarkers, err := h.MarkerFacadeService.UpdateMarkersAddresses()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update marker addresses",
		})
	}

	return c.JSON(fiber.Map{
		"message":        "Successfully updated marker addresses",
		"updatedMarkers": updatedMarkers,
	})
}

// HandleRSS retrieves the RSS feed of markers.
//
// @Summary Get markers RSS feed
// @Description Returns an RSS feed containing the latest marker updates.
// @ID get-markers-rss
// @Tags markers-data
// @Accept json
// @Produce text/xml; charset=utf-8
// @Success 200 {string} string "RSS feed of markers"
// @Failure 500 {string} string "Failed to read RSS feed file"
// @Router /api/v1/markers/rss [get]
func (h *MarkerHandler) HandleRSS(c *fiber.Ctx) error {
	// rss, err := h.MarkerFacadeService.GenerateRSS()
	// if err != nil {
	// 	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch RSS markers"})
	// }

	content, err := os.ReadFile("marker_rss.xml")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to read RSS feed file")
	}

	c.Set("Content-Type", "text/xml; charset=utf-8")
	// c.Type("text/xml", "utf-8")
	// c.Type("application/rss+xml", "utf-8")
	return c.SendString(string(content))
}

// HandleGetAllMarkers handles the HTTP request to get all markers
func (h *MarkerHandler) HandleRefreshMarkerCache(c *fiber.Ctx) error {
	// Fetch markers if cache is empty
	markers, err := h.MarkerFacadeService.GetAllMarkers() // []dto.MarkerSimple, err
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Marshal the markers to JSON for caching and response
	markersJSON, err := sonic.Marshal(markers)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encode markers"})
	}

	// Update cache
	h.MarkerFacadeService.SetMarkerCache(markersJSON)
	return c.SendString("refreshed")
}

// HandleVerifyMarker verifies if a marker location is valid.
//
// @Summary Verify marker location
// @Description Checks if a marker at the given latitude and longitude is valid.
// @ID verify-marker
// @Tags markers-util
// @Accept json
// @Produce json
// @Security
// @Param latitude query number true "Latitude in WGS84 format"
// @Param longitude query number true "Longitude in WGS84 format"
// @Success 200 {string} string "OK"
// @Failure 400 {object} map[string]string "Invalid query parameters or comment contains inappropriate content"
// @Failure 403 {object} map[string]string "Operation is only allowed within South Korea"
// @Failure 409 {object} map[string]string "There is a marker already nearby"
// @Failure 422 {object} map[string]string "Marker is in a restricted area"
// @Failure 500 {object} map[string]string "Internal server error during verification"
// @Router /api/v1/markers/verify [get]
func (h *MarkerHandler) HandleVerifyMarker(c *fiber.Ctx) error {
	lat, lng, err := GetLatLong(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	merr := h.MarkerFacadeService.CheckMarkerValidity(lat, lng, "")
	if merr != nil {
		return c.Status(merr.Code).JSON(fiber.Map{"error": merr.Error()})
	}

	return c.Status(fiber.StatusOK).SendString("OK")
}

// HandleGetRoadViewPicDate retrieves the date of the latest 카카오 road view picture for a given location.
//
// @Summary Get 카카오 road view picture date
// @Description Fetches the most recent 카카오 road view picture date for the given latitude and longitude.
// @ID get-roadview-pic-date
// @Tags markers-util
// @Accept json
// @Produce json
// @Param latitude query number true "Latitude in WGS84 format"
// @Param longitude query number true "Longitude in WGS84 format"
// @Success 200 {object} map[string]string "Date of the most recent road view picture" example: {"shot_date": "2023-05-10T14:00:00Z"}
// @Failure 400 {object} map[string]string "Invalid query parameters"
// @Failure 500 {object} map[string]string "Failed to fetch road view date"
// @Router /api/v1/markers/roadview-date [get]
func (h *MarkerHandler) HandleGetRoadViewPicDate(c *fiber.Ctx) error {
	lat, lng, err := GetLatLong(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	date, err := h.MarkerFacadeService.FacilityService.FetchRoadViewPicDate(lat, lng)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch road view date"})
	}

	return c.JSON(fiber.Map{"shot_date": date.Format(time.RFC3339)})
}

// HandleGetNewMarkers retrieves newly added markers after a given marker ID.
//
// @Summary Get new markers
// @Description Fetches a list of markers that were added after the given marker ID.
// @ID get-new-markers-after-id
// @Tags markers-data
// @Accept json
// @Produce json
// @Security
// @Param lastMarkerID query int false "Last known marker ID (default: 0)"
// @Success 200 {array} dto.MarkersKakaoBot "List of newly added markers"
// @Failure 500 {object} map[string]string "Failed to fetch markers"
// @Router /api/v1/markers/new-markers [get]
func (h *MarkerHandler) HandleGetNewMarkers(c *fiber.Ctx) error {
	lastMarkerIDStr := c.Query("lastMarkerID")
	lastMarkerID, err := strconv.Atoi(lastMarkerIDStr)
	if err != nil {
		lastMarkerID = 0
	}

	markers, err := h.MarkerFacadeService.InteractService.GetMarkersAfterID(lastMarkerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch markers" + err.Error()})
	}

	return c.JSON(markers)
}

// helpers

// From FORM
func GetLatLngFromForm(form *multipart.Form) (float64, float64, error) {
	latStr, latOk := form.Value["latitude"]
	longStr, longOk := form.Value["longitude"]
	if !latOk || !longOk || len(latStr[0]) == 0 || len(longStr[0]) == 0 {
		return 0, 0, errors.New("latitude and longitude are required")
	}

	latitude, err := strconv.ParseFloat(latStr[0], 64)
	if err != nil {
		return 0, 0, errors.New("invalid latitude")
	}

	longitude, err := strconv.ParseFloat(longStr[0], 64)
	if err != nil {
		return 0, 0, errors.New("invalid longitude")
	}

	return latitude, longitude, nil
}

// FROM Query parameters
func GetLatLong(c *fiber.Ctx) (float64, float64, error) {
	latParam := c.Query("latitude")
	longParam := c.Query("longitude")

	lat, err := strconv.ParseFloat(latParam, 64)
	if err != nil {
		return 0, 0, errors.New("invalid latitude")
	}

	long, err := strconv.ParseFloat(longParam, 64)
	if err != nil {
		return 0, 0, errors.New("invalid longitude")
	}

	// Korea rough check
	if lat < 32 || lat > 39 {
		return 0, 0, errors.New("invalid latitude (Must be between 32 and 39)")
	}

	if long < 123 || long > 133 {
		return 0, 0, errors.New("invalid longitude (Must be between 123 and 133)")
	}

	return lat, long, nil
}

func GetDescriptionFromForm(form *multipart.Form) string {
	if descValues, exists := form.Value["description"]; exists && len(descValues[0]) > 0 {
		return descValues[0]
	}
	return ""
}

func GetMarkerIDFromForm(form *multipart.Form) string {
	if descValues, exists := form.Value["markerId"]; exists && len(descValues[0]) > 0 {
		return descValues[0]
	}
	return ""
}
