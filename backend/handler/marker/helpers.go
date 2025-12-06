package marker

import (
	"errors"
	"mime/multipart"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

func GetLatLngFromForm(form *multipart.Form) (float64, float64, error) {
	latStr, latOk := form.Value["latitude"]
	longStr, longOk := form.Value["longitude"]
	if !latOk || !longOk || len(latStr[0]) == 0 || len(longStr[0]) == 0 {
		return 0, 0, errors.New("latitude and longitude are required")
	}

	latitude, err := strconv.ParseFloat(latStr[0], 64)
	if err != nil {
		return 0, 0, errors.New("invalid latitude")
	}

	longitude, err := strconv.ParseFloat(longStr[0], 64)
	if err != nil {
		return 0, 0, errors.New("invalid longitude")
	}

	return latitude, longitude, nil
}

func GetLatLong(c *fiber.Ctx) (float64, float64, error) {
	latParam := c.Query("latitude")
	longParam := c.Query("longitude")

	lat, err := strconv.ParseFloat(latParam, 64)
	if err != nil {
		return 0, 0, errors.New("invalid latitude")
	}

	long, err := strconv.ParseFloat(longParam, 64)
	if err != nil {
		return 0, 0, errors.New("invalid longitude")
	}

	if lat < 32 || lat > 39 {
		return 0, 0, errors.New("invalid latitude (Must be between 32 and 39)")
	}

	if long < 123 || long > 133 {
		return 0, 0, errors.New("invalid longitude (Must be between 123 and 133)")
	}

	return lat, long, nil
}

func GetDescriptionFromForm(form *multipart.Form) string {
	if descValues, exists := form.Value["description"]; exists && len(descValues[0]) > 0 {
		return descValues[0]
	}
	return ""
}

func GetMarkerIDFromForm(form *multipart.Form) string {
	if descValues, exists := form.Value["markerId"]; exists && len(descValues[0]) > 0 {
		return descValues[0]
	}
	return ""
}

// ParseMarkerIDParam extracts a path param and returns a parsed marker ID with a 400 error on failure.
func ParseMarkerIDParam(c *fiber.Ctx, param string) (int, error) {
	markerID, err := strconv.Atoi(c.Params(param))
	if err != nil {
		return 0, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid marker ID"})
	}
	return markerID, nil
}
