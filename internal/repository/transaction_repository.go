package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TransactionRepository struct {
	pool *pgxpool.Pool
}

func NewTransactionRepository(pool *pgxpool.Pool) *TransactionRepository {
	return &TransactionRepository{pool: pool}
}

type ListTransactionsParams struct {
	UserID     string
	From       string
	To         string
	CategoryID string
	Type       string
	Page       int
	Limit      int
}

func (r *TransactionRepository) List(params ListTransactionsParams) ([]Transaction, int, error) {
	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Page <= 0 {
		params.Page = 1
	}

	where := []string{"t.user_id = $1", "t.deleted_at IS NULL"}
	args := []interface{}{params.UserID}
	argIdx := 2

	if params.From != "" {
		where = append(where, fmt.Sprintf("t.transaction_date >= $%d", argIdx))
		args = append(args, params.From)
		argIdx++
	}
	if params.To != "" {
		where = append(where, fmt.Sprintf("t.transaction_date <= $%d", argIdx))
		args = append(args, params.To)
		argIdx++
	}
	if params.CategoryID != "" {
		where = append(where, fmt.Sprintf("t.category_id = $%d", argIdx))
		args = append(args, params.CategoryID)
		argIdx++
	}
	if params.Type != "" {
		where = append(where, fmt.Sprintf("t.type = $%d", argIdx))
		args = append(args, params.Type)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	// Count total
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM transactions t WHERE %s", whereClause)
	if err := r.pool.QueryRow(context.Background(), countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get data
	offset := (params.Page - 1) * params.Limit
	dataQuery := fmt.Sprintf(`
		SELECT t.id, t.user_id, t.category_id, t.type, t.amount,
			   t.transaction_date, t.note, t.created_at, t.updated_at,
			   COALESCE(c.name, '') as category_name
		FROM transactions t
		LEFT JOIN categories c ON t.category_id = c.id
		WHERE %s
		ORDER BY t.transaction_date DESC, t.created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)

	args = append(args, params.Limit, offset)

	rows, err := r.pool.Query(context.Background(), dataQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var transactions []Transaction
	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.UserID, &t.CategoryID, &t.Type, &t.Amount,
			&t.TransactionDate, &t.Note, &t.CreatedAt, &t.UpdatedAt, &t.CategoryName); err != nil {
			return nil, 0, err
		}
		transactions = append(transactions, t)
	}

	return transactions, total, nil
}

func (r *TransactionRepository) Create(t *Transaction) error {
	err := r.pool.QueryRow(context.Background(),
		`INSERT INTO transactions (user_id, category_id, type, amount, transaction_date, note)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at, updated_at`,
		t.UserID, t.CategoryID, t.Type, t.Amount, t.TransactionDate, t.Note).Scan(
		&t.ID, &t.CreatedAt, &t.UpdatedAt)
	return err
}

func (r *TransactionRepository) FindByID(id, userID string) (*Transaction, error) {
	t := &Transaction{}
	err := r.pool.QueryRow(context.Background(),
		`SELECT t.id, t.user_id, t.category_id, t.type, t.amount,
				t.transaction_date, t.note, t.created_at, t.updated_at,
				COALESCE(c.name, '') as category_name
		 FROM transactions t
		 LEFT JOIN categories c ON t.category_id = c.id
		 WHERE t.id = $1 AND t.user_id = $2 AND t.deleted_at IS NULL`, id, userID).Scan(
		&t.ID, &t.UserID, &t.CategoryID, &t.Type, &t.Amount,
		&t.TransactionDate, &t.Note, &t.CreatedAt, &t.UpdatedAt, &t.CategoryName)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (r *TransactionRepository) Update(t *Transaction) error {
	_, err := r.pool.Exec(context.Background(),
		`UPDATE transactions
		 SET category_id = $1, type = $2, amount = $3, transaction_date = $4, note = $5
		 WHERE id = $6 AND user_id = $7`,
		t.CategoryID, t.Type, t.Amount, t.TransactionDate, t.Note, t.ID, t.UserID)
	return err
}

func (r *TransactionRepository) Delete(id, userID string) error {
	_, err := r.pool.Exec(context.Background(),
		`UPDATE transactions SET deleted_at = NOW() WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID)
	return err
}

type CalendarDay struct {
	Date         string  `json:"date"`
	TotalIncome  float64 `json:"total_income"`
	TotalExpense float64 `json:"total_expense"`
	Count        int     `json:"count"`
}

func (r *TransactionRepository) GetCalendarMonth(userID, month string) ([]CalendarDay, error) {
	// Parse month "2026-06" → firstDate "2026-06-01", lastDate "2026-06-30"
	firstDate := month + "-01"
	t, err := time.Parse("2006-01-02", firstDate)
	if err != nil {
		return nil, fmt.Errorf("invalid month format: %s", month)
	}
	lastDate := t.AddDate(0, 1, -1).Format("2006-01-02")

	query := `
		SELECT
			transaction_date,
			COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END), 0) as total_income,
			COALESCE(SUM(CASE WHEN type = 'expense' THEN amount ELSE 0 END), 0) as total_expense,
			COUNT(*) as count
		FROM transactions
		WHERE user_id = $1
			AND deleted_at IS NULL
			AND transaction_date >= $2
			AND transaction_date <= $3
		GROUP BY transaction_date
		ORDER BY transaction_date DESC
	`

	rows, err := r.pool.Query(context.Background(), query, userID, firstDate, lastDate)
	if err != nil {
		return nil, fmt.Errorf("query calendar month %s: %w", month, err)
	}
	defer rows.Close()

	var days []CalendarDay
	for rows.Next() {
		var d CalendarDay
		// transaction_date bertipe DATE — scan ke time.Time, lalu format
		// ke "YYYY-MM-DD" karena klien mencocokkan string tanggal apa adanya.
		var txDate time.Time
		if err := rows.Scan(&txDate, &d.TotalIncome, &d.TotalExpense, &d.Count); err != nil {
			return nil, fmt.Errorf("scan calendar row: %w", err)
		}
		d.Date = txDate.Format("2006-01-02")
		days = append(days, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calendar rows: %w", err)
	}

	return days, nil
}

type Transaction struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	CategoryID      string     `json:"category_id"`
	Type            string     `json:"type"`
	Amount          float64    `json:"amount"`
	TransactionDate time.Time  `json:"transaction_date"`
	Note            string     `json:"note"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	CategoryName    string     `json:"category_name"`
}
