package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SummaryRepository struct {
	pool *pgxpool.Pool
}

func NewSummaryRepository(pool *pgxpool.Pool) *SummaryRepository {
	return &SummaryRepository{pool: pool}
}

func (r *SummaryRepository) GetBalance(userID string) (float64, error) {
	var balance float64
	err := r.pool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE -amount END), 0)
		 FROM transactions WHERE user_id = $1 AND deleted_at IS NULL`, userID).Scan(&balance)
	return balance, err
}

type ReportRow struct {
	Period       time.Time `json:"period"`
	TotalIncome  float64   `json:"total_income"`
	TotalExpense float64   `json:"total_expense"`
}

func (r *SummaryRepository) GetReport(userID, period, from, to string) ([]ReportRow, float64, float64, error) {
	var dateTrunc string
	switch period {
	case "daily":
		dateTrunc = "day"
	case "weekly":
		dateTrunc = "week"
	case "monthly":
		dateTrunc = "month"
	default:
		dateTrunc = "month"
	}

	query := fmt.Sprintf(`
		SELECT DATE_TRUNC('%s', transaction_date)::date as period,
			   COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END), 0) as total_income,
			   COALESCE(SUM(CASE WHEN type = 'expense' THEN amount ELSE 0 END), 0) as total_expense
		FROM transactions
		WHERE user_id = $1
		  AND deleted_at IS NULL
		  AND transaction_date >= $2
		  AND transaction_date <= $3
		GROUP BY period
		ORDER BY period`, dateTrunc)

	rows, err := r.pool.Query(context.Background(), query, userID, from, to)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	// Inisialisasi slice kosong (bukan nil) agar hasil JSON tetap [] saat tidak ada baris
	report := []ReportRow{}
	var grandIncome, grandExpense float64

	for rows.Next() {
		var row ReportRow
		if err := rows.Scan(&row.Period, &row.TotalIncome, &row.TotalExpense); err != nil {
			return nil, 0, 0, err
		}
		report = append(report, row)
		grandIncome += row.TotalIncome
		grandExpense += row.TotalExpense
	}

	return report, grandIncome, grandExpense, nil
}
