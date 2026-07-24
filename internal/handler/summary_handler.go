package handler

import (
	"github.com/gofiber/fiber/v2"

	"personal-finance-tracker/internal/service"
)

type SummaryHandler struct {
	summaryService *service.SummaryService
}

func NewSummaryHandler(summaryService *service.SummaryService) *SummaryHandler {
	return &SummaryHandler{summaryService: summaryService}
}

// GetBalance godoc
// @Summary Saldo berjalan
// @Description Mendapatkan saldo terkini (running balance) = total income - total expense
// @Tags Summary
// @Produce json
// @Security BearerAuth
// @Success 200 {object} service.BalanceResponse
// @Failure 500 {object} map[string]string
// @Router /summary/balance [get]
func (h *SummaryHandler) Balance(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)

	result, err := h.summaryService.GetBalance(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get balance",
		})
	}

	return c.JSON(result)
}

// GetReport godoc
// @Summary Laporan keuangan
// @Description Mendapatkan ringkasan income vs expense per periode
// @Tags Summary
// @Produce json
// @Security BearerAuth
// @Param period query string false "Periode: daily / weekly / monthly (default: monthly)"
// @Param from query string true "Tanggal awal (YYYY-MM-DD)"
// @Param to query string true "Tanggal akhir (YYYY-MM-DD)"
// @Success 200 {object} service.ReportResponse
// @Failure 400 {object} map[string]string
// @Router /summary/report [get]
func (h *SummaryHandler) Report(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)

	period := c.Query("period", "monthly")
	from := c.Query("from")
	to := c.Query("to")

	if from == "" || to == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "from and to query parameters are required",
		})
	}

	result, err := h.summaryService.GetReport(userID, period, from, to)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get report",
		})
	}

	return c.JSON(result)
}
