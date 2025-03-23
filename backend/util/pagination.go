package util

import (
	"math"

	"github.com/gofiber/fiber/v2"
)

type PaginationParams struct {
	Page     int
	PageSize int
	Offset   int
}

type PaginationConfig struct {
	DefaultPage       int
	DefaultPageSize   int
	PageParamName     string
	PageSizeParamName string
	MaxPageSize       int // Add max page size limit
	MaxPage           int // Add max page number limit
	MinPage           int // Add min page number limit (optional, default 1)
}

var DefaultPaginationConfig = PaginationConfig{
	DefaultPage:       1,
	DefaultPageSize:   10,
	PageParamName:     "page",
	PageSizeParamName: "pageSize",
	MaxPageSize:       100,  // Example default limit
	MaxPage:           1000, // Example default limit
	MinPage:           1,
}

// parsePositiveInt parses a []byte as a positive integer, returning def on invalid input.
func parsePositiveInt(b []byte, def int) int {
	if len(b) == 0 || b[0] < '1' || b[0] > '9' { // More direct check for empty or starts with '0'
		return def
	}
	var n int
	for _, c := range b {
		if c < '0' || c > '9' {
			return def
		}
		d := int(c - '0')
		if n > (math.MaxInt-d)/10 { // Using int and math.MaxInt
			return def
		}
		n = n*10 + d
	}
	return n
}

func ParsePaginationParams(c *fiber.Ctx, config *PaginationConfig) (PaginationParams, error) {
	if config == nil {
		config = &DefaultPaginationConfig
	}

	defaultPage := config.DefaultPage
	defaultPageSize := config.DefaultPageSize

	args := c.Context().QueryArgs()

	// Parse page and pageSize directly from []byte using your fast parse function
	page := parsePositiveInt(args.Peek(config.PageParamName), defaultPage)
	pageSize := parsePositiveInt(args.Peek(config.PageSizeParamName), defaultPageSize)

	// Enforce the minimum page limit
	if page < config.MinPage {
		page = config.MinPage
	}

	// Enforce the maximum page limit, if set (non-zero)
	if config.MaxPage > 0 && page > config.MaxPage {
		page = config.MaxPage
	}

	// Enforce the maximum page size limit, if set (non-zero)
	if config.MaxPageSize > 0 && pageSize > config.MaxPageSize {
		pageSize = config.MaxPageSize
	}

	// Check for potential overflow when computing offset
	page64 := int64(page)
	pageSize64 := int64(pageSize)
	if page64 > 1 && pageSize64 > 0 && (page64-1) > math.MaxInt64/pageSize64 {
		// Overflow would occur: revert to defaults
		page = defaultPage
		pageSize = defaultPageSize
	}

	offset := (page - 1) * pageSize

	return PaginationParams{
		Page:     page,
		PageSize: pageSize,
		Offset:   offset,
	}, nil
}
