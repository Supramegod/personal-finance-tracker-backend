package handler

import (
	"github.com/gofiber/fiber/v2"

	"personal-finance-tracker/internal/service"
)

type InstallmentHandler struct {
	installmentService *service.InstallmentService
}

func NewInstallmentHandler(installmentService *service.InstallmentService) *InstallmentHandler {
	return &InstallmentHandler{installmentService: installmentService}
}

// ListInstallments godoc
// @Summary Daftar cicilan
// @Description Mendapatkan semua cicilan milik user beserta progres pembayaran
// @Tags Installments
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Router /installments [get]
func (h *InstallmentHandler) List(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)

	items, err := h.installmentService.List(groupID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get installments",
		})
	}

	return c.JSON(fiber.Map{"data": items})
}

type createInstallmentRequest struct {
	CategoryID    string  `json:"category_id"`
	Title         string  `json:"title"`
	MonthlyAmount float64 `json:"monthly_amount"`
	TenorMonths   int     `json:"tenor_months"`
	StartDate     string  `json:"start_date"`
	Note          string  `json:"note"`
}

// CreateInstallment godoc
// @Summary Buat cicilan baru
// @Description Membuat rencana cicilan bulanan (nominal per bulan + tenor)
// @Tags Installments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createInstallmentRequest true "Data cicilan"
// @Success 201 {object} repository.Installment
// @Failure 400 {object} map[string]string
// @Router /installments [post]
func (h *InstallmentHandler) Create(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	userID := c.Locals("user_id").(string)

	var req createInstallmentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	inst, err := h.installmentService.Create(service.CreateInstallmentInput{
		GroupID:       groupID,
		UserID:        userID,
		CategoryID:    req.CategoryID,
		Title:         req.Title,
		MonthlyAmount: req.MonthlyAmount,
		TenorMonths:   req.TenorMonths,
		StartDate:     req.StartDate,
		Note:          req.Note,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(inst)
}

// GetInstallment godoc
// @Summary Detail cicilan
// @Description Mendapatkan detail satu cicilan beserta riwayat pembayaran
// @Tags Installments
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Cicilan"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]string
// @Router /installments/{id} [get]
func (h *InstallmentHandler) GetByID(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	id := c.Params("id")

	inst, err := h.installmentService.GetByID(id, groupID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "installment not found",
		})
	}

	payments, err := h.installmentService.ListPayments(id, groupID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to get payments",
		})
	}

	return c.JSON(fiber.Map{
		"installment": inst,
		"payments":    payments,
	})
}

// PayInstallment godoc
// @Summary Bayar cicilan bulan ini
// @Description Mencatat pembayaran cicilan bulan berikutnya. Membuat transaksi
// @Description pengeluaran (expense) senilai cicilan per bulan.
// @Tags Installments
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Cicilan"
// @Success 201 {object} repository.InstallmentPayment
// @Failure 400 {object} map[string]string
// @Router /installments/{id}/pay [post]
func (h *InstallmentHandler) Pay(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	userID := c.Locals("user_id").(string)
	id := c.Params("id")

	pay, err := h.installmentService.Pay(id, groupID, userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(pay)
}

// DeleteInstallment godoc
// @Summary Hapus cicilan
// @Description Menghapus rencana cicilan. Transaksi pengeluaran yang sudah
// @Description tercatat TIDAK ikut terhapus.
// @Tags Installments
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Cicilan"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /installments/{id} [delete]
func (h *InstallmentHandler) Delete(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	id := c.Params("id")

	if err := h.installmentService.Delete(id, groupID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{"message": "installment deleted successfully"})
}
