package providers

import (
	"go.uber.org/zap"
)

// NewLogger provides a new logger instance.
func NewLogger() (*zap.Logger, error) {
	return zap.NewProduction()
}
