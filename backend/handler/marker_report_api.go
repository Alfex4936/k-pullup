package handler

import (
	"errors"
	"fmt"
	"mime/multipart"
	"strconv"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/middleware"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	"go.uber.org/zap"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// RegisterReportRoutes sets up the routes for report handling within the application.
func RegisterReportRoutes(api fiber.Router, handler *MarkerHandler, authMiddleware *middleware.AuthMiddleware) {
	reportGroup := api.Group("/reports")
	reportGroup.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			handler.logger.Error("Panic recovered in report API",
				zap.Any("error", e),
				zap.String("url", c.Path()),
				zap.String("method", c.Method()),
			)
		},
	}))

	{
		reportGroup.Get("/all", handler.HandleGetAllReports)
		reportGroup.Get("/marker/:markerID", handler.HandleGetMarkerReports)

		reportGroup.Post("", authMiddleware.VerifySoft, handler.HandleCreateReport)
		reportGroup.Post("/approve/:reportID", authMiddleware.Verify, handler.HandleApproveReport)
		reportGroup.Post("/deny/:reportID", authMiddleware.Verify, handler.HandleDenyReport)

		reportGroup.Delete("", authMiddleware.Verify, handler.HandleDeleteReport)

	}
}

// HandleGetAllReports retrieves all reports for all markers, grouped by Marker ID.
//
// @Summary Get all marker reports
// @Description Fetches all reports related to markers and groups them by Marker ID.
// @ID get-all-marker-reports
// @Tags markers-report
// @Accept json
// @Produce json
// @Security
// @Success 200 {object} dto.ReportsResponse "List of all marker reports grouped by Marker ID"
// @Failure 404 {object} map[string]string "No reports found"
// @Failure 500 {object} map[string]string "Failed to retrieve reports"
// @Router /api/v1/markers/reports/all [get]
func (h *MarkerHandler) HandleGetAllReports(c *fiber.Ctx) error {
	reports, err := h.MarkerFacadeService.GetAllReports()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get reports"})
	}

	if len(reports) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "No reports found"})
	}

	// Group reports by MarkerID
	groupedReports := make(map[int]dto.MarkerReports)
	for _, report := range reports {
		groupedReports[report.MarkerID] = dto.MarkerReports{
			Reports: append(groupedReports[report.MarkerID].Reports, report),
		}
	}

	// Create response structure
	response := dto.ReportsResponse{
		TotalReports: len(reports),
		Markers:      groupedReports,
	}

	return c.JSON(response)
}

// HandleGetMarkerReports retrieves all reports for a specific marker.
//
// @Summary Get reports for a marker
// @Description Fetches all reports related to a specific marker by Marker ID.
// @ID get-marker-reports
// @Tags markers-report
// @Accept json
// @Produce json
// @Security
// @Param markerID path int true "Marker ID"
// @Success 200 {array} dto.MarkerReportResponse "List of reports for the marker"
// @Failure 400 {object} map[string]string "Invalid Marker ID"
// @Failure 500 {object} map[string]string "Failed to retrieve reports"
// @Router /api/v1/markers/reports/marker/{markerID} [get]
func (h *MarkerHandler) HandleGetMarkerReports(c *fiber.Ctx) error {
	markerID, err := strconv.Atoi(c.Params("markerID"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid Marker ID"})
	}

	reports, err := h.MarkerFacadeService.GetAllReportsBy(markerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get reports " + err.Error()})
	}
	return c.JSON(reports)
}

// HandleCreateReport creates a new report for a marker.
//
// @Summary Create a marker report
// @Description Allows an authenticated user to report a marker, including optional location adjustments and a description.
// @ID create-marker-report
// @Tags markers-report
// @Accept multipart/form-data
// @Produce json
// @Param markerID formData int true "Marker ID"
// @Param latitude formData number true "Original latitude"
// @Param longitude formData number true "Original longitude"
// @Param newLatitude formData number false "Updated latitude (must be within 30 meters)"
// @Param newLongitude formData number false "Updated longitude (must be within 30 meters)"
// @Param description formData string true "Report description"
// @Param doesExist formData boolean false "Indicates if the marker exists (true/false)"
// @Param photos formData file true "At least one photo required"
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Report created successfully"
// @Failure 400 {object} map[string]string "Invalid parameters or inappropriate content"
// @Failure 403 {object} map[string]string "Operations only allowed within South Korea"
// @Failure 406 {object} map[string]string "New latitude/longitude too far from original location"
// @Failure 409 {object} map[string]string "Check if the marker exists or upload at least one photo"
// @Failure 500 {object} map[string]string "Failed to create report"
// @Router /api/v1/markers/reports [post]
func (h *MarkerHandler) HandleCreateReport(c *fiber.Ctx) error {
	// Parse the multipart form
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to parse form"})
	}

	// Check if latitude and longitude are provided
	// if user didn't change, frontend must send original point
	latitude, longitude, err := GetLatLngFromForm(form)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to parse latitude and longitude"})
	}

	newLatitude, newLongitude, err := getNewLatLngFromForm(form)
	const maxDistance = 30.0 // maximum distance in meters
	const errorMargin = 1.0  // margin error in meters

	if err != nil {
		newLatitude = latitude
		newLongitude = longitude // use original location if new location is not provided
	} else {
		// check if the updated location is in 30m distance from the original location
		distance := util.CalculateDistanceApproximately(latitude, longitude, newLatitude, newLongitude)
		if distance > maxDistance+errorMargin {
			return c.Status(fiber.StatusNotAcceptable).JSON(fiber.Map{"error": "new latitude and longitude are too far, try to add a new marker."})
		}
	}

	// Location Must Be Inside South Korea
	if !h.MarkerFacadeService.IsInSouthKoreaPrecisely(latitude, longitude) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Operations only allowed within South Korea."})
	}

	description := GetDescriptionFromForm(form)
	if containsBadWord, _ := h.MarkerFacadeService.CheckBadWord(description); containsBadWord {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Comment contains inappropriate content."})
	}

	markerIDstr := GetMarkerIDFromForm(form)
	if markerIDstr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Inappropriate markerId."})
	}
	markerID, err := strconv.Atoi(markerIDstr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid marker ID"})
	}

	var doesExist bool
	doesStr, doesOk := form.Value["doesExist"]
	if !doesOk || len(doesStr[0]) == 0 {
		doesExist = true
	} else {
		// Convert 'doesExist' field to boolean
		doesExist, err = strconv.ParseBool(doesStr[0])
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid value for doesExist field")
		}
	}

	userID, _ := c.Locals("userID").(int) // userID will be 0 if not logged in

	err = h.MarkerFacadeService.CreateReport(&dto.MarkerReportRequest{
		MarkerID:     markerID,
		UserID:       userID,
		Latitude:     latitude,
		Longitude:    longitude,
		NewLatitude:  newLatitude,
		NewLongitude: newLongitude,
		Description:  description,
		DoesExist:    doesExist,
	}, form)
	if err != nil {
		var status int
		var response dto.SimpleErrorResponse

		switch {
		case errors.Is(err, service.ErrNoPhotos):
			status = fiber.StatusConflict
			response = dto.SimpleErrorResponse{Error: "upload at least one photo"}

		case errors.Is(err, service.ErrFileUpload):
			status = fiber.StatusConflict
			response = dto.SimpleErrorResponse{Error: "an error occurred during file upload"}

		case errors.Is(err, service.ErrMarkerDoesNotExist):
			status = fiber.StatusConflict
			response = dto.SimpleErrorResponse{Error: "check if the marker exists"}

		default:
			status = fiber.StatusInternalServerError
			response = dto.SimpleErrorResponse{Error: "failed to create report"}
		}

		return c.Status(status).JSON(response)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "report created successfully"})
}

// HandleApproveReport approves a marker report.
//
// @Summary Approve a marker report
// @Description Allows an authenticated user to approve a report related to a marker.
// @ID approve-marker-report
// @Tags markers-report
// @Accept json
// @Produce json
// @Param reportID path int true "Report ID"
// @Security ApiKeyAuth
// @Success 200 "Report approved successfully"
// @Failure 400 {object} map[string]string "Invalid report ID"
// @Failure 500 {object} map[string]string "Unable to approve report"
// @Router /api/v1/markers/reports/approve/{reportID} [post]
func (h *MarkerHandler) HandleApproveReport(c *fiber.Ctx) error {
	reportID, err := strconv.Atoi(c.Params("reportID"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid report ID"})
	}

	userID, _ := c.Locals("userID").(int)

	if err := h.MarkerFacadeService.ApproveReport(reportID, userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Unable to approve report"})
	}

	return c.SendStatus(fiber.StatusOK)
}

// HandleDenyReport denies a marker report.
//
// @Summary Deny a marker report
// @Description Allows an authenticated user to deny a report related to a marker.
// @ID deny-marker-report
// @Tags markers-report
// @Accept json
// @Produce json
// @Param reportID path int true "Report ID"
// @Security ApiKeyAuth
// @Success 200 "Report denied successfully"
// @Failure 400 {object} map[string]string "Invalid report ID"
// @Failure 500 {object} map[string]string "Unable to deny report"
// @Router /api/v1/markers/reports/deny/{reportID} [post]
func (h *MarkerHandler) HandleDenyReport(c *fiber.Ctx) error {
	reportID, err := strconv.Atoi(c.Params("reportID"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid report ID"})
	}
	userID, _ := c.Locals("userID").(int)

	if err := h.MarkerFacadeService.DenyReport(reportID, userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Unable to deny report"})
	}

	go h.MarkerFacadeService.SetMarkerCache(nil)
	go h.MarkerFacadeService.ResetAllRedisCache(fmt.Sprintf("userMarkers:%d:page:*", userID))

	return c.SendStatus(fiber.StatusOK)
}

// HandleDeleteReport deletes a marker report.
//
// @Summary Delete a marker report
// @Description Allows an authenticated user to delete a report related to a marker.
// @ID delete-marker-report
// @Tags markers-report
// @Accept json
// @Produce json
// @Param reportID query int true "Report ID"
// @Param markerID query int true "Marker ID"
// @Security ApiKeyAuth
// @Success 200 "Report deleted successfully"
// @Failure 400 {object} map[string]string "Invalid report ID or marker ID"
// @Failure 500 {object} map[string]string "Unable to remove report"
// @Router /api/v1/markers/reports [delete]
func (h *MarkerHandler) HandleDeleteReport(c *fiber.Ctx) error {
	reportID, err := strconv.Atoi(c.Query("reportID"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid report ID"})
	}
	markerID, err := strconv.Atoi(c.Query("markerID"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid report ID"})
	}
	userID, _ := c.Locals("userID").(int)

	if err := h.MarkerFacadeService.DeleteReport(reportID, userID, markerID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Unable to remove report"})
	}
	return c.SendStatus(fiber.StatusOK)
}

func getNewLatLngFromForm(form *multipart.Form) (float64, float64, error) {
	latStr, latOk := form.Value["newLatitude"]
	longStr, longOk := form.Value["newLongitude"]
	if !latOk || !longOk || len(latStr[0]) == 0 || len(longStr[0]) == 0 {
		return 0, 0, errors.New("latitude and longitude are empty")
	}

	latitude, err := strconv.ParseFloat(latStr[0], 64)
	if err != nil {
		return -1, -1, errors.New("invalid latitude")
	}

	longitude, err := strconv.ParseFloat(longStr[0], 64)
	if err != nil {
		return -1, -1, errors.New("invalid longitude")
	}

	return latitude, longitude, nil
}
