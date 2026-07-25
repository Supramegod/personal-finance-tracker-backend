package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/swagger"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"personal-finance-tracker/docs"
	"personal-finance-tracker/internal/config"
	"personal-finance-tracker/internal/repository"
	"personal-finance-tracker/internal/router"
	"personal-finance-tracker/internal/service"
)

// @title               Personal Finance Tracker API
// @version             1.0
// @description         Aplikasi pencatatan keuangan pribadi — Backend API
// @termsOfService      http://swagger.io/terms/
// @contact.name        Developer
// @contact.email       admin@example.com
// @license.name        Private
// @host                localhost:8080
// @BasePath            /api/v1
//
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 Masukkan token dengan format: Bearer <access_token>
func main() {
	fmt.Println(`
██████╗ ███████╗██████╗ ███████╗ ██████╗ ███╗   ██╗ █████╗ ██╗
██╔══██╗██╔════╝██╔══██╗██╔════╝██╔═══██╗████╗  ██║██╔══██╗██║
██████╔╝█████╗  ██████╔╝█████╗  ██║   ██║██╔██╗ ██║███████║██║
██╔═══╝ ██╔══╝  ██╔══██╗██╔══╝  ██║   ██║██║╚██╗██║██╔══██║██║
██║     ███████╗██║  ██║██║     ╚██████╔╝██║ ╚████║██║  ██║███████╗
╚═╝     ╚══════╝╚═╝  ╚═╝╚═╝      ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═╝╚══════╝
Personal Finance Tracker API
	`)

	debug := flag.Bool("d", false, "sets log level to debug")
	flag.Parse()

	if *debug {
		log.Println("Debug mode enabled")
	}

	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system env vars")
	}

	// Config
	cfg := config.Load()

	// Set Swagger host dari config
	docs.SwaggerInfo.Host = cfg.SwaggerHost

	// Database connection — pgxpool langsung (pola user)
	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	connPool, err := pgxpool.New(dbCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Can't connect to database: %v", err)
	}
	cancel()
	defer connPool.Close()

	// Run migrations
	db, err := repository.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to create DB connection: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	if err := db.SeedAdmin(); err != nil {
		log.Printf("Warning: seed admin: %v", err)
	}
	db.Close()

	// Repositories
	userRepo := repository.NewUserRepository(connPool)
	categoryRepo := repository.NewCategoryRepository(connPool)
	transactionRepo := repository.NewTransactionRepository(connPool)
	summaryRepo := repository.NewSummaryRepository(connPool)
	refreshTokenRepo := repository.NewRefreshTokenRepository(connPool)

	// Services
	authService := service.NewAuthService(userRepo, categoryRepo, refreshTokenRepo)
	categoryService := service.NewCategoryService(categoryRepo)
	transactionService := service.NewTransactionService(transactionRepo, categoryRepo)
	summaryService := service.NewSummaryService(summaryRepo)

	// Background goroutines
	go StartTokenCleanup(refreshTokenRepo)
	go StartDBHealthCheck(connPool, 5*time.Minute)

	// Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "Personal Finance Tracker API",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	})

	// Global middleware
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSOrigins,
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))
	app.Use(limiter.New(limiter.Config{
		Max: cfg.RateLimitPerMinute,
	}))

	// Health probes
	app.Get("/health", HealthHandler)
	app.Get("/ready", ReadinessHandler(connPool))

	// Swagger
	app.Get("/swagger/*", swagger.HandlerDefault)
	app.Get("/swagger", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/index.html")
	})

	// API routes
	router.SetupRoutes(app, authService, categoryService, transactionService, summaryService)

	// Signal handling goroutine (pola user)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("Shutting down server...")
		if err := app.Shutdown(); err != nil {
			log.Printf("Forced shutdown: %v", err)
		}
	}()

	// Start server
	log.Printf("Server starting on port %s", cfg.Port)
	if err := app.Listen(":" + cfg.Port); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
