package handler

import (
	"errors"
	"strconv"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/middleware"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
)

type CommentHandler struct {
	CommentService *service.MarkerCommentService
	Logger         *zap.Logger
	BadWordUtil    *util.BadWordUtil
}

// NewCommentHandler creates a new CommentHandler with dependencies injected
func NewCommentHandler(comment *service.MarkerCommentService, logger *zap.Logger, butil *util.BadWordUtil,
) *CommentHandler {
	return &CommentHandler{
		CommentService: comment,
		Logger:         logger,
		BadWordUtil:    butil,
	}
}

// RegisterCommentRoutes sets up the routes for comments handling within the application.
func RegisterCommentRoutes(api fiber.Router, handler *CommentHandler, authMiddleware *middleware.AuthMiddleware) {
	api.Get("/comments/:markerId/comments", handler.HandleLoadComments)

	commentGroup := api.Group("/comments")
	commentGroup.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			handler.Logger.Error("Panic recovered in comment API",
				zap.Any("error", e),
				zap.String("url", c.Path()),
				zap.String("method", c.Method()),
			)
		},
	}))

	{
		commentGroup.Use(authMiddleware.Verify)
		commentGroup.Post("", handler.HandlePostComment)
		commentGroup.Patch("/:commentId", handler.HandleUpdateComment)
		commentGroup.Delete("/:commentId", handler.HandleRemoveComment)
	}
}

// HandlePostComment creates a new comment on a marker.
//
// @Summary Post a comment
// @Description Allows an authenticated user to post a comment on a marker. Each user can post up to 3 comments per marker.
// @ID post-comment
// @Tags comments
// @Accept json
// @Produce json
// @Param request body dto.CommentRequest true "Comment request containing marker ID and comment text"
// @Security ApiKeyAuth
// @Success 200 {object} dto.CommentWithUsername "Comment created successfully"
// @Failure 400 {object} map[string]string "Invalid request body or maximum comments reached"
// @Failure 404 {object} map[string]string "Marker not found"
// @Failure 500 {object} map[string]string "Failed to create comment"
// @Router /api/v1/comments [post]
func (h *CommentHandler) HandlePostComment(c *fiber.Ctx) error {
	userID := c.Locals("userID").(int)
	userName := c.Locals("username").(string)
	var req dto.CommentRequest

	if err := util.JsonBodyParser(c, &req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	containsBadWord, _ := h.BadWordUtil.CheckForBadWordsUsingTrie(req.CommentText)
	if containsBadWord {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Comment contains inappropriate content."})
	}

	comment, err := h.CommentService.CreateComment(req.MarkerID, userID, userName, req.CommentText)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMaxCommentsReached):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "You have already commented 3 times on this marker"})
		case errors.Is(err, service.ErrMarkerNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Marker not found"})
		case errors.Is(err, service.ErrDailyCommentLimitReached):
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "일일 댓글 작성 한도(15개)에 도달했습니다. 내일 다시 시도해주세요"})
		case errors.Is(err, service.ErrCommentMax):
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "Comment creation limit exceeded"})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create comment"})
		}
	}

	return c.Status(fiber.StatusOK).JSON(comment)
}

// HandleUpdateComment updates a comment made by the authenticated user.
//
// @Summary Update a comment
// @Description Allows an authenticated user to update their own comment.
// @ID update-comment
// @Tags comments
// @Accept json
// @Produce json
// @Param commentId path int true "Comment ID"
// @Param request body map[string]string true "Updated comment text" example: {"commentText": "Updated comment content"}
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Comment updated successfully"
// @Failure 400 {object} map[string]string "Invalid comment ID or request body"
// @Failure 404 {object} map[string]string "Comment not found or not owned by user"
// @Failure 500 {object} map[string]string "Failed to update the comment"
// @Router /api/v1/comments/{commentId} [patch]
func (h *CommentHandler) HandleUpdateComment(c *fiber.Ctx) error {
	commentID, err := strconv.Atoi(c.Params("commentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid comment ID"})
	}

	// Extract userID and newCommentText from the request
	userID := c.Locals("userID").(int)
	var request struct {
		CommentText string `json:"commentText"`
	}
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// Call the service function to update the comment
	if err := h.CommentService.UpdateComment(commentID, userID, request.CommentText); err != nil {
		if err.Error() == "comment not found or not owned by user" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Failed to update the comment"})
		}
		// Handle other potential errors
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update the comment"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Comment updated successfully"})
}

// HandleRemoveComment deletes a comment made by the authenticated user.
//
// @Summary Delete a comment
// @Description Allows an authenticated user to delete their own comment.
// @ID delete-comment
// @Tags comments
// @Accept json
// @Produce json
// @Param commentId path int true "Comment ID"
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Comment removed successfully"
// @Failure 400 {object} map[string]string "Invalid comment ID"
// @Failure 404 {object} map[string]string "Comment not found or already deleted"
// @Failure 500 {object} map[string]string "Failed to remove comment"
// @Router /api/v1/comments/{commentId} [delete]
func (h *CommentHandler) HandleRemoveComment(c *fiber.Ctx) error {
	commentID, err := strconv.Atoi(c.Params("commentId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid comment ID"})
	}

	userID := c.Locals("userID").(int)

	err = h.CommentService.RemoveComment(commentID, userID)
	if err != nil {
		if err.Error() == "comment not found or already deleted" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "comment might not exist"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to remove comment"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Comment removed successfully"})
}

// HandleLoadComments retrieves paginated comments for a specific marker.
//
// @Summary Get comments for a marker
// @Description Fetches a paginated list of comments for a specific marker.
// @ID get-marker-comments
// @Tags comments, pagination
// @Accept json
// @Produce json
// @Param markerId path int true "Marker ID"
// @Param page query int false "Page number (default: 1)"
// @Param pageSize query int false "Number of comments per page (default: 4)"
// @Security
// @Success 200 {object} map[string]interface{} "Paginated list of comments"
// @Failure 400 {object} map[string]string "Invalid marker ID or pagination parameters"
// @Failure 500 {object} map[string]string "Failed to retrieve comments"
// @Router /api/v1/comments/{markerId}/comments [get]
func (h *CommentHandler) HandleLoadComments(c *fiber.Ctx) error {
	pagination, err := util.ParsePaginationParams(c, &util.PaginationConfig{
		DefaultPage:       1,
		DefaultPageSize:   4,
		PageParamName:     "page",
		PageSizeParamName: "pageSize",
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid pagination parameters"})
	}

	markerID, err := c.ParamsInt("markerId")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid marker ID provided",
		})
	}

	// Call service function to load comments for the marker
	comments, total, err := h.CommentService.LoadCommentsForMarker(markerID, pagination.PageSize, pagination.Offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	totalPages := total / pagination.PageSize
	if total%pagination.PageSize != 0 {
		totalPages++
	}

	return c.JSON(fiber.Map{
		"comments":      comments,
		"currentPage":   pagination.Page,
		"totalPages":    totalPages,
		"totalComments": total,
	})
}
