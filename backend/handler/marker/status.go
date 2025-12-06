package marker

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/util"
	sonic "github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
)

const WEATHER_MINUTES = 15 * time.Minute

// HandleFindCloseMarkers retrieves markers that are close to a specified location within a given distance.
//
// @Summary Find close markers
// @Description Retrieves markers that are within the specified distance from a given latitude and longitude.
// @Description The maximum allowed distance is 50km.
// @ID find-close-markers
// @Tags markers-data, pagination
// @Accept json
// @Produce json
// @Security
// @Param latitude query number true "Latitude of the location (float)"
// @Param longitude query number true "Longitude of the location (float)"
// @Param distance query int true "Search radius distance (meters), maximum 50,000m"
// @Param pageSize query int false "Number of markers per page (default: 4)"
// @Param page query int true "Page index number (default: 1)"
// @Success 200 {object} dto.MarkersClose "Markers found successfully with pagination"
// @Failure 400 {object} map[string]string "Invalid query or pagination parameters"
// @Failure 403 {object} map[string]string "Distance cannot exceed 50,000m (50km)"
// @Failure 404 {object} map[string]string "No markers found within the specified distance"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /api/v1/markers/close [get]
func (h *MarkerStatusHandler) HandleFindCloseMarkers(c *fiber.Ctx) error {
	var params dto.QueryParams
	if err := c.QueryParser(&params); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid query parameters"})
	}

	if params.Distance > 50000 {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Distance cannot be greater than 50,000m (50km)"})
	}

	pagination, err := util.ParsePaginationParams(c, &util.PaginationConfig{
		DefaultPage:       1,
		DefaultPageSize:   4,
		PageParamName:     "page",
		PageSizeParamName: "pageSize",
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid pagination parameters"})
	}

	page := pagination.Page
	pageSize := pagination.PageSize

	cacheKey := fmt.Sprintf("close_markers:%f:%f:%d:%d:%d", params.Latitude, params.Longitude, params.Distance, page, pageSize)

	cachedData, err := h.CacheService.GetCloseMarkersCache(cacheKey)
	if err == nil && len(cachedData) > 0 {
		c.Append("X-Cache", "hit")
		return c.Send(cachedData)
	}

	markers, total, err := h.MarkerFacadeService.FindClosestNMarkersWithinDistance(params.Latitude, params.Longitude, params.Distance, pageSize, pagination.Offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve markers"})
	}

	totalPages := total / pageSize
	if total%pageSize != 0 {
		totalPages++
	}

	if page > totalPages {
		page = totalPages
	}
	if page < 1 {
		page = 1
	}

	response := dto.MarkersClose{
		Markers:      markers,
		CurrentPage:  page,
		TotalPages:   totalPages,
		TotalMarkers: total,
	}

	responseJSON, err := sonic.Marshal(response)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encode response"})
	}

	go h.CacheService.SetCloseMarkersCache(cacheKey, responseJSON, 10*time.Minute)

	return c.Send(responseJSON)
}

// HandleGetCurrentAreaMarkerRanking retrieves top-ranked markers in the current area.
//
// @Summary Get ranked markers in the current area
// @Description Fetches a list of markers ranked by popularity within a 10km radius from the provided coordinates.
// @ID get-current-area-marker-ranking
// @Tags ranking
// @Accept json
// @Produce json
// @Param latitude query number true "Latitude of the current location"
// @Param longitude query number true "Longitude of the current location"
// @Param limit query int false "Number of markers to return (default: 10)"
// @Success 200 {array} dto.MarkerWithDistanceAndPhoto "List of ranked markers within the area"
// @Failure 400 {object} map[string]string "Invalid query parameters"
// @Failure 500 {object} map[string]string "Failed to retrieve ranked markers"
// @Router /api/v1/markers/area-ranking [get]
func (h *MarkerFeedHandler) HandleGetCurrentAreaMarkerRanking(c *fiber.Ctx) error {
	limitParam := c.Query("limit", "10")
	lat, lng, err := GetLatLong(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	limit, err := strconv.Atoi(limitParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid limit"})
	}

	const currentAreaDistance = 10000 // meters

	markers, err := h.MarkerFacadeService.FindRankedMarkersInCurrentArea(lat, lng, currentAreaDistance, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve markers"})
	}

	if markers == nil {
		return c.JSON([]dto.MarkerWithDistance{})
	}

	return c.JSON(markers)
}

func (h *MarkerStatusHandler) HandleGetMarkersClosebyAdmin(c *fiber.Ctx) error {
	markers, err := h.MarkerFacadeService.CheckNearbyMarkersInDB()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve markers: " + err.Error()})
	}

	return c.JSON(markers)
}

// HandleGetWeatherByWGS84 retrieves weather data for a given WGS84 coordinate.
// The weather data is fetched from the Kakao Weather API and cached for 15 minutes.
//
// @Summary Get weather by WGS84 coordinates
// @Description Fetches weather information based on the provided latitude and longitude.
// @ID get-weather-by-wgs84
// @Tags markers-data
// @Accept json
// @Produce json
// @Security
// @Param latitude query number true "Latitude in WGS84 format"
// @Param longitude query number true "Longitude in WGS84 format"
// @Success 200 {object} kakao.WeatherRequest "Weather information for the given location"
// @Failure 400 {object} map[string]string "Invalid query parameters"
// @Failure 409 {object} map[string]string "Failed to fetch weather data"
// @Router /api/v1/markers/weather [get]
func (h *MarkerStatusHandler) HandleGetWeatherByWGS84(c *fiber.Ctx) error {
	// Check the Referer header and redirect if it matches the specific URL pattern
	// if !strings.HasSuffix(c.Get("Referer"), ".k-pullup.com") || c.Get("Referer") != "https://www.k-pullup.com/" {
	// 	return c.Redirect("https://k-pullup.com", fiber.StatusFound) // Use HTTP 302 for standard redirection
	// }

	lat, lng, err := GetLatLong(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to get latitude and longitude"})
	}

	weather, cacheErr := h.CacheService.GetWcongCache(lat, lng)
	if cacheErr == nil && weather != nil {
		c.Append("X-Cache", "hit")
		return c.JSON(weather)
	}

	result, err := h.MarkerFacadeService.FetchWeatherFromAddress(lat, lng)
	if err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "failed to fetch weather from address"})
	}

	go h.CacheService.SetWcongCache(lat, lng, result)

	return c.JSON(result)
}

// HandleConvertWGS84ToWCONGNAMUL converts WGS84 coordinates to WCONGNAMUL.
//
// @Summary Convert WGS84 to WCONGNAMUL
// @Description Converts latitude and longitude from WGS84 format to WCONGNAMUL coordinate system.
// @ID convert-wgs84-to-wcongnamul
// @Tags markers-util
// @Accept json
// @Produce json
// @Security
// @Param latitude query number true "Latitude in WGS84 format"
// @Param longitude query number true "Longitude in WGS84 format"
// @Success 200 {object} util.WCONGNAMULCoord "Converted coordinates in WCONGNAMUL format"
// @Failure 400 {object} map[string]string "Invalid query parameters"
// @Router /api/v1/markers/convert [get]
func (h *MarkerStatusHandler) HandleConvertWGS84ToWCONGNAMUL(c *fiber.Ctx) error {
	lat, lng, err := GetLatLong(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	result := util.ConvertWGS84ToWCONGNAMUL(lat, lng)

	return c.JSON(result)
}

func (h *MarkerStatusHandler) HandleIsInSouthKorea(c *fiber.Ctx) error {
	lat, lng, err := GetLatLong(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	result := h.MarkerFacadeService.IsInSouthKoreaPrecisely(lat, lng)

	return c.JSON(fiber.Map{"result": result})
}

// HandleSaveOfflineMap2 generates and downloads an offline map as a PDF.
//
// @Summary Save offline map
// @Description Generates an offline map for the specified location and provides a downloadable PDF.
// @Description Rate limit: Maximum 5 requests per minute per IP.
// @ID save-offline-map
// @Tags markers-data
// @Accept json
// @Produce application/pdf
// @Security
// @Param latitude query number true "Latitude in WGS84 format"
// @Param longitude query number true "Longitude in WGS84 format"
// @Success 200 {file} application/pdf "Generated PDF map"
// @Failure 204 {object} map[string]string "No content available for this location"
// @Failure 400 {object} map[string]string "Invalid query parameters"
// @Failure 429 {string} string "Too many requests, please try again later"
// @Failure 500 {object} map[string]string "Failed to create a PDF"
// @Router /api/v1/markers/save-offline [get]
func (h *MarkerStatusHandler) HandleSaveOfflineMap2(c *fiber.Ctx) error {
	lat, lng, err := GetLatLong(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	pdf, _, err := h.MarkerFacadeService.SaveOfflineMap2(lat, lng)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create a PDF"})
	}
	if pdf == "" {
		return c.Status(fiber.StatusNoContent).JSON(fiber.Map{"error": "no content for this location"})
	}

	// Use Fiber's SendFile method
	// err = c.SendFile(pdf, true) // 'true' to enable compression
	// if err != nil {
	// 	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send file"})
	// }
	// return nil
	return c.Download(pdf)
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
func (h *MarkerStatusHandler) HandleVerifyMarker(c *fiber.Ctx) error {
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

func (h *MarkerStatusHandler) HandleTestDynamic(c *fiber.Ctx) error {
	lat, lng, err := GetLatLong(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	sParam := c.Query("scale")
	wParam := c.Query("width")
	hParam := c.Query("height")
	scale, err := strconv.ParseFloat(sParam, 64)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid latitude"})
	}
	width, err := strconv.ParseInt(wParam, 0, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid longitude"})
	}
	height, err := strconv.ParseInt(hParam, 0, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid longitude"})
	}

	h.MarkerFacadeService.TestDynamic(lat, lng, scale, width, height)
	return c.SendString("Dynamic API test")
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
func (h *MarkerStatusHandler) HandleGetRoadViewPicDate(c *fiber.Ctx) error {
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
