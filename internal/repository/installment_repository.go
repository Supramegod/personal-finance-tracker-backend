package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InstallmentRepository struct {
	pool *pgxpool.Pool
}

func NewInstallmentRepository(pool *pgxpool.Pool) *InstallmentRepository {
	return &InstallmentRepository{pool: pool}
}

// Installment merepresentasikan satu rencana cicilan bulanan.
type Installment struct {
	ID            string    `json:"id"`
	GroupID       string    `json:"group_id"`
	UserID        string    `json:"user_id"`
	CategoryID    string    `json:"category_id"`
	Title         string    `json:"title"`
	MonthlyAmount float64   `json:"monthly_amount"`
	TenorMonths   int       `json:"tenor_months"`
	StartDate     time.Time `json:"start_date"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
}

// InstallmentWithProgress adalah Installment plus data turunan (dihitung, tidak
// disimpan sebagai kolom) untuk ditampilkan di daftar.
type InstallmentWithProgress struct {
	Installment
	CategoryName string  `json:"category_name"`
	PaidCount    int     `json:"paid_count"`
	TotalAmount  float64 `json:"total_amount"` // monthly_amount * tenor_months
	Paid         bool    `json:"paid"`         // lunas jika paid_count >= tenor_months
}

// InstallmentPayment adalah satu pembayaran bulanan yang sudah tercatat.
type InstallmentPayment struct {
	ID            string    `json:"id"`
	InstallmentID string    `json:"installment_id"`
	TransactionID *string   `json:"transaction_id"`
	PeriodIndex   int       `json:"period_index"`
	Amount        float64   `json:"amount"`
	PaidAt        time.Time `json:"paid_at"`
}

func (r *InstallmentRepository) Create(i *Installment) error {
	err := r.pool.QueryRow(context.Background(),
		`INSERT INTO installments (group_id, user_id, category_id, title, monthly_amount, tenor_months, start_date, note)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at`,
		i.GroupID, i.UserID, i.CategoryID, i.Title, i.MonthlyAmount, i.TenorMonths, i.StartDate, i.Note).Scan(
		&i.ID, &i.CreatedAt)
	if err != nil {
		return fmt.Errorf("create installment: %w", err)
	}
	return nil
}

// List mengembalikan semua cicilan milik kelompok beserta progres pembayarannya.
func (r *InstallmentRepository) List(groupID string) ([]InstallmentWithProgress, error) {
	rows, err := r.pool.Query(context.Background(),
		`SELECT i.id, i.group_id, i.user_id, i.category_id, i.title, i.monthly_amount,
		        i.tenor_months, i.start_date, COALESCE(i.note, ''), i.created_at,
		        COALESCE(c.name, '') AS category_name,
		        COUNT(p.id) AS paid_count
		 FROM installments i
		 LEFT JOIN categories c ON c.id = i.category_id
		 LEFT JOIN installment_payments p ON p.installment_id = i.id
		 WHERE i.group_id = $1
		 GROUP BY i.id, c.name
		 ORDER BY i.created_at DESC`, groupID)
	if err != nil {
		return nil, fmt.Errorf("query installments: %w", err)
	}
	defer rows.Close()

	items := []InstallmentWithProgress{}
	for rows.Next() {
		var it InstallmentWithProgress
		if err := rows.Scan(&it.ID, &it.GroupID, &it.UserID, &it.CategoryID, &it.Title, &it.MonthlyAmount,
			&it.TenorMonths, &it.StartDate, &it.Note, &it.CreatedAt,
			&it.CategoryName, &it.PaidCount); err != nil {
			return nil, fmt.Errorf("scan installment: %w", err)
		}
		it.TotalAmount = it.MonthlyAmount * float64(it.TenorMonths)
		it.Paid = it.PaidCount >= it.TenorMonths
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate installments: %w", err)
	}
	return items, nil
}

// FindByID mengambil satu cicilan (dengan pengecekan kepemilikan kelompok).
func (r *InstallmentRepository) FindByID(id, groupID string) (*Installment, error) {
	i := &Installment{}
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, group_id, user_id, category_id, title, monthly_amount, tenor_months,
		        start_date, COALESCE(note, ''), created_at
		 FROM installments WHERE id = $1 AND group_id = $2`, id, groupID).Scan(
		&i.ID, &i.GroupID, &i.UserID, &i.CategoryID, &i.Title, &i.MonthlyAmount, &i.TenorMonths,
		&i.StartDate, &i.Note, &i.CreatedAt)
	if err != nil {
		return nil, err
	}
	return i, nil
}

// CountPayments menghitung berapa bulan yang sudah dibayar untuk satu cicilan.
func (r *InstallmentRepository) CountPayments(installmentID string) (int, error) {
	var n int
	err := r.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM installment_payments WHERE installment_id = $1`, installmentID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count payments: %w", err)
	}
	return n, nil
}

// ListPayments mengembalikan riwayat pembayaran satu cicilan.
func (r *InstallmentRepository) ListPayments(installmentID string) ([]InstallmentPayment, error) {
	rows, err := r.pool.Query(context.Background(),
		`SELECT id, installment_id, transaction_id, period_index, amount, paid_at
		 FROM installment_payments WHERE installment_id = $1 ORDER BY period_index`, installmentID)
	if err != nil {
		return nil, fmt.Errorf("query payments: %w", err)
	}
	defer rows.Close()

	payments := []InstallmentPayment{}
	for rows.Next() {
		var p InstallmentPayment
		if err := rows.Scan(&p.ID, &p.InstallmentID, &p.TransactionID,
			&p.PeriodIndex, &p.Amount, &p.PaidAt); err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}
		payments = append(payments, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payments: %w", err)
	}
	return payments, nil
}

// PayParams membawa data pembayaran cicilan bulanan.
type PayParams struct {
	InstallmentID string
	GroupID       string
	UserID        string
	CategoryID    string
	Amount        float64
	PeriodIndex   int
	Date          time.Time
	Note          string
}

// Pay mencatat satu pembayaran cicilan secara ATOMIK: membuat transaksi expense
// dan baris installment_payments dalam satu transaksi DB. Kalau salah satu gagal,
// keduanya di-rollback — supaya tidak ada pembayaran tanpa transaksi (atau
// sebaliknya). Tidak memakai TransactionRepository.Create karena itu memakai
// pool sendiri dan tidak ikut dalam transaksi ini.
func (r *InstallmentRepository) Pay(p PayParams) (*InstallmentPayment, error) {
	ctx := context.Background()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) // no-op kalau sudah commit

	var txnID string
	err = tx.QueryRow(ctx,
		`INSERT INTO transactions (group_id, user_id, category_id, type, amount, transaction_date, note)
		 VALUES ($1, $2, $3, 'expense', $4, $5, $6)
		 RETURNING id`,
		p.GroupID, p.UserID, p.CategoryID, p.Amount, p.Date, p.Note).Scan(&txnID)
	if err != nil {
		return nil, fmt.Errorf("insert expense transaction: %w", err)
	}

	pay := &InstallmentPayment{}
	err = tx.QueryRow(ctx,
		`INSERT INTO installment_payments (installment_id, transaction_id, period_index, amount)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, installment_id, transaction_id, period_index, amount, paid_at`,
		p.InstallmentID, txnID, p.PeriodIndex, p.Amount).Scan(
		&pay.ID, &pay.InstallmentID, &pay.TransactionID, &pay.PeriodIndex, &pay.Amount, &pay.PaidAt)
	if err != nil {
		// Pelanggaran unique (installment_id, period_index) = bulan itu sudah dibayar.
		if isUniqueViolation(err) {
			return nil, ErrPeriodAlreadyPaid
		}
		return nil, fmt.Errorf("insert installment payment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return pay, nil
}

func (r *InstallmentRepository) Delete(id, groupID string) error {
	ct, err := r.pool.Exec(context.Background(),
		`DELETE FROM installments WHERE id = $1 AND group_id = $2`, id, groupID)
	if err != nil {
		return fmt.Errorf("delete installment: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrInstallmentNotFound
	}
	return nil
}

// ErrPeriodAlreadyPaid dikembalikan bila bulan tersebut sudah dibayar.
var ErrPeriodAlreadyPaid = errors.New("installment period already paid")

// ErrInstallmentNotFound dikembalikan bila cicilan tidak ada / bukan milik kelompok.
var ErrInstallmentNotFound = errors.New("installment not found")

// isUniqueViolation mendeteksi pelanggaran unique constraint PostgreSQL (23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
