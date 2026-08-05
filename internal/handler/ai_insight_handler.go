package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"personal-finance-tracker/internal/service"
)

type AIInsightHandler struct{ service *service.AIInsightService }

func NewAIInsightHandler(service *service.AIInsightService) *AIInsightHandler {
	return &AIInsightHandler{service: service}
}

// ByMonth godoc
// @Summary Insight AI untuk bulan tertentu
// @Tags AI Insights
// @Produce json
// @Security BearerAuth
// @Param month query string true "Bulan YYYY-MM"
// @Success 200 {object} service.InsightResponse
// @Router /summary/ai-insights [get]
func (h *AIInsightHandler) ByMonth(c *fiber.Ctx) error {
	month, err := time.Parse("2006-01", c.Query("month"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "month must use YYYY-MM format"})
	}
	result, err := h.service.Get(c.Locals("group_id").(string), month)
	if err == pgx.ErrNoRows {
		return c.JSON(fiber.Map{"status": "not_available", "period": month.Format("2006-01")})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get AI insight"})
	}
	return c.JSON(result)
}

// Latest godoc
// @Summary Insight AI terbaru untuk dashboard
// @Tags AI Insights
// @Produce json
// @Security BearerAuth
// @Success 200 {object} service.InsightResponse
// @Router /summary/ai-insights/latest [get]
func (h *AIInsightHandler) Latest(c *fiber.Ctx) error {
	result, err := h.service.Latest(c.Locals("group_id").(string))
	if err == pgx.ErrNoRows {
		return c.JSON(fiber.Map{"status": "not_available"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get AI insight"})
	}
	return c.JSON(result)
}

// Consent godoc
// @Summary Status consent AI kelompok
// @Tags AI Insights
// @Produce json
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Success 200 {object} repository.AIConsent
// @Router /groups/{id}/ai-consent [get]
func (h *AIInsightHandler) Consent(c *fiber.Ctx) error {
	result, err := h.service.GetConsent(c.Params("id"), c.Locals("user_id").(string))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "group not found"})
	}
	return c.JSON(result)
}

// UpdateConsent godoc
// @Summary Ubah consent AI kelompok (owner-only)
// @Tags AI Insights
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Param request body object true "{enabled: boolean}"
// @Success 200 {object} repository.AIConsent
// @Router /groups/{id}/ai-consent [put]
func (h *AIInsightHandler) UpdateConsent(c *fiber.Ctx) error {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	result, err := h.service.SetConsent(c.Params("id"), c.Locals("user_id").(string), body.Enabled)
	if err != nil {
		return c.Status(403).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}
