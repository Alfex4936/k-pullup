package handler

import (
	"bytes"
	"context"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/facade"
	"github.com/Alfex4936/chulbong-kr/middleware"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"go.uber.org/zap"

	"github.com/chai2010/webp"
)

type AdminHandler struct {
	AdminFacade       *facade.AdminFacadeService
	UserFacadeService *facade.UserFacadeService
	UserService       *service.UserService

	TokenUtil *util.TokenUtil
	Logger    *zap.Logger
	ChatUtil  *util.ChatUtil
}

// NewAdminHandler creates a new AdminHandler with dependencies injected
func NewAdminHandler(
	admin *facade.AdminFacadeService,
	userfa *facade.UserFacadeService,
	user *service.UserService,
	tutil *util.TokenUtil,
	logger *zap.Logger,
	chatUtil *util.ChatUtil,
) *AdminHandler {
	return &AdminHandler{
		AdminFacade:       admin,
		UserFacadeService: userfa,
		UserService:       user,
		TokenUtil:         tutil,
		Logger:            logger,
		ChatUtil:          chatUtil,
	}
}

// RegisterAdminRoutes sets up the routes for admin handling within the application.
func RegisterAdminRoutes(api fiber.Router, handler *AdminHandler, authMiddleware *middleware.AuthMiddleware) {
	api.Post("/chat/ban/:markerID/:userID", authMiddleware.CheckAdmin, handler.HandleBanUser)

	api.Post("/admin/blur-encode", handler.HandleEncodeBlurImage)
	api.Get("/admin/blur-decode", handler.HandleDecodeBlurImage)

	api.Get("/notices", handler.HandleListNotices)
	api.Get("/admin/openapi", authMiddleware.CheckAdmin, handler.HandleHTMLReportAdmin)

	// Mimic Next.js image optimization URL:
	// e.g. /next/image?url=<encoded-image-url>&w=3840&q=75
	api.Get("/next/image", limiter.New(limiter.Config{
		KeyGenerator: func(c *fiber.Ctx) string {
			return "image-" + handler.ChatUtil.GetUserIP(c)
		},
		Max:               75,
		Expiration:        1 * time.Minute,
		LimiterMiddleware: limiter.SlidingWindow{},
		LimitReached: func(c *fiber.Ctx) error {
			c.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
			c.Status(429).SendString("Too many requests, please try again later.")
			return nil
		},
		SkipFailedRequests: false,
	}), handler.HandleNextImage)

	adminGroup := api.Group("/admin")
	{
		adminGroup.Use(authMiddleware.CheckAdmin)
		adminGroup.Get("/dead", handler.HandleListUnreferencedS3Objects)
		adminGroup.Get("/fetch", handler.HandleListUpdatedMarkers)
		adminGroup.Get("/unique-visitors/:date", handler.HandleListVisitors)
		adminGroup.Get("/s3-list", handler.HandleListS3)
		adminGroup.Get("/reports-ui", handler.HandleReportAdminPage)

		adminGroup.Post("/notices", handler.HandleCreateNotice)
		adminGroup.Delete("/notices/:noticeID", handler.HandleDeleteNotice)

		adminGroup.Delete("/photo", handler.HandleDeletePhoto)
	}
}

func (h *AdminHandler) HandleListUnreferencedS3Objects(c *fiber.Ctx) error {
	killSwitch := c.Query("kill", "n")

	dbURLs, err := h.AdminFacade.FetchAllPhotoURLsFromDB()
	if err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "fetching URLs from database:" + err.Error()})
	}

	s3Objects, err := h.AdminFacade.ListAllObjectsInS3()
	if err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "fetching keys from S3"})
	}

	var keys []string
	for _, obj := range s3Objects {
		if key, ok := obj["Key"].(string); ok {
			keys = append(keys, key)
		}
	}

	unreferenced := h.AdminFacade.FindUnreferencedS3Objects(dbURLs, keys)

	if killSwitch == "y" {
		for _, unreferencedURL := range unreferenced {
			_ = h.AdminFacade.DeleteDataFromS3(unreferencedURL)
		}
	}

	return c.JSON(unreferenced)
}

func (h *AdminHandler) HandleListS3(c *fiber.Ctx) error {
	s3Objects, err := h.AdminFacade.ListAllObjectsInS3()
	if err != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "fetching keys from S3"})
	}

	return c.JSON(s3Objects)
}

func (h *AdminHandler) HandleBanUser(c *fiber.Ctx) error {
	// Extract markerID and userID from the path parameters
	markerID := c.Params("markerID")
	userID := c.Params("userID")

	// assert duration is sent in the request body as JSON
	var requestBody struct {
		DurationInMinutes int `json:"duration"`
	}
	if err := c.BodyParser(&requestBody); err != nil {
		requestBody = struct {
			DurationInMinutes int `json:"duration"`
		}{
			DurationInMinutes: 5, // default 5 minutes banned
		}
		// return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		// 	"error": "Invalid request format",
		// })
	}

	if requestBody.DurationInMinutes < 1 {
		requestBody.DurationInMinutes = 5
	} else if requestBody.DurationInMinutes > 15 {
		requestBody.DurationInMinutes = 10 // max 10 minutes
	}

	// Convert duration to time.Duration
	duration := time.Duration(requestBody.DurationInMinutes) * time.Minute

	// Call the BanUser method on the manager instance
	err := h.AdminFacade.BanUser(markerID, userID, duration)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to ban user",
		})
	}

	// Return success response
	return c.JSON(fiber.Map{
		"message": "User successfully banned",
		"time":    duration,
	})
}

func (h *AdminHandler) HandleListUpdatedMarkers(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	postSwitch := c.Query("post", "n")
	currentDateString := c.Query("date", time.Now().Format("2006-01-02"))

	currentDate, _ := time.Parse("2006-1-2", currentDateString)

	markers, err := h.AdminFacade.FetchLatestMarkers(currentDate)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if postSwitch == "y" {
		for _, m := range markers {
			latitude, err := strconv.ParseFloat(string(m.Latitude), 64)
			if err != nil {
				h.Logger.Warn("Failed to parse latitude", zap.String("latitude", string(m.Latitude)), zap.Error(err))
				continue
			}

			longitude, err := strconv.ParseFloat(string(m.Longitude), 64)
			if err != nil {
				h.Logger.Warn("Failed to parse longitude", zap.String("longitude", string(m.Longitude)), zap.Error(err))
				continue
			}

			if fErr := h.AdminFacade.CheckMarkerValidity(latitude, longitude, ""); fErr != nil {
				h.Logger.Info("Skipping marker", zap.String("reason", fErr.Message))
				continue
			}

			userID := c.Locals("userID").(int)

			latitudeForm := []string{string(m.Latitude)}
			longitudeForm := []string{string(m.Longitude)}

			// Create the form with the initial value map containing the latitude and longitude.
			form := &multipart.Form{
				Value: map[string][]string{
					"latitude":  latitudeForm,
					"longitude": longitudeForm,
				},
				File: nil, // No file uploads are being handled
			}

			marker, err := h.AdminFacade.CreateMarkerWithPhotos(ctx, &dto.MarkerRequest{
				Latitude:    latitude,
				Longitude:   longitude,
				Description: "",
			}, userID, form)
			if err != nil {
				h.Logger.Error("Error creating marker", zap.Error(err))
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Error creating marker"})
			}

			newMarkerID := marker.MarkerID

			if newMarkerID == 0 {
				h.Logger.Error("Error creating marker with ID 0", zap.Error(err))
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Error creating marker"})
			}

			// Now, prepare the request for setting facilities
			if m.ChulbongCount < 1 || m.PyeongCount < 1 {
				continue
				// return c.Status(fiber.StatusOK).JSON(fiber.Map{"error": "No facilities to add"})
			}

			if err := h.AdminFacade.SetMarkerFacilities(newMarkerID, []dto.FacilityQuantity{
				{FacilityID: 1, Quantity: m.ChulbongCount},
				{FacilityID: 2, Quantity: m.PyeongCount},
			}); err != nil {
				continue
				// return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to set facilities for marker"})
			}

		}
	}

	h.AdminFacade.ResetMarkerCache()

	return c.JSON(markers)
}

// date := time.Now().Format("2006-01-02")
func (h *AdminHandler) HandleListVisitors(c *fiber.Ctx) error {
	date := c.Params("date")
	count, err := h.AdminFacade.GetUniqueVisitorsDB(date)
	if err != nil {
		return c.Status(500).SendString("Internal Server Error")
	}

	return c.JSON(fiber.Map{"date": date, "unique_visitors": count})
}

func (h *AdminHandler) HandleReportAdminPage(c *fiber.Ctx) error {
	// Get the userID from context
	userID, ok := c.Locals("userID").(int)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Fetch the reports data
	reportsData, err := h.UserFacadeService.GetAllReportsForMyMarkersByUser(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}

	// Initialize slices for each status
	type ReportItem struct {
		MarkerID int
		Address  string
		Report   dto.ReportWithPhotos
	}

	var pendingReports, deniedReports, approvedReports []ReportItem

	// Iterate over markers and their reports
	for _, marker := range reportsData.Markers {
		markerID := marker.MarkerID
		address := marker.Address
		for _, report := range marker.Reports {
			item := ReportItem{
				MarkerID: markerID,
				Address:  address,
				Report:   report,
			}
			switch report.Status {
			case "PENDING":
				pendingReports = append(pendingReports, item)
			case "DENIED":
				deniedReports = append(deniedReports, item)
			case "APPROVED":
				approvedReports = append(approvedReports, item)
			}
		}
	}

	// Sort the reports by CreatedAt in descending order within each status group
	sort.SliceStable(pendingReports, func(i, j int) bool {
		return pendingReports[i].Report.CreatedAt.After(pendingReports[j].Report.CreatedAt)
	})
	sort.SliceStable(deniedReports, func(i, j int) bool {
		return deniedReports[i].Report.CreatedAt.After(deniedReports[j].Report.CreatedAt)
	})
	sort.SliceStable(approvedReports, func(i, j int) bool {
		return approvedReports[i].Report.CreatedAt.After(approvedReports[j].Report.CreatedAt)
	})

	// Render the template with the grouped reports
	return c.Render("report_admin", fiber.Map{
		"pending_reports":  pendingReports,
		"denied_reports":   deniedReports,
		"approved_reports": approvedReports,
	})
}

func (h *AdminHandler) HandleEncodeBlurImage(c *fiber.Ctx) error {
	// Parse the uploaded file
	file, err := c.FormFile("image")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid file"})
	}

	// Extract file extension
	ext := filepath.Ext(file.Filename) // e.g., ".jpg" or ".png"

	// Open the file
	src, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to open file"})
	}
	defer src.Close()

	// Get EXIF orientation
	orientation := util.GetOrientationByReader(src)

	// Reset the file pointer after reading orientation
	src.Seek(0, 0)

	// Decode the image
	img, _, err := image.Decode(src)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid image format"})
	}

	blurHash := util.EncodeBlurHashImageWithMeta(img, 6, 5, ext, orientation)
	return c.JSON(fiber.Map{"hash": blurHash, "extension": ext, "orientation": orientation})
}

func (h *AdminHandler) HandleDecodeBlurImage(c *fiber.Ctx) error {
	hash := c.Query("hash", "")
	if hash == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Hash is required"})
	}

	if !util.IsValidExtendedBlurhash(hash) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid blurhash"})
	}

	pixels, width, height, orientation, ext, err := util.DecodeBlurHashWithMeta(hash, 1.0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to decode blurhash"})
	}

	// Convert decoded pixels into an image (returns *image.RGBA but we store in image.Image)
	var decodedImg image.Image = util.PixelsToImage(pixels, width, height)

	// Fix orientation (FixOrientation returns image.Image)
	decodedImg = util.FixOrientation(decodedImg, orientation)

	// Map of file extensions to MIME types
	mimeTypes := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
	}

	mimeType, supported := mimeTypes[ext]
	if !supported {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unsupported image format"})
	}

	// Encode the image into the requested format
	var buf bytes.Buffer
	switch ext {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(&buf, decodedImg, nil)
	case ".png":
		err = png.Encode(&buf, decodedImg)
	case ".gif":
		err = gif.Encode(&buf, decodedImg, nil)
	case ".webp":
		err = webp.Encode(&buf, decodedImg, nil)
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to encode image"})
	}

	c.Set(fiber.HeaderContentType, mimeType)
	return c.Send(buf.Bytes())
}

func (h *AdminHandler) HandleListNotices(c *fiber.Ctx) error {
	notices, err := h.AdminFacade.ListNotices()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			fiber.Map{"error": err.Error()})
	}
	return c.JSON(notices)
}

func (h *AdminHandler) HandleCreateNotice(c *fiber.Ctx) error {
	var dto dto.NoticePostDTO
	if err := c.BodyParser(&dto); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "Invalid input"})
	}

	authorID := c.Locals("userID").(int)

	noticeID, err := h.AdminFacade.CreateNotice(dto.Title, dto.Content, authorID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"noticeID": noticeID,
		"message":  "Notice created successfully",
	})
}

func (h *AdminHandler) HandleDeleteNotice(c *fiber.Ctx) error {
	noticeIDParam := c.Params("noticeID")
	noticeID, err := strconv.Atoi(noticeIDParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(fiber.Map{"error": "invalid notice ID"})
	}

	if err := h.AdminFacade.DeleteNotice(noticeID); err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Notice deleted successfully"})
}

// HandleDeletePhoto deletes a photo for a given marker by its index (sorted by UploadedAt).
// It expects two query parameters: markerId and photoIdx.
func (h *AdminHandler) HandleDeletePhoto(c *fiber.Ctx) error {
	markerIDStr := c.Query("markerId")
	photoIdxStr := c.Query("photoIdx")
	if markerIDStr == "" || photoIdxStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "both markerId and photoIdx query parameters are required",
		})
	}

	markerID, err := strconv.Atoi(markerIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid markerId"})
	}
	photoIdx, err := strconv.Atoi(photoIdxStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid photoIdx"})
	}

	// Create a context with timeout (adjust the duration as needed)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Call the business logic to delete the photo
	if err := h.AdminFacade.DeleteMarkerPhoto(ctx, markerID, photoIdx); err != nil {
		h.Logger.Error("failed to delete photo", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to delete photo",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "photo deleted successfully",
	})
}

// HandleNextImage mimics Next.js’s image optimization with caching.
func (h *AdminHandler) HandleNextImage(c *fiber.Ctx) error {
	srcURL := c.Query("url", "")
	width, err := strconv.Atoi(c.Query("w", "0"))
	if err != nil {
		width = 0
	}
	quality, err := strconv.Atoi(c.Query("q", "75"))
	if err != nil {
		quality = 75
	}

	// Set aggressive timeout for low-resource server
	c.Context().SetUserValue("timeout", 10*time.Second)

	// Call the service to optimize the image
	resultBytes, contentType, err := h.AdminFacade.OptimizeImage(srcURL, width, quality, c.Get("Accept"))
	if err != nil {
		// Return 503 for resource exhaustion instead of 500
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "busy") {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Service temporarily unavailable"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Add cache headers for CDN/browser caching
	c.Set(fiber.HeaderCacheControl, "public, max-age=86400") // 24 hours
	c.Set(fiber.HeaderContentType, contentType)

	return c.Send(resultBytes)
}

func (h *AdminHandler) HandleHTMLReportAdmin(c *fiber.Ctx) error {
	return c.Render("swagger", fiber.Map{})
}
