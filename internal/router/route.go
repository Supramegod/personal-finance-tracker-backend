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
	installmentService *service.InstallmentService,
	groupService *service.GroupService,
	aiInsightService *service.AIInsightService,
) {
	// ─── Public routes ──────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authService)
	app.Post("/api/v1/auth/login", authHandler.Login)
	app.Post("/api/v1/auth/refresh", authHandler.Refresh)
	app.Post("/api/v1/auth/logout", authHandler.Logout)

	// ─── Protected routes (memerlukan JWT) ──────────────────────────
	protected := app.Group("/api/v1", middleware.AuthMiddleware())
	{
		// Groups & multi-user (kelompok)
		groupHandler := handler.NewGroupHandler(groupService, authService)
		protected.Post("/auth/switch-group", groupHandler.SwitchGroup)
		protected.Get("/groups", groupHandler.List)
		protected.Post("/groups", groupHandler.Create)
		protected.Get("/users", groupHandler.ListManagedUsers)
		protected.Post("/users", groupHandler.CreateUser)
		protected.Get("/groups/:id/members", groupHandler.ListMembers)
		protected.Post("/groups/:id/members", groupHandler.AddMember)
		protected.Delete("/groups/:id/members/:userId", groupHandler.RemoveMember)
		aiHandler := handler.NewAIInsightHandler(aiInsightService)
		protected.Get("/groups/:id/ai-consent", aiHandler.Consent)
		protected.Put("/groups/:id/ai-consent", aiHandler.UpdateConsent)

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
		protected.Get("/summary/ai-insights", aiHandler.ByMonth)
		protected.Get("/summary/ai-insights/latest", aiHandler.Latest)

		// Installments / Cicilan
		installmentHandler := handler.NewInstallmentHandler(installmentService)
		protected.Get("/installments", installmentHandler.List)
		protected.Post("/installments", installmentHandler.Create)
		protected.Get("/installments/:id", installmentHandler.GetByID)
		protected.Post("/installments/:id/pay", installmentHandler.Pay)
		protected.Delete("/installments/:id", installmentHandler.Delete)
	}
}
