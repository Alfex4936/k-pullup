package marker

import (
	"github.com/Alfex4936/chulbong-kr/facade"
	"github.com/Alfex4936/chulbong-kr/service"
	"go.uber.org/zap"
)

// MarkerDeps bundles shared services for all marker handlers.
type MarkerDeps struct {
	MarkerFacadeService *facade.MarkerFacadeService
	CacheService        *service.MarkerCacheService
	logger              *zap.Logger
}

func NewMarkerDeps(facade *facade.MarkerFacadeService, cache *service.MarkerCacheService, logger *zap.Logger) *MarkerDeps {
	return &MarkerDeps{
		MarkerFacadeService: facade,
		CacheService:        cache,
		logger:              logger,
	}
}

// Feature handlers embed the shared deps.
type MarkerReadHandler struct{ *MarkerDeps }
type MarkerWriteHandler struct{ *MarkerDeps }
type MarkerInteractionHandler struct{ *MarkerDeps }
type MarkerStatusHandler struct{ *MarkerDeps }
type MarkerFeedHandler struct{ *MarkerDeps }
type MarkerStoryHandler struct{ *MarkerDeps }
type MarkerReportHandler struct{ *MarkerDeps }

func NewMarkerReadHandler(deps *MarkerDeps) *MarkerReadHandler {
	return &MarkerReadHandler{MarkerDeps: deps}
}
func NewMarkerWriteHandler(deps *MarkerDeps) *MarkerWriteHandler {
	return &MarkerWriteHandler{MarkerDeps: deps}
}
func NewMarkerInteractionHandler(deps *MarkerDeps) *MarkerInteractionHandler {
	return &MarkerInteractionHandler{MarkerDeps: deps}
}
func NewMarkerStatusHandler(deps *MarkerDeps) *MarkerStatusHandler {
	return &MarkerStatusHandler{MarkerDeps: deps}
}
func NewMarkerFeedHandler(deps *MarkerDeps) *MarkerFeedHandler {
	return &MarkerFeedHandler{MarkerDeps: deps}
}
func NewMarkerStoryHandler(deps *MarkerDeps) *MarkerStoryHandler {
	return &MarkerStoryHandler{MarkerDeps: deps}
}
func NewMarkerReportHandler(deps *MarkerDeps) *MarkerReportHandler {
	return &MarkerReportHandler{MarkerDeps: deps}
}

// MarkerHandler aggregates feature handlers so routing stays simple.
type MarkerHandler struct {
	deps   *MarkerDeps
	logger *zap.Logger

	*MarkerReadHandler
	*MarkerWriteHandler
	*MarkerInteractionHandler
	*MarkerStatusHandler
	*MarkerFeedHandler
	*MarkerStoryHandler
	*MarkerReportHandler
}

func NewMarkerHandler(
	read *MarkerReadHandler,
	write *MarkerWriteHandler,
	interactions *MarkerInteractionHandler,
	status *MarkerStatusHandler,
	feed *MarkerFeedHandler,
	story *MarkerStoryHandler,
	report *MarkerReportHandler,
	deps *MarkerDeps,
) *MarkerHandler {
	return &MarkerHandler{
		deps:   deps,
		logger: deps.logger,

		MarkerReadHandler:        read,
		MarkerWriteHandler:       write,
		MarkerInteractionHandler: interactions,
		MarkerStatusHandler:      status,
		MarkerFeedHandler:        feed,
		MarkerStoryHandler:       story,
		MarkerReportHandler:      report,
	}
}
