package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"personal-finance-tracker/internal/repository"
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

// Regenerate godoc
// @Summary Buat ulang insight AI untuk satu bulan (owner-only)
// @Tags AI Insights
// @Produce json
// @Security BearerAuth
// @Param month query string true "Bulan YYYY-MM"
// @Success 202 {object} map[string]string
// @Router /summary/ai-insights/regenerate [post]
func (h *AIInsightHandler) Regenerate(c *fiber.Ctx) error {
	month, err := time.Parse("2006-01", c.Query("month"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "month must use YYYY-MM format"})
	}
	err = h.service.Regenerate(c.Locals("group_id").(string), c.Locals("user_id").(string), month)
	if err != nil {
		return regenerateError(c, err)
	}
	return c.Status(202).JSON(fiber.Map{"status": "processing", "period": month.Format("2006-01")})
}

// regenerateError memetakan kegagalan Regenerate ke status HTTP yang tepat.
//
// Dipisah dari consentError karena kondisinya berbeda dan dua di antaranya
// bukan kesalahan pengguna: 409 berarti pekerjaan yang sama sedang berjalan,
// 429 berarti terlalu cepat setelah regenerasi sebelumnya. Selebihnya
// diperlakukan sebagai kegagalan internal dengan pesan generik, sejalan dengan
// handler lain di berkas ini.
func regenerateError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, repository.ErrRegenerateNotOwner):
		return c.Status(403).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, repository.ErrRegenerateInProgress):
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, repository.ErrRegenerateTooSoon):
		return c.Status(429).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, service.ErrRegenerateFutureMonth):
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	case err.Error() == "AI insights are not configured":
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(500).JSON(fiber.Map{"error": "failed to regenerate AI insight"})
	}
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
		return consentError(c, err)
	}
	return c.JSON(result)
}

// consentError memetakan kegagalan SetConsent ke status HTTP yang tepat.
//
// Sebelumnya semua error dibalas 403 dengan err.Error() mentah. Dua
// akibatnya: kegagalan database dilaporkan kepada pengguna sebagai
// "forbidden" (menyesatkan saat debug), dan pesan driver Postgres —
// termasuk potongan query dan nama kolom — bocor ke klien.
//
// Hanya dua kondisi yang benar-benar diketahui berasal dari service;
// selebihnya diperlakukan sebagai kegagalan internal dan dibalas pesan
// generik, sejalan dengan ByMonth/Latest di file yang sama.
func consentError(c *fiber.Ctx, err error) error {
	switch err.Error() {
	case "only owner can manage AI consent":
		return c.Status(403).JSON(fiber.Map{"error": err.Error()})
	case "AI insights are not configured":
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	default:
		return c.Status(500).JSON(fiber.Map{"error": "failed to update AI consent"})
	}
}
