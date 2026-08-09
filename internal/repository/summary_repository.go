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

// GetBalance mengembalikan saldo kas kelompok.
//
// Baris transfer (setoran/penarikan tabungan) SENGAJA ikut dihitung: menabung
// memang memindahkan uang keluar dari kas, jadi saldo kas harus turun. Yang
// dikecualikan hanya laporan pemasukan/pengeluaran di GetReport.
func (r *SummaryRepository) GetBalance(groupID string) (float64, error) {
	var balance float64
	err := r.pool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE -amount END), 0)
		 FROM transactions WHERE group_id = $1 AND deleted_at IS NULL`, groupID).Scan(&balance)
	return balance, err
}

type ReportRow struct {
	Period       time.Time `json:"period"`
	TotalIncome  float64   `json:"total_income"`
	TotalExpense float64   `json:"total_expense"`
	// TotalSavings adalah setoran dikurangi penarikan tabungan pada periode itu.
	// Bisa negatif kalau penarikan lebih besar dari setoran.
	TotalSavings float64 `json:"total_savings"`
}

// GetReport menyusun laporan per periode.
//
// Baris bertanda is_transfer dikeluarkan dari pemasukan/pengeluaran dan
// dijumlahkan terpisah sebagai total_savings. Tanpa pemisahan ini, bulan yang
// rajin menabung akan terbaca sebagai bulan boros — tabungan bukan beban,
// uangnya hanya pindah tempat.
func (r *SummaryRepository) GetReport(groupID, period, from, to string) ([]ReportRow, float64, float64, float64, error) {
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
			   COALESCE(SUM(CASE WHEN type = 'income'  AND NOT is_transfer THEN amount ELSE 0 END), 0) as total_income,
			   COALESCE(SUM(CASE WHEN type = 'expense' AND NOT is_transfer THEN amount ELSE 0 END), 0) as total_expense,
			   COALESCE(SUM(CASE WHEN is_transfer
			                     THEN (CASE WHEN type = 'expense' THEN amount ELSE -amount END)
			                     ELSE 0 END), 0) as total_savings
		FROM transactions
		WHERE group_id = $1
		  AND deleted_at IS NULL
		  AND transaction_date >= $2
		  AND transaction_date <= $3
		GROUP BY period
		ORDER BY period`, dateTrunc)

	rows, err := r.pool.Query(context.Background(), query, groupID, from, to)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rows.Close()

	// Inisialisasi slice kosong (bukan nil) agar hasil JSON tetap [] saat tidak ada baris
	report := []ReportRow{}
	var grandIncome, grandExpense, grandSavings float64

	for rows.Next() {
		var row ReportRow
		if err := rows.Scan(&row.Period, &row.TotalIncome, &row.TotalExpense, &row.TotalSavings); err != nil {
			return nil, 0, 0, 0, err
		}
		report = append(report, row)
		grandIncome += row.TotalIncome
		grandExpense += row.TotalExpense
		grandSavings += row.TotalSavings
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, 0, fmt.Errorf("iterate report rows: %w", err)
	}

	return report, grandIncome, grandExpense, grandSavings, nil
}
