package main

import (
	"os"

	"github.com/Alfex4936/chulbong-kr/app"
	configfx "github.com/Alfex4936/chulbong-kr/configfx"
	servicefx "github.com/Alfex4936/chulbong-kr/di"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

// @title k-pullup API
// @description This is the API documentation for the k-pullup service.
// @version 1.0
// @license MIT
// @license.url https://raw.githubusercontent.com/Alfex4936/chulbong-kr/refs/heads/main/LICENSE
// @contact.name API Support
// @contact.email chulbong.kr@gmail.com
// @contact.url https://github.com/Alfex4936
func main() {
	// Load configuration
	viper.AutomaticEnv() // Automatically read from environment variables
	if viper.GetString("DEPLOYMENT") != "production" {
		viper.SetConfigFile(".env")
		if err := viper.ReadInConfig(); err != nil {
			// Ignore error, env vars might be sufficient
		}
	}

	// Load environment variables from a .env file if not in production
	if os.Getenv("DEPLOYMENT") != "production" {
		godotenv.Overload()
	}

	// Create an Fx application with provided dependencies and lifecycle hooks
	fx.New(
		// Configuration modules
		configfx.FxConfigModule,

		// Provider modules (infrastructure, resources, app)
		servicefx.FxProviderModule,

		// Domain modules (services, handlers, facades)
		servicefx.FxMarkerModule,
		servicefx.FxExternalModle,
		servicefx.FxChatModule,
		servicefx.FxUtilModule,
		servicefx.FxUserModule,
		servicefx.FxAPIModule,
		servicefx.FxFacadeModule,

		// Lifecycle invocations
		fx.Invoke(
			app.RegisterHooks,
			util.RegisterBadWordUtilLifecycle,
			service.RegisterSchedulerLifecycle,
			util.RegisterPdfInitLifecycle,
			service.RegisterMarkerLifecycle,
			service.RegisterMarkerLocationLifecycle,
			service.RegisterAuthLifecycle,
			service.RegisteBleveLifecycle,
			service.RegisterTokenServiceLifecycle,
		),
	).Run()
}
