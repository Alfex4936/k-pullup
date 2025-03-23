package handler

import (
	"github.com/gofiber/fiber/v2"
)

// HandleGetMarkerRanking retrieves the top-ranked markers.
//
// @Summary Get marker ranking
// @Description Fetches the top 50 markers based on click count.
// @ID get-marker-ranking
// @Tags ranking
// @Accept json
// @Produce json
// @Security
// @Success 200 {array} dto.MarkerSimpleWithAddr "List of top-ranked markers"
// @Failure 500 {object} map[string]string "Failed to retrieve marker ranking"
// @Router /api/v1/markers/ranking [get]
func (h *MarkerHandler) HandleGetMarkerRanking(c *fiber.Ctx) error {
	ranking := h.MarkerFacadeService.GetTopMarkers(50)

	return c.JSON(ranking)
}

func (h *MarkerHandler) HandleGetUniqueVisitorCount(c *fiber.Ctx) error {
	markerID := c.Query("markerId")
	if markerID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid Marker ID"})
	}

	count := h.MarkerFacadeService.GetUniqueVisitorCount(markerID)

	return c.JSON(fiber.Map{"markerId": markerID, "visitors": count})
}

func (h *MarkerHandler) HandleGetAllUniqueVisitorCount(c *fiber.Ctx) error {
	count := h.MarkerFacadeService.GetAllUniqueVisitorCounts()
	return c.JSON(count)
}
