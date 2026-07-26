package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"personal-finance-tracker/internal/repository"
)

type InstallmentService struct {
	repo         *repository.InstallmentRepository
	categoryRepo *repository.CategoryRepository
}

func NewInstallmentService(
	repo *repository.InstallmentRepository,
	categoryRepo *repository.CategoryRepository,
) *InstallmentService {
	return &InstallmentService{repo: repo, categoryRepo: categoryRepo}
}

type CreateInstallmentInput struct {
	GroupID       string
	UserID        string
	CategoryID    string
	Title         string
	MonthlyAmount float64
	TenorMonths   int
	StartDate     string
	Note          string
}

func (s *InstallmentService) Create(input CreateInstallmentInput) (*repository.Installment, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return nil, errors.New("title is required")
	}
	if input.MonthlyAmount <= 0 {
		return nil, errors.New("monthly_amount must be greater than 0")
	}
	if input.TenorMonths <= 0 {
		return nil, errors.New("tenor_months must be greater than 0")
	}
	if input.CategoryID == "" {
		return nil, errors.New("category_id is required")
	}

	// Pastikan kategori valid, milik kelompok, dan bertipe expense — cicilan selalu
	// tercatat sebagai pengeluaran. Pola sama dengan TransactionService.Create.
	category, err := s.categoryRepo.FindByIDAndGroupID(input.CategoryID, input.GroupID)
	if err != nil {
		return nil, errors.New("category not found")
	}
	if category.Type != "expense" {
		return nil, errors.New("category must be an expense category")
	}

	if input.StartDate == "" {
		return nil, errors.New("start_date is required")
	}
	startDate, err := time.Parse("2006-01-02", input.StartDate)
	if err != nil {
		return nil, errors.New("invalid start_date format, use YYYY-MM-DD")
	}

	inst := &repository.Installment{
		GroupID:       input.GroupID,
		UserID:        input.UserID,
		CategoryID:    input.CategoryID,
		Title:         input.Title,
		MonthlyAmount: input.MonthlyAmount,
		TenorMonths:   input.TenorMonths,
		StartDate:     startDate,
		Note:          input.Note,
	}
	if err := s.repo.Create(inst); err != nil {
		return nil, err
	}
	return inst, nil
}

func (s *InstallmentService) List(groupID string) ([]repository.InstallmentWithProgress, error) {
	return s.repo.List(groupID)
}

func (s *InstallmentService) GetByID(id, groupID string) (*repository.Installment, error) {
	return s.repo.FindByID(id, groupID)
}

func (s *InstallmentService) ListPayments(id, groupID string) ([]repository.InstallmentPayment, error) {
	// Pastikan cicilan milik kelompok sebelum mengembalikan pembayarannya.
	if _, err := s.repo.FindByID(id, groupID); err != nil {
		return nil, errors.New("installment not found")
	}
	return s.repo.ListPayments(id)
}

// Pay mencatat pembayaran cicilan bulan berikutnya: membuat transaksi expense +
// baris pembayaran (atomik di repository). Ditolak bila cicilan sudah lunas.
func (s *InstallmentService) Pay(id, groupID, userID string) (*repository.InstallmentPayment, error) {
	inst, err := s.repo.FindByID(id, groupID)
	if err != nil {
		return nil, errors.New("installment not found")
	}

	paidCount, err := s.repo.CountPayments(inst.ID)
	if err != nil {
		return nil, err
	}
	if paidCount >= inst.TenorMonths {
		return nil, errors.New("installment already fully paid")
	}

	periodIndex := paidCount + 1
	note := fmt.Sprintf("Cicilan: %s (bln %d/%d)", inst.Title, periodIndex, inst.TenorMonths)

	pay, err := s.repo.Pay(repository.PayParams{
		InstallmentID: inst.ID,
		GroupID:       groupID,
		UserID:        userID,
		CategoryID:    inst.CategoryID,
		Amount:        inst.MonthlyAmount,
		PeriodIndex:   periodIndex,
		Date:          time.Now(),
		Note:          note,
	})
	if err != nil {
		if errors.Is(err, repository.ErrPeriodAlreadyPaid) {
			return nil, errors.New("this month's installment is already paid")
		}
		return nil, err
	}
	return pay, nil
}

func (s *InstallmentService) Delete(id, groupID string) error {
	if err := s.repo.Delete(id, groupID); err != nil {
		if errors.Is(err, repository.ErrInstallmentNotFound) {
			return errors.New("installment not found")
		}
		return err
	}
	return nil
}
