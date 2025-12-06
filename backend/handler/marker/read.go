package marker

import (
	"context"
	"math"
	"net/url"
	"strconv"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/protos"
	"github.com/Alfex4936/chulbong-kr/util"
	sonic "github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func (h *MarkerReadHandler) HandleGetAllMarkersProto(c *fiber.Ctx) error {
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
// @Router /api/v1/markers [get].
func (h *MarkerReadHandler) HandleGetAllMarkersLocal(c *fiber.Ctx) error {
	// Check the Referer header and redirect if it matches the specific URL pattern
	// if !strings.HasSuffix(c.Get("Referer"), ".k-pullup.com") || c.Get("Referer") != "https://www.k-pullup.com/" {
	// 	return c.Redirect("https://k-pullup.com", fiber.StatusFound) // Use HTTP 302 for standard redirection
	// }
	c.Set("Content-type", "application/json")

	reqCtx := c.UserContext()
	if reqCtx == nil {
		reqCtx = context.Background()
	}

	payload, fromCache, err := h.CacheService.GetOrLoadAllMarkers(reqCtx, func(ctx context.Context) ([]byte, error) {
		markers, err := h.MarkerFacadeService.GetAllMarkers()
		if err != nil {
			return nil, err
		}

		if len(markers) == 0 {
			return []byte("[]"), nil
		}

		return sonic.ConfigFastest.Marshal(markers)
	})

	if err != nil {
		h.logger.Error("Failed to get markers", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get markers"})
	}

	if fromCache {
		c.Append("X-Cache", "hit")
	} else {
		c.Append("X-Cache", "miss")
	}

	return c.Send(payload)
}

func (h *MarkerReadHandler) HandleGetAllMarkersLocalMsgp(c *fiber.Ctx) error {
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
func (h *MarkerReadHandler) HandleGetMarker(c *fiber.Ctx) error {
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

	var userIDPtr *int
	if userOK {
		userIDPtr = &userID
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

	reactions, err := h.MarkerFacadeService.GetMarkerReactionSummary(markerID, userIDPtr)
	if err != nil {
		h.logger.Error("Failed to load marker reactions", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load reactions"})
	}
	marker.Reactions = reactions

	go h.MarkerFacadeService.BufferClickEvent(markerID)
	// go h.MarkerFacadeService.SaveUniqueVisitor(c.Params("markerId"), c)
	return c.JSON(marker)
}

func (h *MarkerReadHandler) HandleGetAllMarkersWithAddr(c *fiber.Ctx) error {
	markersWithPhotos, err := h.MarkerFacadeService.GetAllMarkersWithAddr()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(markersWithPhotos)
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
func (h *MarkerReadHandler) HandleGetFacilities(c *fiber.Ctx) error {
	markerID, err := ParseMarkerIDParam(c, "markerID")
	if err != nil {
		return err
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

// HandleGetMarkersByUsername retrieves a paginated list of markers created by a specific user identified by username.
//
// @Summary Get markers by username
// @Description Fetches a paginated list of markers created by a user identified by their username. This is a public endpoint that does not require authentication.
// @ID get-markers-by-username
// @Tags markers-data, pagination
// @Accept json
// @Produce json
// @Param username path string true "Username of the user whose markers to retrieve"
// @Param page query int false "Page number (default: 1)"
// @Param pageSize query int false "Number of markers per page (default: 10, max: 50)"
// @Success 200 {object} dto.UserMarkers "List of user's markers with pagination"
// @Failure 400 {object} map[string]string "Invalid username or pagination parameters"
// @Failure 404 {object} map[string]string "User not found"
// @Failure 500 {object} map[string]string "Failed to get markers"
// @Router /api/v1/markers/user/{username} [get]
func (h *MarkerReadHandler) HandleGetMarkersByUsername(c *fiber.Ctx) error {
	username := c.Params("username")
	if username == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Username is required"})
	}

	// URL decode the username to handle Korean characters and other special characters
	decodedUsername, err := url.QueryUnescape(username)
	if err != nil {
		// If decoding fails, use the original username
		decodedUsername = username
	}

	pagination, err := util.ParsePaginationParams(c, &util.PaginationConfig{
		DefaultPage:       1,
		DefaultPageSize:   10,
		PageParamName:     "page",
		PageSizeParamName: "pageSize",
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid pagination parameters"})
	}

	page := pagination.Page
	pageSize := pagination.PageSize

	// Limit maximum pageSize to prevent abuse
	if pageSize > 50 {
		pageSize = 50
	}

	// Get markers by username
	markersWithPhotos, total, err := h.MarkerFacadeService.GetAllMarkersByUsernameWithPagination(decodedUsername, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get markers"})
	}

	// If no markers found and total is 0, user might not exist or has no markers
	if len(markersWithPhotos) == 0 && total == 0 {
		return c.Status(fiber.StatusOK).JSON(dto.UserMarkers{
			MarkersWithPhotos: []dto.MarkerSimpleWithDescription{},
			CurrentPage:       page,
			TotalPages:        0,
			TotalMarkers:      0,
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	// Prepare the response
	response := dto.UserMarkers{
		MarkersWithPhotos: markersWithPhotos,
		CurrentPage:       page,
		TotalPages:        totalPages,
		TotalMarkers:      total,
	}

	// Return the response
	return c.JSON(response)
}
