package util

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestParsePaginationParams(t *testing.T) {
	// Create a new Fiber app for testing
	app := fiber.New()

	// Route with default configuration
	app.Get("/paginate", func(c *fiber.Ctx) error {
		params, err := ParsePaginationParams(c, nil) // nil config uses defaults
		if err != nil {
			return err
		}
		return c.JSON(params)
	})

	// Route with custom configuration
	app.Get("/custom", func(c *fiber.Ctx) error {
		config := &PaginationConfig{
			DefaultPage:       2,
			DefaultPageSize:   20,
			PageParamName:     "customPage",
			PageSizeParamName: "customPageSize",
		}
		params, err := ParsePaginationParams(c, config)
		if err != nil {
			return err
		}
		return c.JSON(params)
	})

	// Helper function to make requests and parse response
	getParams := func(url string) (PaginationParams, int) {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		resp, _ := app.Test(req)
		var params PaginationParams
		json.NewDecoder(resp.Body).Decode(&params)
		return params, resp.StatusCode
	}

	// Test cases
	t.Run("Default Configuration - No Query Params", func(t *testing.T) {
		params, status := getParams("/paginate")
		assert.Equal(t, 200, status)
		assert.Equal(t, 1, params.Page)
		assert.Equal(t, 10, params.PageSize)
		assert.Equal(t, 0, params.Offset)
	})

	t.Run("Default Configuration - Missing Page", func(t *testing.T) {
		params, status := getParams("/paginate?pageSize=15")
		assert.Equal(t, 200, status)
		assert.Equal(t, 1, params.Page)
		assert.Equal(t, 15, params.PageSize)
		assert.Equal(t, 0, params.Offset)
	})

	t.Run("Default Configuration - Missing PageSize", func(t *testing.T) {
		params, status := getParams("/paginate?page=2")
		assert.Equal(t, 200, status)
		assert.Equal(t, 2, params.Page)
		assert.Equal(t, 10, params.PageSize)
		assert.Equal(t, 10, params.Offset)
	})

	t.Run("Custom Configuration", func(t *testing.T) {
		params, status := getParams("/custom?customPage=3&customPageSize=25")
		assert.Equal(t, 200, status)
		assert.Equal(t, 3, params.Page)
		assert.Equal(t, 25, params.PageSize)
		assert.Equal(t, 50, params.Offset)
	})

	t.Run("Valid Inputs", func(t *testing.T) {
		params, status := getParams("/paginate?page=5&pageSize=20")
		assert.Equal(t, 200, status)
		assert.Equal(t, 5, params.Page)
		assert.Equal(t, 20, params.PageSize)
		assert.Equal(t, 80, params.Offset)
	})

	t.Run("Invalid Inputs - Non-Integer", func(t *testing.T) {
		params, status := getParams("/paginate?page=abc&pageSize=def")
		assert.Equal(t, 200, status)
		assert.Equal(t, 1, params.Page)
		assert.Equal(t, 10, params.PageSize)
		assert.Equal(t, 0, params.Offset)
	})

	t.Run("Invalid Inputs - Negative Integers", func(t *testing.T) {
		params, status := getParams("/paginate?page=-1&pageSize=-10")
		assert.Equal(t, 200, status)
		assert.Equal(t, 1, params.Page)
		assert.Equal(t, 10, params.PageSize)
		assert.Equal(t, 0, params.Offset)
	})

	t.Run("Invalid Inputs - Zero", func(t *testing.T) {
		params, status := getParams("/paginate?page=0&pageSize=0")
		assert.Equal(t, 200, status)
		assert.Equal(t, 1, params.Page)
		assert.Equal(t, 10, params.PageSize)
		assert.Equal(t, 0, params.Offset)
	})

	t.Run("Edge Cases - Empty Strings", func(t *testing.T) {
		params, status := getParams("/paginate?page=&pageSize=")
		assert.Equal(t, 200, status)
		assert.Equal(t, 1, params.Page)
		assert.Equal(t, 10, params.PageSize)
		assert.Equal(t, 0, params.Offset)
	})

	t.Run("Edge Cases - Overflow", func(t *testing.T) {
		params, status := getParams("/paginate?page=999999999999999999&pageSize=999999999999999999")
		assert.Equal(t, 200, status)
		assert.Equal(t, 1000, params.Page)    // default MaxPage
		assert.Equal(t, 100, params.PageSize) // default MaxPageSize
		assert.Equal(t, 99900, params.Offset) // (page-1)*pageSize)
	})

	t.Run("Edge Cases - Multiple Query Params", func(t *testing.T) {
		params, status := getParams("/paginate?page=2&pageSize=15&extra=param")
		assert.Equal(t, 200, status)
		assert.Equal(t, 2, params.Page)
		assert.Equal(t, 15, params.PageSize)
		assert.Equal(t, 15, params.Offset)
	})
}
