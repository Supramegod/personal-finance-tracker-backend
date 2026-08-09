package service

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"personal-finance-tracker/internal/repository"
)

type SavingsService struct {
	repo         *repository.SavingsRepository
	categoryRepo *repository.CategoryRepository
}

func NewSavingsService(
	repo *repository.SavingsRepository,
	categoryRepo *repository.CategoryRepository,
) *SavingsService {
	return &SavingsService{repo: repo, categoryRepo: categoryRepo}
}

// ─── Pot tabungan ───────────────────────────────────────────────────

type CreateSavingsGoalInput struct {
	GroupID      string
	UserID       string
	Name         string
	TargetAmount *float64
	TargetDate   string
	Icon         string
	Color        string
	Note         string
}

func (s *SavingsService) Create(input CreateSavingsGoalInput) (*repository.SavingsGoal, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return nil, invalid("name is required")
	}
	if input.TargetAmount != nil && *input.TargetAmount <= 0 {
		return nil, invalid("target_amount must be greater than 0")
	}

	targetDate, err := parseOptionalDate(input.TargetDate, "target_date")
	if err != nil {
		return nil, err
	}

	goal := &repository.SavingsGoal{
		GroupID:      input.GroupID,
		UserID:       input.UserID,
		Name:         input.Name,
		TargetAmount: input.TargetAmount,
		TargetDate:   targetDate,
		Icon:         input.Icon,
		Color:        input.Color,
		Note:         input.Note,
	}
	if err := s.repo.Create(goal); err != nil {
		return nil, err
	}
	return goal, nil
}

// List mengembalikan pot milik kelompok, sudah dilengkapi nilai turunan.
func (s *SavingsService) List(groupID, status string) ([]repository.SavingsGoalWithProgress, error) {
	if status != "" && !isValidSavingsStatus(status) {
		return nil, invalid("status must be 'active', 'completed', or 'archived'")
	}

	items, err := s.repo.List(groupID, status)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for i := range items {
		enrichSavingsGoal(&items[i], now)
	}
	return items, nil
}

func (s *SavingsService) GetByID(id, groupID string) (*repository.SavingsGoalWithProgress, error) {
	goal, err := s.repo.FindByID(id, groupID)
	if err != nil {
		return nil, translateSavingsErr(err)
	}
	saved, err := s.repo.SavedAmount(goal.ID)
	if err != nil {
		return nil, err
	}
	item := &repository.SavingsGoalWithProgress{SavingsGoal: *goal, SavedAmount: saved}
	enrichSavingsGoal(item, time.Now())
	return item, nil
}

func (s *SavingsService) ListEntries(id, groupID string) ([]repository.SavingsEntry, error) {
	// Pastikan pot milik kelompok sebelum mengembalikan mutasinya.
	if _, err := s.repo.FindByID(id, groupID); err != nil {
		return nil, translateSavingsErr(err)
	}
	return s.repo.ListEntries(id)
}

type UpdateSavingsGoalInput struct {
	ID           string
	GroupID      string
	Name         string
	TargetAmount *float64
	TargetDate   string
	Icon         string
	Color        string
	Note         string
	Status       string
}

func (s *SavingsService) Update(input UpdateSavingsGoalInput) (*repository.SavingsGoal, error) {
	existing, err := s.repo.FindByID(input.ID, input.GroupID)
	if err != nil {
		return nil, translateSavingsErr(err)
	}

	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return nil, invalid("name is required")
	}
	if input.TargetAmount != nil && *input.TargetAmount <= 0 {
		return nil, invalid("target_amount must be greater than 0")
	}
	if input.Status != "" && !isValidSavingsStatus(input.Status) {
		return nil, invalid("status must be 'active', 'completed', or 'archived'")
	}

	targetDate, err := parseOptionalDate(input.TargetDate, "target_date")
	if err != nil {
		return nil, err
	}

	existing.Name = input.Name
	existing.TargetAmount = input.TargetAmount
	existing.TargetDate = targetDate
	existing.Icon = input.Icon
	existing.Color = input.Color
	existing.Note = input.Note
	if input.Status != "" {
		existing.Status = input.Status
	}

	if err := s.repo.Update(existing); err != nil {
		return nil, translateSavingsErr(err)
	}
	return existing, nil
}

func (s *SavingsService) Delete(id, groupID string) error {
	return translateSavingsErr(s.repo.Delete(id, groupID))
}

// ─── Mutasi ─────────────────────────────────────────────────────────

type SavingsEntryInput struct {
	GoalID  string
	GroupID string
	UserID  string
	Amount  float64
	Date    string
	Note    string
}

// Deposit menyetor ke pot: membuat transaksi expense bertanda transfer.
//
// Sengaja TIDAK memvalidasi setoran terhadap saldo kas. Saldo di aplikasi belum
// tentu mencerminkan seluruh uang user (ada kas di luar catatan), jadi menolak
// setoran karena "saldo kurang" akan salah lebih sering daripada benar — itu
// urusan peringatan di UI, bukan error di sini.
func (s *SavingsService) Deposit(input SavingsEntryInput) (*repository.SavingsEntry, error) {
	return s.addEntry(input, repository.SavingsDirectionDeposit)
}

// Withdraw menarik dari pot: membuat transaksi income bertanda transfer.
// Ditolak bila melebihi saldo pot — saldo pot tidak boleh negatif, dan
// pengecekan finalnya ada di dalam transaksi DB (lihat repository.AddEntry).
func (s *SavingsService) Withdraw(input SavingsEntryInput) (*repository.SavingsEntry, error) {
	return s.addEntry(input, repository.SavingsDirectionWithdraw)
}

func (s *SavingsService) addEntry(input SavingsEntryInput, direction string) (*repository.SavingsEntry, error) {
	if input.Amount <= 0 {
		return nil, invalid("amount must be greater than 0")
	}

	goal, err := s.repo.FindByID(input.GoalID, input.GroupID)
	if err != nil {
		return nil, translateSavingsErr(err)
	}

	date, err := parseEntryDate(input.Date)
	if err != nil {
		return nil, err
	}

	categoryID, err := s.systemCategoryID(input.GroupID, input.UserID, direction)
	if err != nil {
		return nil, err
	}

	note := strings.TrimSpace(input.Note)
	if note == "" {
		if direction == repository.SavingsDirectionDeposit {
			note = fmt.Sprintf("Setor tabungan: %s", goal.Name)
		} else {
			note = fmt.Sprintf("Tarik tabungan: %s", goal.Name)
		}
	}

	entry, err := s.repo.AddEntry(repository.AddEntryParams{
		GoalID:     goal.ID,
		GroupID:    input.GroupID,
		UserID:     input.UserID,
		CategoryID: categoryID,
		Direction:  direction,
		Amount:     input.Amount,
		Date:       date,
		Note:       note,
	})
	if err != nil {
		return nil, translateSavingsErr(err)
	}
	return entry, nil
}

// DeleteEntry membatalkan satu mutasi beserta transaksinya.
func (s *SavingsService) DeleteEntry(entryID, goalID, groupID string) error {
	return translateSavingsErr(s.repo.DeleteEntry(entryID, goalID, groupID))
}

// TotalSaved mengembalikan total tabungan kelompok. Dipakai SummaryService
// untuk menyusun kekayaan bersih.
func (s *SavingsService) TotalSaved(groupID string) (float64, error) {
	return s.repo.TotalSaved(groupID)
}

// systemCategoryID mengembalikan id kategori sistem untuk arah mutasi, membuatnya
// bila belum ada. Migrasi 013 sudah mem-backfill kedua kategori ke semua group,
// tapi group yang dibuat oleh versi lama seed bisa saja belum punya — jadi
// dipastikan di sini daripada gagal dengan "category not found" yang
// membingungkan user.
func (s *SavingsService) systemCategoryID(groupID, userID, direction string) (string, error) {
	name, catType, icon := repository.SavingsDepositCategoryName, "expense", "savings"
	if direction == repository.SavingsDirectionWithdraw {
		name, catType, icon = repository.SavingsWithdrawCategoryName, "income", "savings_withdraw"
	}

	id, err := s.categoryRepo.EnsureSystemCategory(groupID, userID, name, catType, icon)
	if err != nil {
		if errors.Is(err, repository.ErrCategoryTypeMismatch) {
			return "", invalid("category %q already exists with a different type, rename it first", name)
		}
		return "", err
	}
	return id, nil
}

// ─── Nilai turunan ──────────────────────────────────────────────────

// enrichSavingsGoal mengisi field turunan dari SavedAmount + target. Dipisah
// dari repository karena bergantung pada "hari ini" — dengan begini bisa diuji
// tanpa database, cukup dengan now yang ditentukan.
func enrichSavingsGoal(item *repository.SavingsGoalWithProgress, now time.Time) {
	if item.TargetAmount == nil || *item.TargetAmount <= 0 {
		return // pot bebas tanpa target: tidak ada progres yang bisa dihitung
	}
	target := *item.TargetAmount

	progress := item.SavedAmount / target
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	item.Progress = &progress

	remaining := math.Max(0, target-item.SavedAmount)
	item.RemainingAmount = &remaining

	if item.TargetDate == nil {
		return // tanpa tenggat, tidak ada saran bulanan maupun on-track
	}

	months := monthsUntil(now, *item.TargetDate)
	item.MonthsLeft = &months

	// Bagi dengan minimal 1 bulan: kalau tenggat sudah lewat atau tinggal
	// bulan ini, sarannya adalah menyetor seluruh sisa sekarang.
	suggested := remaining / float64(max(1, months))
	item.SuggestedMonthly = &suggested

	// On-track: bandingkan yang sudah terkumpul dengan yang seharusnya
	// terkumpul pada titik waktu ini, lurus dari created_at ke target_date.
	total := item.TargetDate.Sub(item.CreatedAt)
	if total <= 0 {
		onTrack := item.SavedAmount >= target
		item.IsOnTrack = &onTrack
		return
	}
	ratio := float64(now.Sub(item.CreatedAt)) / float64(total)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	onTrack := item.SavedAmount >= target*ratio
	item.IsOnTrack = &onTrack
}

// monthsUntil menghitung sisa bulan penuh dari now ke target, minimal 0.
// Dihitung dari selisih kalender (tahun*12 + bulan), bukan dari jumlah hari,
// supaya "akhir Juni ke awal Juli" terbaca 1 bulan dan bukan 0.
func monthsUntil(now, target time.Time) int {
	months := (target.Year()-now.Year())*12 + int(target.Month()) - int(now.Month())
	if target.Day() < now.Day() {
		months-- // belum genap sebulan penuh di bulan terakhir
	}
	if months < 0 {
		return 0
	}
	return months
}

// ─── Helper ─────────────────────────────────────────────────────────

func isValidSavingsStatus(status string) bool {
	switch status {
	case repository.SavingsStatusActive,
		repository.SavingsStatusCompleted,
		repository.SavingsStatusArchived:
		return true
	}
	return false
}

// parseOptionalDate memperlakukan string kosong sebagai "tidak diisi" (nil),
// bukan sebagai error — target_date memang opsional.
func parseOptionalDate(value, field string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, invalid("invalid %s format, use YYYY-MM-DD", field)
	}
	return &parsed, nil
}

// parseEntryDate default ke hari ini bila tidak dikirim.
func parseEntryDate(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, invalid("invalid date format, use YYYY-MM-DD")
	}
	return parsed, nil
}

// translateSavingsErr mengubah sentinel error repository jadi pesan yang aman
// dikirim ke klien. Error lain diteruskan apa adanya untuk dijadikan 500 oleh
// handler.
func translateSavingsErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrSavingsGoalNotFound):
		return ErrSavingsNotFound
	case errors.Is(err, repository.ErrSavingsEntryNotFound):
		return ErrSavingsEntryNotFound
	case errors.Is(err, repository.ErrInsufficientSavings):
		return ErrSavingsInsufficient
	case errors.Is(err, repository.ErrSavingsGoalNotEmpty):
		return ErrSavingsNotEmpty
	}
	return err
}

// Sentinel error tingkat service. Handler memetakannya ke status HTTP —
// tanpa ini handler harus mencocokkan string pesan, yang rapuh.
var (
	ErrSavingsNotFound      = errors.New("savings goal not found")
	ErrSavingsEntryNotFound = errors.New("savings entry not found")
	ErrSavingsInsufficient  = errors.New("withdrawal exceeds savings balance")
	ErrSavingsNotEmpty      = errors.New("withdraw the remaining balance before deleting this savings goal")
)

// ValidationError menandai error yang berasal dari input user, bukan dari
// kegagalan sistem. Handler membedakannya lewat errors.As: yang ini dibalas
// 400 dengan pesannya apa adanya, sedangkan error lain dianggap tak terduga —
// di-log lengkap dan dibalas pesan generik supaya detail internal (nama tabel,
// pesan driver) tidak bocor ke klien.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}
