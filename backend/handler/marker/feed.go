package marker

import (
	"os"
	"strconv"

	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/gofiber/fiber/v2"
)

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
func (h *MarkerFeedHandler) HandleGet10NewPictures(c *fiber.Ctx) error {
	markers, err := h.MarkerFacadeService.GetNew10Pictures()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to fetch markers " + err.Error(),
		})
	}
	return c.JSON(markers)
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
func (h *MarkerFeedHandler) HandleGetAllNewMarkers(c *fiber.Ctx) error {
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
func (h *MarkerFeedHandler) HandleRSS(c *fiber.Ctx) error {
	content, err := os.ReadFile("marker_rss.xml")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to read RSS feed file")
	}

	c.Set("Content-Type", "text/xml; charset=utf-8")
	// c.Type("text/xml", "utf-8")
	// c.Type("application/rss+xml", "utf-8")
	return c.SendString(string(content))
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
func (h *MarkerFeedHandler) HandleGetNewMarkers(c *fiber.Ctx) error {
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
