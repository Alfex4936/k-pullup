package handler

import (
	"errors"
	"mime/multipart"
	"strconv"
	"strings"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/gofiber/fiber/v2"
)

// HandleAddStory adds a new story to a specific marker.
//
// @Summary Add a story to a marker
// @Description Allows the authenticated user to add a story with a photo to a specific marker.
// @ID add-marker-story
// @Tags stories
// @Accept multipart/form-data
// @Produce json
// @Param markerID path int true "Marker ID"
// @Param caption formData string false "Story caption (max 30 characters)"
// @Param photo formData file true "Photo for the story"
// @Security ApiKeyAuth
// @Success 201 {object} dto.StoryResponse "Story added successfully"
// @Failure 400 {object} map[string]string "Invalid marker ID, form data, or missing required fields"
// @Failure 409 {object} map[string]string "Story already posted"
// @Failure 500 {object} map[string]string "Failed to add story"
// @Router /api/v1/markers/{markerID}/stories [post]
func (h *MarkerHandler) HandleAddStory(c *fiber.Ctx) error {
	markerIDParam := c.Params("markerID")
	markerID, err := strconv.Atoi(markerIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid marker ID"})
	}

	userID := c.Locals("userID").(int)

	// Parse the multipart form data
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Failed to parse form"})
	}

	caption := ""
	if captions, ok := form.Value["caption"]; ok && len(captions) > 0 {
		caption = captions[0]
		if len(caption) > 30 {
			caption = caption[:30]
		}
	}

	caption, _ = h.MarkerFacadeService.BadWordUtil.ReplaceBadWords(caption)

	// Get the photo
	var photo *multipart.FileHeader
	if photos, ok := form.File["photo"]; ok && len(photos) > 0 {
		photo = photos[0]
	} else {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Photo is required"})
	}

	// Call the service to add the story
	storyResponse, err := h.MarkerFacadeService.StoryService.AddStory(markerID, userID, caption, photo)
	if err != nil {
		if errors.Is(err, service.ErrAlreadyStoryPost) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Story already posted"}) // 409
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to add story"})
	}

	return c.Status(fiber.StatusCreated).JSON(storyResponse)
}

// HandleGetStories retrieves a paginated list of stories for a specific marker.
//
// @Summary Get stories for a marker
// @Description Fetches a paginated list of stories associated with a specific marker.
// @ID get-marker-stories
// @Tags stories, pagination
// @Accept json
// @Produce json
// @Security
// @Param markerID path int true "Marker ID"
// @Param page query int false "Page number (default: 1)"
// @Param pageSize query int false "Number of stories per page (default: 30)"
// @Success 200 {array} dto.StoryResponseOneMarker "List of stories for the marker. Each story includes:
//   - ThumbsUp: total number of 'thumbsup' reactions
//   - ThumbsDown: total number of 'thumbsdown' reactions
//   - UserLiked: boolean indicating if the currently logged-in user has liked the story (only true if user is logged in and has liked the story)."
//
// @Failure 400 {object} map[string]string "Invalid marker ID or pagination parameters"
// @Failure 500 {object} map[string]string "Failed to get stories"
// @Router /api/v1/markers/{markerID}/stories [get]
func (h *MarkerHandler) HandleGetStories(c *fiber.Ctx) error {
	markerIDParam := c.Params("markerID")
	markerID, err := strconv.Atoi(markerIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid marker ID"})
	}

	userID, _ := c.Locals("userID").(int) // default 0 = not logged in

	pagination, err := util.ParsePaginationParams(c, &util.PaginationConfig{
		DefaultPage:       1,
		DefaultPageSize:   30,
		PageParamName:     "page",
		PageSizeParamName: "pageSize",
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid pagination parameters"})
	}

	// Call the service to get stories
	stories, err := h.MarkerFacadeService.StoryService.GetStories(userID, markerID, pagination.Offset, pagination.PageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get stories"})
	}

	return c.JSON(stories)
}

// HandleGetAllStories retrieves a paginated list of all marker stories.
//
// @Summary Get all stories
// @Description Fetches a paginated list of stories associated with markers.
// @ID get-all-stories
// @Tags stories, pagination
// @Accept json
// @Produce json
// @Security
// @Param page query int false "Page number (default: 1)"
// @Param pageSize query int false "Number of stories per page (default: 10)"
// @Success 200 {array} dto.StoryResponse "List of marker stories"
// @Failure 400 {object} map[string]string "Invalid pagination parameters"
// @Failure 500 {object} map[string]string "Failed to get stories"
// @Router /api/v1/markers/stories [get]
func (h *MarkerHandler) HandleGetAllStories(c *fiber.Ctx) error {
	pagination, err := util.ParsePaginationParams(c, &util.PaginationConfig{
		DefaultPage:       1,
		DefaultPageSize:   10,
		PageParamName:     "page",
		PageSizeParamName: "pageSize",
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid pagination parameters"})
	}

	// Call the service to get all stories
	stories, err := h.MarkerFacadeService.StoryService.GetAllStories(pagination.Page, pagination.PageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get stories"})
	}

	return c.JSON(stories)
}

// HandleDeleteStory deletes a specific story from a marker.
//
// @Summary Delete a story from a marker
// @Description Allows the authenticated user (owner or admin) to delete a story from a specific marker.
// @ID delete-marker-story
// @Tags stories
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Param storyID path int true "Story ID"
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Story deleted successfully"
// @Failure 400 {object} map[string]string "Invalid marker ID or story ID"
// @Failure 401 {object} map[string]string "User is not authorized to delete this story"
// @Failure 404 {object} map[string]string "Story not found"
// @Failure 500 {object} map[string]string "Failed to delete story"
// @Router /api/v1/markers/{markerID}/stories/{storyID} [delete]
func (h *MarkerHandler) HandleDeleteStory(c *fiber.Ctx) error {
	markerIDParam := c.Params("markerID")
	storyIDParam := c.Params("storyID")

	markerID, err := strconv.Atoi(markerIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid marker ID"})
	}
	storyID, err := strconv.Atoi(storyIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid story ID"})
	}

	userRole := c.Locals("role").(string)
	userID := c.Locals("userID").(int)

	// Call the service to delete the story
	err = h.MarkerFacadeService.StoryService.DeleteStory(markerID, storyID, userID, userRole)
	if err != nil {
		if err == service.ErrUnauthorized {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "You are not authorized to delete this story"})
		} else if err == service.ErrStoryNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Story not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete story"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Story deleted"})
}

// HandleAddReaction adds a reaction (thumbs up/down) to a story.
//
// @Summary Add a reaction to a story
// @Description Allows the authenticated user to react to a story with either a thumbs up or thumbs down.
// @ID add-story-reaction
// @Tags stories
// @Accept json
// @Produce json
// @Param storyID path int true "Story ID"
// @Param request body dto.ReactionRequest true "Reaction type (thumbsup or thumbsdown)"
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Reaction added successfully"
// @Failure 400 {object} map[string]string "Invalid story ID or request body"
// @Failure 500 {object} map[string]string "Failed to add reaction"
// @Router /api/v1/markers/stories/{storyID}/reactions [post]
func (h *MarkerHandler) HandleAddReaction(c *fiber.Ctx) error {
	storyIDParam := c.Params("storyID")
	storyID, err := strconv.Atoi(storyIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid story ID"})
	}

	userID := c.Locals("userID").(int)

	// Parse the request body to get the reaction type
	var reactionRequest dto.ReactionRequest
	if err := c.BodyParser(&reactionRequest); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if reactionRequest.ReactionType != "thumbsup" && reactionRequest.ReactionType != "thumbsdown" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid reaction type"})
	}

	// Call the service to add the reaction
	err = h.MarkerFacadeService.StoryService.AddReaction(storyID, userID, reactionRequest.ReactionType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to add reaction" + err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Reaction added"})
}

// HandleRemoveReaction removes a reaction from a story.
//
// @Summary Remove a reaction from a story
// @Description Allows the authenticated user to remove their reaction from a specific story.
// @ID remove-story-reaction
// @Tags stories
// @Accept json
// @Produce json
// @Param storyID path int true "Story ID"
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Reaction removed successfully"
// @Failure 400 {object} map[string]string "Invalid story ID"
// @Failure 500 {object} map[string]string "Failed to remove reaction"
// @Router /api/v1/markers/stories/{storyID}/reactions [delete]
func (h *MarkerHandler) HandleRemoveReaction(c *fiber.Ctx) error {
	storyIDParam := c.Params("storyID")
	storyID, err := strconv.Atoi(storyIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid story ID"})
	}

	userID := c.Locals("userID").(int)

	// Call the service to remove the reaction
	err = h.MarkerFacadeService.StoryService.RemoveReaction(storyID, userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to remove reaction" + err.Error()})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Reaction removed"})
}

// HandleReportStory reports a story for inappropriate content.
//
// @Summary Report a story
// @Description Allows the authenticated user to report a story for violating community guidelines.
// @ID report-story
// @Tags stories
// @Accept json
// @Produce json
// @Param storyID path int true "Story ID"
// @Param request body map[string]string true "Report reason" example: {"reason": "Inappropriate content"}
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Story reported successfully"
// @Failure 400 {object} map[string]string "Invalid story ID or request body"
// @Failure 404 {object} map[string]string "Story not found"
// @Failure 409 {object} map[string]string "User has already reported this story"
// @Failure 500 {object} map[string]string "Failed to report story"
// @Router /api/v1/markers/stories/{storyID}/report [post]
func (h *MarkerHandler) HandleReportStory(c *fiber.Ctx) error {
	storyIDParam := c.Params("storyID")
	storyID, err := strconv.Atoi(storyIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid story ID"})
	}

	userID := c.Locals("userID").(int)

	// Get reason from request body
	var reportRequest struct {
		Reason string `json:"reason"`
	}
	if err := c.BodyParser(&reportRequest); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if len(reportRequest.Reason) > 255 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Reason is too long"})
	}

	// Call the service to report the story
	err = h.MarkerFacadeService.StoryService.ReportStory(storyID, userID, reportRequest.Reason)
	if err != nil {
		if errors.Is(err, service.ErrStoryNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Story not found"})
		}
		// Handle duplicate report error
		if strings.Contains(err.Error(), "duplicate entry") {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "You have already reported this story"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to report story"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Story reported"})
}
