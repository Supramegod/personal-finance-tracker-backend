package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"personal-finance-tracker/internal/repository"
	"personal-finance-tracker/internal/service"
)

type SavingsHandler struct {
	savingsService *service.SavingsService
}

func NewSavingsHandler(savingsService *service.SavingsService) *SavingsHandler {
	return &SavingsHandler{savingsService: savingsService}
}

// savingsError membalas error dengan status yang tepat.
//
// Dicocokkan lewat errors.Is/errors.As, bukan lewat isi pesan — pesan boleh
// berubah, identitas error tidak. Hanya error domain yang pesannya diteruskan
// ke klien; sisanya dianggap kegagalan sistem, di-log lengkap di server dan
// dibalas pesan generik supaya nama tabel atau pesan driver tidak bocor.
func savingsError(c *fiber.Ctx, err error) error {
	var validation *service.ValidationError

	switch {
	case errors.Is(err, service.ErrSavingsNotFound),
		errors.Is(err, service.ErrSavingsEntryNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})

	// Konflik keadaan, bukan permintaan yang cacat: tarik melebihi saldo, atau
	// hapus pot yang masih berisi uang.
	case errors.Is(err, service.ErrSavingsInsufficient),
		errors.Is(err, service.ErrSavingsNotEmpty):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})

	case errors.As(err, &validation):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": validation.Message})
	}

	log.Printf("[savings] unexpected error on %s %s: %v", c.Method(), c.Path(), err)
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"error": "failed to process savings request",
	})
}

// savingsScope mengambil group_id dan user_id dari context auth. Bentuk dua
// nilai, bukan type assertion telanjang: aplikasi tidak memasang middleware
// Recover, jadi assertion yang gagal akan menjatuhkan seluruh proses.
func savingsScope(c *fiber.Ctx) (groupID, userID string, ok bool) {
	groupID, gOK := c.Locals("group_id").(string)
	userID, uOK := c.Locals("user_id").(string)
	return groupID, userID, gOK && uOK
}

// ListSavings godoc
// @Summary Daftar tabungan
// @Description Mendapatkan semua pot tabungan milik kelompok beserta progresnya
// @Tags Savings
// @Produce json
// @Security BearerAuth
// @Param status query string false "Filter status: active, completed, archived"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Router /savings [get]
func (h *SavingsHandler) List(c *fiber.Ctx) error {
	groupID, _, ok := savingsScope(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	items, err := h.savingsService.List(groupID, c.Query("status"))
	if err != nil {
		return savingsError(c, err)
	}

	return c.JSON(fiber.Map{"data": items})
}

type createSavingsRequest struct {
	Name         string   `json:"name"`
	TargetAmount *float64 `json:"target_amount"`
	TargetDate   string   `json:"target_date"`
	Icon         string   `json:"icon"`
	Color        string   `json:"color"`
	Note         string   `json:"note"`
}

// CreateSavings godoc
// @Summary Buat tabungan baru
// @Description Membuat pot tabungan. target_amount dan target_date opsional —
// @Description pot tanpa target (mis. dana darurat) tetap sah.
// @Tags Savings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createSavingsRequest true "Data tabungan"
// @Success 201 {object} repository.SavingsGoal
// @Failure 400 {object} map[string]string
// @Router /savings [post]
func (h *SavingsHandler) Create(c *fiber.Ctx) error {
	groupID, userID, ok := savingsScope(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req createSavingsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	goal, err := h.savingsService.Create(service.CreateSavingsGoalInput{
		GroupID:      groupID,
		UserID:       userID,
		Name:         req.Name,
		TargetAmount: req.TargetAmount,
		TargetDate:   req.TargetDate,
		Icon:         req.Icon,
		Color:        req.Color,
		Note:         req.Note,
	})
	if err != nil {
		return savingsError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(goal)
}

// GetSavings godoc
// @Summary Detail tabungan
// @Description Mendapatkan satu pot tabungan beserta riwayat mutasinya
// @Tags Savings
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Tabungan"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]string
// @Router /savings/{id} [get]
func (h *SavingsHandler) GetByID(c *fiber.Ctx) error {
	groupID, _, ok := savingsScope(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	id := c.Params("id")

	goal, err := h.savingsService.GetByID(id, groupID)
	if err != nil {
		return savingsError(c, err)
	}

	entries, err := h.savingsService.ListEntries(id, groupID)
	if err != nil {
		return savingsError(c, err)
	}

	return c.JSON(fiber.Map{
		"goal":    goal,
		"entries": entries,
	})
}

type updateSavingsRequest struct {
	Name         string   `json:"name"`
	TargetAmount *float64 `json:"target_amount"`
	TargetDate   string   `json:"target_date"`
	Icon         string   `json:"icon"`
	Color        string   `json:"color"`
	Note         string   `json:"note"`
	Status       string   `json:"status"`
}

// UpdateSavings godoc
// @Summary Ubah tabungan
// @Description Mengubah nama, target, tenggat, atau status pot tabungan.
// @Description Saldo tidak bisa diubah dari sini — saldo adalah turunan mutasi.
// @Tags Savings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Tabungan"
// @Param request body updateSavingsRequest true "Data tabungan"
// @Success 200 {object} repository.SavingsGoal
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /savings/{id} [put]
func (h *SavingsHandler) Update(c *fiber.Ctx) error {
	groupID, _, ok := savingsScope(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	id := c.Params("id")

	var req updateSavingsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	goal, err := h.savingsService.Update(service.UpdateSavingsGoalInput{
		ID:           id,
		GroupID:      groupID,
		Name:         req.Name,
		TargetAmount: req.TargetAmount,
		TargetDate:   req.TargetDate,
		Icon:         req.Icon,
		Color:        req.Color,
		Note:         req.Note,
		Status:       req.Status,
	})
	if err != nil {
		return savingsError(c, err)
	}

	return c.JSON(goal)
}

// DeleteSavings godoc
// @Summary Hapus tabungan
// @Description Menghapus pot tabungan. Ditolak bila saldonya masih ada —
// @Description tarik seluruh saldo lebih dulu. Transaksi yang sudah tercatat
// @Description TIDAK ikut terhapus.
// @Tags Savings
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Tabungan"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /savings/{id} [delete]
func (h *SavingsHandler) Delete(c *fiber.Ctx) error {
	groupID, _, ok := savingsScope(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	id := c.Params("id")

	if err := h.savingsService.Delete(id, groupID); err != nil {
		return savingsError(c, err)
	}

	return c.JSON(fiber.Map{"message": "savings goal deleted successfully"})
}

type savingsEntryRequest struct {
	Amount float64 `json:"amount"`
	Date   string  `json:"date"`
	Note   string  `json:"note"`
}

// DepositSavings godoc
// @Summary Setor ke tabungan
// @Description Mencatat setoran. Membuat transaksi pengeluaran bertanda
// @Description transfer: saldo kas turun, tapi tidak dihitung sebagai
// @Description pengeluaran di laporan.
// @Tags Savings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Tabungan"
// @Param request body savingsEntryRequest true "Nominal setoran"
// @Success 201 {object} repository.SavingsEntry
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /savings/{id}/deposit [post]
func (h *SavingsHandler) Deposit(c *fiber.Ctx) error {
	return h.handleEntry(c, false)
}

// WithdrawSavings godoc
// @Summary Tarik dari tabungan
// @Description Mencatat penarikan. Membuat transaksi pemasukan bertanda
// @Description transfer. Ditolak bila melebihi saldo pot.
// @Tags Savings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Tabungan"
// @Param request body savingsEntryRequest true "Nominal penarikan"
// @Success 201 {object} repository.SavingsEntry
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /savings/{id}/withdraw [post]
func (h *SavingsHandler) Withdraw(c *fiber.Ctx) error {
	return h.handleEntry(c, true)
}

// handleEntry menampung bagian yang identik antara setor dan tarik. Keduanya
// hanya berbeda pada arah mutasi, jadi memisahkannya jadi dua fungsi utuh
// cuma menggandakan parsing dan penanganan error yang sama persis.
func (h *SavingsHandler) handleEntry(c *fiber.Ctx, withdraw bool) error {
	groupID, userID, ok := savingsScope(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	id := c.Params("id")

	var req savingsEntryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	input := service.SavingsEntryInput{
		GoalID:  id,
		GroupID: groupID,
		UserID:  userID,
		Amount:  req.Amount,
		Date:    req.Date,
		Note:    req.Note,
	}

	var (
		entry *repository.SavingsEntry
		err   error
	)
	if withdraw {
		entry, err = h.savingsService.Withdraw(input)
	} else {
		entry, err = h.savingsService.Deposit(input)
	}
	if err != nil {
		return savingsError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(entry)
}

// DeleteSavingsEntry godoc
// @Summary Batalkan mutasi tabungan
// @Description Menghapus satu setoran/penarikan beserta transaksi yang
// @Description tercipta bersamanya. Ini satu-satunya cara membatalkan mutasi —
// @Description transaksinya tidak bisa dihapus dari halaman Transaksi.
// @Tags Savings
// @Produce json
// @Security BearerAuth
// @Param id path string true "ID Tabungan"
// @Param entryId path string true "ID Mutasi"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /savings/{id}/entries/{entryId} [delete]
func (h *SavingsHandler) DeleteEntry(c *fiber.Ctx) error {
	groupID, _, ok := savingsScope(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	id := c.Params("id")
	entryID := c.Params("entryId")

	if err := h.savingsService.DeleteEntry(entryID, id, groupID); err != nil {
		return savingsError(c, err)
	}

	return c.JSON(fiber.Map{"message": "savings entry deleted successfully"})
}
