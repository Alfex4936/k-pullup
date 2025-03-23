package handler

import (
	"strings"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/service"

	"github.com/gofiber/fiber/v2"
)

type SearchHandler struct {
	SearchService      *service.ZincSearchService
	BleveSearchService *service.BleveSearchService
}

// NewSearchHandler creates a new SearchHandler with dependencies injected
func NewSearchHandler(
	zinc *service.ZincSearchService,
	bleve *service.BleveSearchService,
) *SearchHandler {
	return &SearchHandler{
		SearchService:      zinc,
		BleveSearchService: bleve,
	}
}

// RegisterSearchRoutes sets up the routes for search handling within the application.
func RegisterSearchRoutes(api fiber.Router, handler *SearchHandler) {
	searchGroup := api.Group("/search")
	{
		searchGroup.Get("/marker", handler.HandleBleveSearchMarkerAddress)
		searchGroup.Get("/autocomplete", handler.HandleAutoComplete)
		searchGroup.Get("/station", handler.HandleGeoSearchByStation)
		// searchGroup.Get("/marker-zinc", handler.HandleSearchMarkerAddress)
		// searchGroup.Post("/marker", handler.HandleInsertMarkerAddressTest)
		// searchGroup.Delete("", handler.HandleDeleteMarkerAddressTest)
	}
}

// Handler for searching marker addresses
func (h *SearchHandler) HandleSearchMarkerAddress(c *fiber.Ctx) error {
	term := c.Query("term")
	term = strings.TrimSpace(term)
	if term == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "search term is required",
		})
	}

	// Call the service function
	response, err := h.SearchService.SearchMarkerAddress(term)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

// HandleBleveSearchMarkerAddress searches for markers by address using Bleve.
//
// @Summary Search marker by address
// @Description Searches for markers matching the given address term using the Bleve search engine.
// @ID search-marker-address
// @Tags markers-search
// @Accept json
// @Produce json
// @Security
// @Param term query string true "Search term for the marker address"
// @Success 200 {array} dto.MarkerSearchResponse "List of matching markers"
// @Failure 400 {object} map[string]string "Search term is required"
// @Failure 500 {object} map[string]string "Failed to execute search"
// @Router /api/v1/search/marker [get]
func (h *SearchHandler) HandleBleveSearchMarkerAddress(c *fiber.Ctx) error {
	term := c.Query("term")
	term = strings.TrimSpace(term)
	if term == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "search term is required",
		})
	}

	// Call the service function
	response, err := h.BleveSearchService.SearchMarkerAddress(term)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

// HandleAutoComplete provides autocomplete suggestions for marker addresses.
//
// @Summary Autocomplete marker addresses
// @Description Returns a list of autocomplete suggestions for marker addresses based on the search term.
// @ID autocomplete-marker-address
// @Tags markers-search
// @Accept json
// @Produce json
// @Security
// @Param term query string true "Search term for autocomplete suggestions"
// @Success 200 {array} string "List of autocomplete suggestions"
// @Failure 400 {object} map[string]string "Search term is required"
// @Failure 500 {object} map[string]string "Failed to fetch autocomplete results"
// @Router /api/v1/search/autocomplete [get]
func (h *SearchHandler) HandleAutoComplete(c *fiber.Ctx) error {
	term := c.Query("term")
	term = strings.TrimSpace(term)
	if term == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "search term is required",
		})
	}

	// Call the service function
	response, err := h.BleveSearchService.AutoComplete(term)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

// HandleGeoSearchByStation searches for markers near a subway station.
//
// @Summary Search markers by station
// @Description Finds markers located near a specified subway station. The search term must end with '역' (station).
// @ID search-markers-by-station
// @Tags markers-search
// @Accept json
// @Produce json
// @Security
// @Param term query string true "Subway station name (automatically appends '역' if missing)"
// @Success 200 {object} dto.MarkerSearchResponse "List of markers near the specified station"
// @Failure 400 {object} map[string]string "Search term is required"
// @Failure 500 {object} map[string]string "Failed to fetch search results"
// @Router /api/v1/search/station [get]
func (h *SearchHandler) HandleGeoSearchByStation(c *fiber.Ctx) error {
	term := c.Query("term")
	term = strings.TrimSpace(term)
	if term == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "search term is required",
		})
	}

	if !strings.HasSuffix(term, "역") {
		term = term + "역"
	}

	// Call the service function
	response, err := h.BleveSearchService.SearchMarkersNearLocation(term)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).JSON(response)
}

// Handler for searching marker addresses
func (h *SearchHandler) HandleInsertMarkerAddressTest(c *fiber.Ctx) error {
	// Call the service function
	err := h.SearchService.InsertMarkerIndex(dto.MarkerIndexData{MarkerID: 9999, Address: "test address"})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).SendString("success")
}
func (h *SearchHandler) HandleDeleteMarkerAddressTest(c *fiber.Ctx) error {
	markerID := c.Query("markerId")
	markerID = strings.TrimSpace(markerID)
	if markerID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "search term is required",
		})
	}

	// Call the service function
	err := h.SearchService.DeleteMarkerIndex(markerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusOK).SendString("success")
}
