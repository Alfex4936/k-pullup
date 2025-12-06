package marker

import (
	"math"
	"strconv"
	"strings"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/gofiber/fiber/v2"
)

var allowedMarkerReactions = map[string]struct{}{
	"clean":   {},
	"crowded": {},
	"broken":  {},
	"busy":    {},
	"calm":    {},
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
func (h *MarkerInteractionHandler) HandleLeaveDislike(c *fiber.Ctx) error {
	// Auth
	userID := c.Locals("userID").(int)

	markerID, err := ParseMarkerIDParam(c, "markerID")
	if err != nil {
		return err
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
func (h *MarkerInteractionHandler) HandleUndoDislike(c *fiber.Ctx) error {
	// Auth
	userID := c.Locals("userID").(int)

	markerID, err := ParseMarkerIDParam(c, "markerID")
	if err != nil {
		return err
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
func (h *MarkerInteractionHandler) HandleGetUserMarkers(c *fiber.Ctx) error {
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
func (h *MarkerInteractionHandler) HandleCheckDislikeStatus(c *fiber.Ctx) error {
	userID := c.Locals("userID").(int)
	markerID, err := ParseMarkerIDParam(c, "markerID")
	if err != nil {
		return err
	}

	disliked, err := h.MarkerFacadeService.CheckUserDislike(userID, markerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "error checking dislike status"})
	}

	return c.JSON(fiber.Map{"disliked": disliked})
}

// HandleSetMarkerReaction upserts the user's reaction for a marker.
//
// @Summary React to a marker
// @Description Adds or updates the authenticated user's reaction for the marker.
// @ID set-marker-reaction
// @Tags markers
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Param request body dto.ReactionRequest true "Reaction payload"
// @Security ApiKeyAuth
// @Success 200 {object} model.MarkerReactionSummary "Updated reaction summary"
// @Failure 400 {object} map[string]string "Invalid marker ID or reaction type"
// @Failure 500 {object} map[string]string "Failed to save reaction"
// @Router /api/v1/markers/{markerID}/reactions [post]
func (h *MarkerInteractionHandler) HandleSetMarkerReaction(c *fiber.Ctx) error {
	userID := c.Locals("userID").(int)
	markerID, err := ParseMarkerIDParam(c, "markerID")
	if err != nil {
		return err
	}

	var req dto.ReactionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	reaction := strings.ToLower(req.ReactionType)
	if _, ok := allowedMarkerReactions[reaction]; !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid reaction type"})
	}

	if err := h.MarkerFacadeService.SetMarkerReaction(userID, markerID, reaction); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save reaction"})
	}

	summary, err := h.MarkerFacadeService.GetMarkerReactionSummary(markerID, &userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load reactions"})
	}

	return c.JSON(summary)
}

// HandleRemoveMarkerReaction deletes the user's reaction for a marker.
//
// @Summary Remove marker reaction
// @Description Removes the authenticated user's reaction from the marker.
// @ID remove-marker-reaction
// @Tags markers
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Security ApiKeyAuth
// @Success 200 {object} model.MarkerReactionSummary "Updated reaction summary"
// @Failure 400 {object} map[string]string "Invalid marker ID"
// @Failure 500 {object} map[string]string "Failed to remove reaction"
// @Router /api/v1/markers/{markerID}/reactions [delete]
func (h *MarkerInteractionHandler) HandleRemoveMarkerReaction(c *fiber.Ctx) error {
	userID := c.Locals("userID").(int)
	markerID, err := ParseMarkerIDParam(c, "markerID")
	if err != nil {
		return err
	}

	if err := h.MarkerFacadeService.RemoveMarkerReaction(userID, markerID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to remove reaction"})
	}

	summary, err := h.MarkerFacadeService.GetMarkerReactionSummary(markerID, &userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load reactions"})
	}

	return c.JSON(summary)
}

// HandleGetMarkerReactions returns aggregated reactions (and optionally the caller's reaction).
//
// @Summary Get marker reactions
// @Description Returns aggregated reaction counts for a marker and the current user's reaction if authenticated.
// @ID get-marker-reactions
// @Tags markers-data
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Security ApiKeyAuth
// @Success 200 {object} model.MarkerReactionSummary "Reaction summary"
// @Failure 400 {object} map[string]string "Invalid marker ID"
// @Failure 500 {object} map[string]string "Failed to load reactions"
// @Router /api/v1/markers/{markerID}/reactions [get]
func (h *MarkerInteractionHandler) HandleGetMarkerReactions(c *fiber.Ctx) error {
	markerID, err := ParseMarkerIDParam(c, "markerID")
	if err != nil {
		return err
	}

	var userIDPtr *int
	if userID, ok := c.Locals("userID").(int); ok {
		userIDPtr = &userID
	}

	summary, err := h.MarkerFacadeService.GetMarkerReactionSummary(markerID, userIDPtr)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load reactions"})
	}

	return c.JSON(summary)
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
func (h *MarkerInteractionHandler) HandleAddFavorite(c *fiber.Ctx) error {
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
func (h *MarkerInteractionHandler) HandleRemoveFavorite(c *fiber.Ctx) error {
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
