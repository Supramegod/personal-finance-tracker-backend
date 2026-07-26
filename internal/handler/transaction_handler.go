package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"personal-finance-tracker/internal/repository"
	"personal-finance-tracker/internal/service"
)

type TransactionHandler struct {
	transactionService *service.TransactionService
}

func NewTransactionHandler(transactionService *service.TransactionService) *TransactionHandler {
	return &TransactionHandler{transactionService: transactionService}
}

// ListTransactions godoc
// @Summary Daftar transaksi
// @Description Mendapatkan daftar transaksi dengan filter dan pagination
// @Tags Transactions
// @Produce json
// @Security BearerAuth
// @Param from query string false "Filter tanggal awal (YYYY-MM-DD)"
// @Param to query string false "Filter tanggal akhir (YYYY-MM-DD)"
// @Param category_id query string false "Filter by kategori"
// @Param type query string false "Filter by type: income / expense"
// @Param page query int false "Halaman (default: 1)"
// @Param limit query int false "Item per halaman (default: 20)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Security BearerAuth
// @Router /transactions [get]
func (h *TransactionHandler) List(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	transactions, total, err := h.transactionService.List(service.ListTransactionsInput{
		GroupID:    groupID,
		From:       c.Query("from"),
		To:         c.Query("to"),
		CategoryID: c.Query("category_id"),
		Type:       c.Query("type"),
		Page:       page,
		Limit:      limit,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Pastikan hasil JSON tetap [] bukan null saat transaksi kosong
	if transactions == nil {
		transactions = []repository.Transaction{}
	}

	return c.JSON(fiber.Map{
		"data":  transactions,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

type createTransactionRequest struct {
	CategoryID      string  `json:"category_id"`
	Type            string  `json:"type"`
	Amount          float64 `json:"amount"`
	TransactionDate string  `json:"transaction_date"`
	Note            string  `json:"note"`
}

// CreateTransaction godoc
// @Summary Buat transaksi baru
// @Description Mencatat transaksi income atau expense baru
// @Tags Transactions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createTransactionRequest true "Data transaksi"
// @Success 201 {object} repository.Transaction
// @Failure 400 {object} map[string]string
// @Router /transactions [post]
func (h *TransactionHandler) Create(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	userID := c.Locals("user_id").(string)

	var req createTransactionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	transaction, err := h.transactionService.Create(service.CreateTransactionInput{
		GroupID:         groupID,
		UserID:          userID,
		CategoryID:      req.CategoryID,
		Type:            req.Type,
		Amount:          req.Amount,
		TransactionDate: req.TransactionDate,
		Note:            req.Note,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(transaction)
}

// GetCalendar godoc
// @Summary Kalender transaksi bulanan
// @Description Mendapatkan ringkasan transaksi per tanggal dalam satu bulan (default: bulan berjalan jika tidak ada query)
// @Tags Transactions
// @Produce json
// @Security BearerAuth
// @Param month query string false "Bulan (YYYY-MM) — default: bulan berjalan"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Router /transactions/calendar [get]
func (h *TransactionHandler) GetCalendar(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	month := c.Query("month")

	days, err := h.transactionService.GetCalendarMonth(groupID, month)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Ensure we return [] not null when empty
	if days == nil {
		days = []repository.CalendarDay{}
	}

	return c.JSON(fiber.Map{
		"data": days,
	})
}

// GetTransactionByID godoc
// @Summary Detail transaksi
// @Description Mendapatkan detail transaksi berdasarkan ID
// @Tags Transactions
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Transaksi"
// @Success 200 {object} repository.Transaction
// @Failure 404 {object} map[string]string
// @Router /transactions/{id} [get]
func (h *TransactionHandler) GetByID(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	id := c.Params("id")

	transaction, err := h.transactionService.GetByID(id, groupID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "transaction not found",
		})
	}

	return c.JSON(transaction)
}

type updateTransactionRequest struct {
	CategoryID      string  `json:"category_id"`
	Type            string  `json:"type"`
	Amount          float64 `json:"amount"`
	TransactionDate string  `json:"transaction_date"`
	Note            string  `json:"note"`
}

// UpdateTransaction godoc
// @Summary Update transaksi
// @Description Memperbarui data transaksi berdasarkan ID
// @Tags Transactions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Transaksi"
// @Param request body updateTransactionRequest true "Data transaksi yang diupdate"
// @Success 200 {object} repository.Transaction
// @Failure 400 {object} map[string]string
// @Router /transactions/{id} [put]
func (h *TransactionHandler) Update(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	id := c.Params("id")

	var req updateTransactionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	transaction, err := h.transactionService.Update(service.UpdateTransactionInput{
		ID:              id,
		GroupID:         groupID,
		CategoryID:      req.CategoryID,
		Type:            req.Type,
		Amount:          req.Amount,
		TransactionDate: req.TransactionDate,
		Note:            req.Note,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(transaction)
}

// DeleteTransaction godoc
// @Summary Hapus transaksi
// @Description Menghapus transaksi berdasarkan ID
// @Tags Transactions
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Transaksi"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /transactions/{id} [delete]
func (h *TransactionHandler) Delete(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	id := c.Params("id")

	if err := h.transactionService.Delete(id, groupID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "transaction not found",
		})
	}

	return c.JSON(fiber.Map{
		"message": "transaction deleted successfully",
	})
}
