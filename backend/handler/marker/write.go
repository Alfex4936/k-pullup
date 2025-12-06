package marker

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/util"
	sonic "github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

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
func (h *MarkerWriteHandler) HandleCreateMarkerWithPhotos(c *fiber.Ctx) error {
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
		} else if strings.Contains(err.Error(), "일일 마커 생성 한도") {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "일일 마커 생성 한도(10개)에 도달했습니다. 내일 다시 시도해주세요."})
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
func (h *MarkerWriteHandler) HandleUpdateMarker(c *fiber.Ctx) error {
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
func (h *MarkerWriteHandler) HandleDeleteMarker(c *fiber.Ctx) error {
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

	if err := h.CacheService.InvalidateFullMarkersCache(); err != nil {
		h.logger.Warn("Failed to invalidate marker cache after delete", zap.Error(err))
	}
	h.CacheService.InvalidateFacilities(markerID)
	h.CacheService.RemoveUserMarker(userID, markerID)

	return c.SendStatus(fiber.StatusOK)
}

// HandleDeleteMarkerPhoto deletes a photo belonging to a marker if the user is the owner or an admin.
//
// @Summary Delete marker photo
// @Description Allows the authenticated owner or an admin to delete a specific photo of a marker.
// @ID delete-marker-photo
// @Tags markers
// @Accept json
// @Produce json
// @Param markerID path int true "Marker ID"
// @Param photoID path int true "Photo ID"
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Photo deleted successfully"
// @Failure 400 {object} map[string]string "Invalid marker ID or photo ID"
// @Failure 403 {object} map[string]string "User is not authorized to delete this photo"
// @Failure 404 {object} map[string]string "Photo not found"
// @Failure 500 {object} map[string]string "Failed to delete photo"
// @Router /api/v1/markers/{markerID}/photos/{photoID} [delete]
func (h *MarkerWriteHandler) HandleDeleteMarkerPhoto(c *fiber.Ctx) error {
	userID := c.Locals("userID").(int)
	userRole := c.Locals("role").(string)

	markerID, err := ParseMarkerIDParam(c, "markerID")
	if err != nil {
		return err
	}
	photoID, err := strconv.Atoi(c.Params("photoID"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid photo ID"})
	}

	err = h.MarkerFacadeService.DeleteMarkerPhotoByID(userID, userRole, markerID, photoID)
	if err != nil {
		// Authorization failure still returns 403
		if strings.Contains(err.Error(), "not authorized") {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "User is not authorized to delete this photo"})
		}
		// Idempotent behavior: if photo already absent treat as success (200)
		if strings.Contains(err.Error(), "no photo found") {
			c.Set("X-Idempotent", "true")
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Photo already absent"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete photo"})
	}

	// Successful deletion
	c.Set("X-Idempotent", "true")
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Photo deleted successfully"})
}

// HandleUploadMarkerPhotoToS3 handles marker photo uploads.
func (h *MarkerWriteHandler) HandleUploadMarkerPhotoToS3(c *fiber.Ctx) error {
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
func (h *MarkerWriteHandler) HandleSetMarkerFacilities(c *fiber.Ctx) error {
	req := new(dto.FacilityRequest)
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse request"})
	}

	if err := h.MarkerFacadeService.SetMarkerFacilities(req.MarkerID, req.Facilities); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to set facilities for marker"})
	}

	return c.SendStatus(fiber.StatusOK)
}

// HandleUpdateMarkersAddresses handles the request to update all markers' addresses.
func (h *MarkerWriteHandler) HandleUpdateMarkersAddresses(c *fiber.Ctx) error {
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

// HandleRSS and HandleRefreshMarkerCache live in feed/write handlers to keep concerns narrow.
func (h *MarkerWriteHandler) HandleRefreshMarkerCache(c *fiber.Ctx) error {
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
