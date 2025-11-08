package servicefx

import (
	"github.com/Alfex4936/chulbong-kr/app"
	"github.com/Alfex4936/chulbong-kr/middleware"
	"github.com/Alfex4936/chulbong-kr/providers"
	"go.uber.org/fx"
)

// FxProviderModule contains all provider functions for infrastructure and resources
var FxProviderModule = fx.Module("providers",
	fx.Provide(
		// Infrastructure
		providers.NewHTTPClient,
		providers.NewLogger,
		providers.NewDatabase,
		providers.NewRedis,
		providers.NewGoCacheLocalStorage,

		// Search
		providers.NewBleveIndex,

		// Resources
		providers.NewWsConfig,
		providers.NewStationData,
		providers.NewTimeZoneFinder,

		// Metrics
		providers.NewRegistry,
		providers.NewLoginCounter,

		// Middleware
		middleware.NewAuthMiddleware,
		middleware.NewLogMiddleware,

		// Application
		app.NewFiberApp,
	),
)

// FxInvokeModule contains all invocation functions for lifecycle management
// This is kept as a variable to allow easy extension if needed,
// but the actual invocations should be in main.go for clarity
var FxInvokeModule = fx.Options()
