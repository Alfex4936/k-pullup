package main

import (
	"chulbong-kr/database"
	"chulbong-kr/handlers"
	"chulbong-kr/middlewares"
	"chulbong-kr/services"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	// Initialize database connection
	if err := database.Connect(); err != nil {
		panic(err)
	}

	// Initialize Fiber app
	app := fiber.New(fiber.Config{
		// Optional: Set up Fiber's config for production
		Prefork:       true, // Enable prefork mode for high-concurrency
		CaseSensitive: true,
		StrictRouting: true,
	})

	// Enable CORS for all routes
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Setup routes

	// Group routes under /markers
	markerGroup := app.Group("/markers")
	{
		markerGroup.Post("/", handlers.CreateMarker)
		markerGroup.Get("/:id", handlers.GetMarker)
		markerGroup.Put("/:id", handlers.UpdateMarker)
	}

	userGroup := app.Group("/users")
	{
		userGroup.Post("/signup", handlers.SignUpHandler)
		userGroup.Get("/login", handlers.LoginHandler)
	}

	apiGroup := app.Group("/api")
	{
		apiGroup.Use(middlewares.AuthMiddleware)
		apiGroup.Get("/", func(c *fiber.Ctx) error { return c.JSON("yo") })

	}

	app.Get("/example-get", handlers.GetExample)
	app.Put("/example-put", handlers.PutExample)
	app.Delete("/example-delete", handlers.DeleteExample)
	app.Post("/example-post", handlers.PostExample)

	app.Get("/example/:string/:id", handlers.DynamicRouteExample)
	app.Get("/example-optional/:param?", handlers.QueryParamsExample)

	services.CronCleanUpToken()

	// Start the Fiber app
	if err := app.Listen(":9452"); err != nil {
		panic(err)
	}
}
