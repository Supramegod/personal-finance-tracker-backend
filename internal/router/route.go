// Package router bertanggung jawab atas semua definisi routing API.
// Routing dipisahkan dari main.go agar lebih mudah di-test dan dikelola.
package router

import (
	"github.com/gofiber/fiber/v2"

	"personal-finance-tracker/internal/handler"
	"personal-finance-tracker/internal/middleware"
	"personal-finance-tracker/internal/service"
)

// SetupRoutes mendaftarkan semua endpoint API ke aplikasi Fiber.
// Endpoint dibagi menjadi:
//   - Public: auth (login, refresh, logout)
//   - Protected: categories, transactions, summary (memerlukan JWT)
func SetupRoutes(
	app *fiber.App,
	authService *service.AuthService,
	categoryService *service.CategoryService,
	transactionService *service.TransactionService,
	summaryService *service.SummaryService,
) {
	// ─── Public routes ──────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authService)
	app.Post("/api/v1/auth/login", authHandler.Login)
	app.Post("/api/v1/auth/refresh", authHandler.Refresh)
	app.Post("/api/v1/auth/logout", authHandler.Logout)

	// ─── Protected routes (memerlukan JWT) ──────────────────────────
	protected := app.Group("/api/v1", middleware.AuthMiddleware())
	{
		// Categories
		categoryHandler := handler.NewCategoryHandler(categoryService)
		protected.Get("/categories", categoryHandler.List)
		protected.Post("/categories", categoryHandler.Create)

		// Transactions
		transactionHandler := handler.NewTransactionHandler(transactionService)
		protected.Get("/transactions", transactionHandler.List)
		protected.Post("/transactions", transactionHandler.Create)
		protected.Get("/transactions/calendar", transactionHandler.GetCalendar)
		protected.Get("/transactions/:id", transactionHandler.GetByID)
		protected.Put("/transactions/:id", transactionHandler.Update)
		protected.Delete("/transactions/:id", transactionHandler.Delete)

		// Summary / Laporan
		summaryHandler := handler.NewSummaryHandler(summaryService)
		protected.Get("/summary/balance", summaryHandler.Balance)
		protected.Get("/summary/report", summaryHandler.Report)
	}
}
