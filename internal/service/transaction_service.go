package service

import (
	"errors"
	"strings"
	"time"

	"personal-finance-tracker/internal/repository"
)

type TransactionService struct {
	repo         *repository.TransactionRepository
	categoryRepo *repository.CategoryRepository
}

func NewTransactionService(repo *repository.TransactionRepository, categoryRepo *repository.CategoryRepository) *TransactionService {
	return &TransactionService{repo: repo, categoryRepo: categoryRepo}
}

type ListTransactionsInput struct {
	UserID     string
	From       string
	To         string
	CategoryID string
	Type       string
	Page       int
	Limit      int
}

func (s *TransactionService) List(input ListTransactionsInput) ([]repository.Transaction, int, error) {
	if input.Type != "" {
		input.Type = strings.ToLower(input.Type)
		if input.Type != "income" && input.Type != "expense" {
			return nil, 0, errors.New("type must be 'income' or 'expense'")
		}
	}

	return s.repo.List(repository.ListTransactionsParams{
		UserID:     input.UserID,
		From:       input.From,
		To:         input.To,
		CategoryID: input.CategoryID,
		Type:       input.Type,
		Page:       input.Page,
		Limit:      input.Limit,
	})
}

type CreateTransactionInput struct {
	UserID          string
	CategoryID      string
	Type            string
	Amount          float64
	TransactionDate string
	Note            string
}

func (s *TransactionService) Create(input CreateTransactionInput) (*repository.Transaction, error) {
	// Validations
	input.Type = strings.ToLower(input.Type)
	if input.Type != "income" && input.Type != "expense" {
		return nil, errors.New("type must be 'income' or 'expense'")
	}

	if input.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}

	if input.CategoryID == "" {
		return nil, errors.New("category_id is required")
	}

	// Pastikan category_id valid dan milik user yang sedang login,
	// serta type-nya sesuai dengan type transaksi.
	category, err := s.categoryRepo.FindByIDAndUserID(input.CategoryID, input.UserID)
	if err != nil {
		return nil, errors.New("category not found")
	}
	if category.Type != input.Type {
		return nil, errors.New("category type does not match transaction type")
	}

	if input.TransactionDate == "" {
		return nil, errors.New("transaction_date is required")
	}

	parsedDate, err := time.Parse("2006-01-02", input.TransactionDate)
	if err != nil {
		return nil, errors.New("invalid transaction_date format, use YYYY-MM-DD")
	}

	t := &repository.Transaction{
		UserID:          input.UserID,
		CategoryID:      input.CategoryID,
		Type:            input.Type,
		Amount:          input.Amount,
		TransactionDate: parsedDate,
		Note:            input.Note,
	}

	if err := s.repo.Create(t); err != nil {
		return nil, err
	}

	return t, nil
}

func (s *TransactionService) GetByID(id, userID string) (*repository.Transaction, error) {
	return s.repo.FindByID(id, userID)
}

type UpdateTransactionInput struct {
	ID              string
	UserID          string
	CategoryID      string
	Type            string
	Amount          float64
	TransactionDate string
	Note            string
}

func (s *TransactionService) Update(input UpdateTransactionInput) (*repository.Transaction, error) {
	// Check ownership
	existing, err := s.repo.FindByID(input.ID, input.UserID)
	if err != nil {
		return nil, errors.New("transaction not found")
	}

	// Validate
	input.Type = strings.ToLower(input.Type)
	if input.Type != "income" && input.Type != "expense" {
		return nil, errors.New("type must be 'income' or 'expense'")
	}

	if input.Amount <= 0 {
		return nil, errors.New("amount must be greater than 0")
	}

	if input.CategoryID == "" {
		return nil, errors.New("category_id is required")
	}

	// Pastikan category_id valid dan milik user yang sedang login,
	// serta type-nya sesuai dengan type transaksi.
	category, err := s.categoryRepo.FindByIDAndUserID(input.CategoryID, input.UserID)
	if err != nil {
		return nil, errors.New("category not found")
	}
	if category.Type != input.Type {
		return nil, errors.New("category type does not match transaction type")
	}

	if input.TransactionDate == "" {
		return nil, errors.New("transaction_date is required")
	}

	parsedDate, err := time.Parse("2006-01-02", input.TransactionDate)
	if err != nil {
		return nil, errors.New("invalid transaction_date format, use YYYY-MM-DD")
	}

	existing.CategoryID = input.CategoryID
	existing.Type = input.Type
	existing.Amount = input.Amount
	existing.TransactionDate = parsedDate
	existing.Note = input.Note

	if err := s.repo.Update(existing); err != nil {
		return nil, err
	}

	return existing, nil
}

func (s *TransactionService) GetCalendarMonth(userID, month string) ([]repository.CalendarDay, error) {
	// Jika month kosong, default ke bulan berjalan
	if month == "" {
		now := time.Now()
		month = now.Format("2006-01")
	}
	// Validasi format month YYYY-MM
	if len(month) != 7 || month[4] != '-' {
		return nil, errors.New("invalid month format, use YYYY-MM")
	}

	return s.repo.GetCalendarMonth(userID, month)
}

func (s *TransactionService) Delete(id, userID string) error {
	return s.repo.Delete(id, userID)
}
