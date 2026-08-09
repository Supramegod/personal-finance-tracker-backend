package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Nama kategori sistem yang dipakai modul tabungan. Setoran tercatat sebagai
// expense berkategori "Tabungan", penarikan sebagai income berkategori
// "Tarik Tabungan". Keduanya di-seed sebagai kategori default per group dan
// di-backfill oleh migrasi 013.
const (
	SavingsDepositCategoryName  = "Tabungan"
	SavingsWithdrawCategoryName = "Tarik Tabungan"
)

// Arah mutasi tabungan.
const (
	SavingsDirectionDeposit  = "deposit"
	SavingsDirectionWithdraw = "withdraw"
)

// Status pot tabungan.
const (
	SavingsStatusActive    = "active"
	SavingsStatusCompleted = "completed"
	SavingsStatusArchived  = "archived"
)

var (
	// ErrSavingsGoalNotFound dikembalikan bila pot tidak ada / bukan milik kelompok.
	ErrSavingsGoalNotFound = errors.New("savings goal not found")
	// ErrInsufficientSavings dikembalikan bila penarikan melebihi saldo pot.
	ErrInsufficientSavings = errors.New("insufficient savings balance")
	// ErrSavingsGoalNotEmpty dikembalikan bila pot yang masih bersaldo dihapus.
	ErrSavingsGoalNotEmpty = errors.New("savings goal still has balance")
	// ErrSavingsEntryNotFound dikembalikan bila mutasi tidak ada / bukan milik pot.
	ErrSavingsEntryNotFound = errors.New("savings entry not found")
)

type SavingsRepository struct {
	pool *pgxpool.Pool
}

func NewSavingsRepository(pool *pgxpool.Pool) *SavingsRepository {
	return &SavingsRepository{pool: pool}
}

// SavingsGoal merepresentasikan satu pot tabungan.
// TargetAmount dan TargetDate nullable: pot tanpa target (mis. dana darurat)
// tetap sah.
type SavingsGoal struct {
	ID           string     `json:"id"`
	GroupID      string     `json:"group_id"`
	UserID       string     `json:"user_id"`
	Name         string     `json:"name"`
	TargetAmount *float64   `json:"target_amount"`
	TargetDate   *time.Time `json:"target_date"`
	Icon         string     `json:"icon"`
	Color        string     `json:"color"`
	Status       string     `json:"status"`
	Note         string     `json:"note"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// SavingsGoalWithProgress adalah SavingsGoal plus data turunan. SavedAmount
// dihitung di SQL; field sisanya diisi oleh service (butuh "hari ini", jadi
// dipisah agar bisa diuji tanpa DB). Pola sama dengan InstallmentWithProgress.
type SavingsGoalWithProgress struct {
	SavingsGoal
	SavedAmount      float64  `json:"saved_amount"`
	EntryCount       int      `json:"entry_count"`
	Progress         *float64 `json:"progress"`
	RemainingAmount  *float64 `json:"remaining_amount"`
	MonthsLeft       *int     `json:"months_left"`
	SuggestedMonthly *float64 `json:"suggested_monthly"`
	IsOnTrack        *bool    `json:"is_on_track"`
}

// SavingsEntry adalah satu mutasi tabungan (setoran atau penarikan).
type SavingsEntry struct {
	ID            string    `json:"id"`
	GoalID        string    `json:"goal_id"`
	UserID        string    `json:"user_id"`
	TransactionID *string   `json:"transaction_id"`
	Direction     string    `json:"direction"`
	Amount        float64   `json:"amount"`
	EntryDate     time.Time `json:"entry_date"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
}

// ─── Pot tabungan ───────────────────────────────────────────────────

func (r *SavingsRepository) Create(g *SavingsGoal) error {
	err := r.pool.QueryRow(context.Background(),
		`INSERT INTO savings_goals (group_id, user_id, name, target_amount, target_date, icon, color, note)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, status, created_at, updated_at`,
		g.GroupID, g.UserID, g.Name, g.TargetAmount, g.TargetDate, g.Icon, g.Color, g.Note).Scan(
		&g.ID, &g.Status, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create savings goal: %w", err)
	}
	return nil
}

// List mengembalikan pot milik kelompok beserta saldo terkumpulnya.
// status kosong = semua status.
func (r *SavingsRepository) List(groupID, status string) ([]SavingsGoalWithProgress, error) {
	// Subquery, bukan LEFT JOIN + GROUP BY: agregat setor-minus-tarik lebih
	// jelas dibaca dan tidak perlu menyeret seluruh kolom goal ke GROUP BY.
	rows, err := r.pool.Query(context.Background(),
		`SELECT g.id, g.group_id, g.user_id, g.name, g.target_amount, g.target_date,
		        COALESCE(g.icon, ''), COALESCE(g.color, ''), g.status, COALESCE(g.note, ''),
		        g.created_at, g.updated_at,
		        COALESCE(e.saved, 0)   AS saved_amount,
		        COALESCE(e.n, 0)       AS entry_count
		 FROM savings_goals g
		 LEFT JOIN (
		     SELECT goal_id,
		            SUM(CASE WHEN direction = 'deposit' THEN amount ELSE -amount END) AS saved,
		            COUNT(*) AS n
		     FROM savings_entries
		     GROUP BY goal_id
		 ) e ON e.goal_id = g.id
		 WHERE g.group_id = $1 AND ($2 = '' OR g.status = $2)
		 ORDER BY
		     CASE g.status WHEN 'active' THEN 0 WHEN 'completed' THEN 1 ELSE 2 END,
		     g.created_at DESC`, groupID, status)
	if err != nil {
		return nil, fmt.Errorf("query savings goals: %w", err)
	}
	defer rows.Close()

	items := []SavingsGoalWithProgress{}
	for rows.Next() {
		var it SavingsGoalWithProgress
		if err := rows.Scan(&it.ID, &it.GroupID, &it.UserID, &it.Name, &it.TargetAmount, &it.TargetDate,
			&it.Icon, &it.Color, &it.Status, &it.Note, &it.CreatedAt, &it.UpdatedAt,
			&it.SavedAmount, &it.EntryCount); err != nil {
			return nil, fmt.Errorf("scan savings goal: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate savings goals: %w", err)
	}
	return items, nil
}

// FindByID mengambil satu pot dengan pengecekan kepemilikan kelompok.
func (r *SavingsRepository) FindByID(id, groupID string) (*SavingsGoal, error) {
	g := &SavingsGoal{}
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, group_id, user_id, name, target_amount, target_date,
		        COALESCE(icon, ''), COALESCE(color, ''), status, COALESCE(note, ''),
		        created_at, updated_at
		 FROM savings_goals WHERE id = $1 AND group_id = $2`, id, groupID).Scan(
		&g.ID, &g.GroupID, &g.UserID, &g.Name, &g.TargetAmount, &g.TargetDate,
		&g.Icon, &g.Color, &g.Status, &g.Note, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSavingsGoalNotFound
		}
		return nil, fmt.Errorf("find savings goal: %w", err)
	}
	return g, nil
}

// SavedAmount mengembalikan saldo pot: SUM(setor) - SUM(tarik).
func (r *SavingsRepository) SavedAmount(goalID string) (float64, error) {
	var saved float64
	err := r.pool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(CASE WHEN direction = 'deposit' THEN amount ELSE -amount END), 0)
		 FROM savings_entries WHERE goal_id = $1`, goalID).Scan(&saved)
	if err != nil {
		return 0, fmt.Errorf("sum savings entries: %w", err)
	}
	return saved, nil
}

// TotalSaved mengembalikan total seluruh tabungan satu kelompok. Pot berstatus
// archived tetap dihitung — uangnya nyata, statusnya hanya soal tampilan.
func (r *SavingsRepository) TotalSaved(groupID string) (float64, error) {
	var total float64
	err := r.pool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(CASE WHEN e.direction = 'deposit' THEN e.amount ELSE -e.amount END), 0)
		 FROM savings_entries e
		 JOIN savings_goals g ON g.id = e.goal_id
		 WHERE g.group_id = $1`, groupID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("sum group savings: %w", err)
	}
	return total, nil
}

// Update mengubah atribut pot. Saldo tidak ikut diubah — itu turunan mutasi.
func (r *SavingsRepository) Update(g *SavingsGoal) error {
	ct, err := r.pool.Exec(context.Background(),
		`UPDATE savings_goals
		 SET name = $1, target_amount = $2, target_date = $3, icon = $4,
		     color = $5, note = $6, status = $7, updated_at = NOW()
		 WHERE id = $8 AND group_id = $9`,
		g.Name, g.TargetAmount, g.TargetDate, g.Icon, g.Color, g.Note, g.Status, g.ID, g.GroupID)
	if err != nil {
		return fmt.Errorf("update savings goal: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrSavingsGoalNotFound
	}
	return nil
}

// Delete menghapus pot. Menolak bila saldo masih ada — kalau dibiarkan, uang
// itu lenyap dari pembukuan tanpa jejak penarikan. Pengecekan saldo dan
// penghapusan dijalankan dalam satu transaksi dengan baris pot terkunci,
// supaya tidak ada setoran menyelip di antara keduanya.
func (r *SavingsRepository) Delete(id, groupID string) error {
	ctx := context.Background()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op kalau sudah commit

	if _, err := lockGoal(ctx, tx, id, groupID); err != nil {
		return err
	}

	saved, err := savedAmountTx(ctx, tx, id)
	if err != nil {
		return err
	}
	// Toleransi pembulatan DECIMAL(15,2) → float64.
	if saved > 0.005 {
		return ErrSavingsGoalNotEmpty
	}

	if _, err := tx.Exec(ctx, `DELETE FROM savings_goals WHERE id = $1 AND group_id = $2`, id, groupID); err != nil {
		return fmt.Errorf("delete savings goal: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// ─── Mutasi ─────────────────────────────────────────────────────────

// AddEntryParams membawa data satu setoran/penarikan.
type AddEntryParams struct {
	GoalID     string
	GroupID    string
	UserID     string
	CategoryID string
	Direction  string // deposit | withdraw
	Amount     float64
	Date       time.Time
	Note       string
}

// AddEntry mencatat satu mutasi tabungan secara ATOMIK: membuat transaksi
// (is_transfer = true) + baris savings_entries + menyesuaikan status pot, semua
// dalam satu transaksi DB. Kalau salah satu gagal, semuanya di-rollback —
// supaya tidak pernah ada mutasi tanpa transaksi, atau sebaliknya.
//
// Baris pot dikunci FOR UPDATE sebelum saldo dibaca. Tanpa kunci itu, dua
// penarikan bersamaan bisa sama-sama lolos pengecekan saldo dan membuat saldo
// pot negatif. Pola dasarnya sama dengan InstallmentRepository.Pay, tapi
// pengecekan invariant di sini harus ikut di dalam transaksi.
func (r *SavingsRepository) AddEntry(p AddEntryParams) (*SavingsEntry, error) {
	ctx := context.Background()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op kalau sudah commit

	target, err := lockGoal(ctx, tx, p.GoalID, p.GroupID)
	if err != nil {
		return nil, err
	}

	saved, err := savedAmountTx(ctx, tx, p.GoalID)
	if err != nil {
		return nil, err
	}

	// Setoran = uang keluar dari kas (expense). Penarikan = uang kembali ke
	// kas (income). Keduanya transfer: tidak menambah/mengurangi kekayaan.
	txnType := "expense"
	delta := p.Amount
	if p.Direction == SavingsDirectionWithdraw {
		txnType = "income"
		delta = -p.Amount
		if p.Amount > saved+0.005 {
			return nil, ErrInsufficientSavings
		}
	}

	var txnID string
	err = tx.QueryRow(ctx,
		`INSERT INTO transactions (group_id, user_id, category_id, type, amount, transaction_date, note, is_transfer)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, true)
		 RETURNING id`,
		p.GroupID, p.UserID, p.CategoryID, txnType, p.Amount, p.Date, p.Note).Scan(&txnID)
	if err != nil {
		return nil, fmt.Errorf("insert savings transaction: %w", err)
	}

	entry := &SavingsEntry{}
	err = tx.QueryRow(ctx,
		`INSERT INTO savings_entries (goal_id, user_id, transaction_id, direction, amount, entry_date, note)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, goal_id, user_id, transaction_id, direction, amount, entry_date, COALESCE(note, ''), created_at`,
		p.GoalID, p.UserID, txnID, p.Direction, p.Amount, p.Date, p.Note).Scan(
		&entry.ID, &entry.GoalID, &entry.UserID, &entry.TransactionID, &entry.Direction,
		&entry.Amount, &entry.EntryDate, &entry.Note, &entry.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert savings entry: %w", err)
	}

	if err := syncStatusTx(ctx, tx, p.GoalID, saved+delta, target); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return entry, nil
}

// ListEntries mengembalikan riwayat mutasi satu pot, terbaru dulu.
func (r *SavingsRepository) ListEntries(goalID string) ([]SavingsEntry, error) {
	rows, err := r.pool.Query(context.Background(),
		`SELECT id, goal_id, user_id, transaction_id, direction, amount, entry_date,
		        COALESCE(note, ''), created_at
		 FROM savings_entries WHERE goal_id = $1
		 ORDER BY entry_date DESC, created_at DESC`, goalID)
	if err != nil {
		return nil, fmt.Errorf("query savings entries: %w", err)
	}
	defer rows.Close()

	entries := []SavingsEntry{}
	for rows.Next() {
		var e SavingsEntry
		if err := rows.Scan(&e.ID, &e.GoalID, &e.UserID, &e.TransactionID, &e.Direction,
			&e.Amount, &e.EntryDate, &e.Note, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan savings entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate savings entries: %w", err)
	}
	return entries, nil
}

// DeleteEntry membatalkan satu mutasi: menghapus barisnya DAN men-soft-delete
// transaksi yang tercipta bersamanya, atomik. Ini satu-satunya jalan yang benar
// untuk membatalkan setoran — menghapus transaksinya langsung dari halaman
// Transaksi ditolak (lihat TransactionRepository.Delete), karena akan membuat
// saldo pot melenceng dari ledger.
//
// Menghapus setoran menurunkan saldo; kalau sudah ada penarikan setelahnya,
// saldo bisa jatuh di bawah nol — itu ditolak.
func (r *SavingsRepository) DeleteEntry(entryID, goalID, groupID string) error {
	ctx := context.Background()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op kalau sudah commit

	target, err := lockGoal(ctx, tx, goalID, groupID)
	if err != nil {
		return err
	}

	var direction string
	var amount float64
	var txnID *string
	err = tx.QueryRow(ctx,
		`SELECT direction, amount, transaction_id FROM savings_entries
		 WHERE id = $1 AND goal_id = $2`, entryID, goalID).Scan(&direction, &amount, &txnID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSavingsEntryNotFound
		}
		return fmt.Errorf("find savings entry: %w", err)
	}

	saved, err := savedAmountTx(ctx, tx, goalID)
	if err != nil {
		return err
	}

	// Membatalkan setoran mengurangi saldo, membatalkan penarikan menambahnya.
	newSaved := saved - amount
	if direction == SavingsDirectionWithdraw {
		newSaved = saved + amount
	}
	if newSaved < -0.005 {
		return ErrInsufficientSavings
	}

	if _, err := tx.Exec(ctx, `DELETE FROM savings_entries WHERE id = $1`, entryID); err != nil {
		return fmt.Errorf("delete savings entry: %w", err)
	}

	if txnID != nil {
		// Soft delete, konsisten dengan cara transaksi dihapus di mana pun.
		if _, err := tx.Exec(ctx,
			`UPDATE transactions SET deleted_at = NOW()
			 WHERE id = $1 AND deleted_at IS NULL`, *txnID); err != nil {
			return fmt.Errorf("soft delete savings transaction: %w", err)
		}
	}

	if err := syncStatusTx(ctx, tx, goalID, newSaved, target); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// ─── Helper internal (dipakai di dalam transaksi DB) ────────────────

// lockGoal mengunci baris pot dan mengembalikan target_amount-nya. Kunci ini
// yang menyerialkan mutasi bersamaan pada satu pot.
func lockGoal(ctx context.Context, tx pgx.Tx, goalID, groupID string) (*float64, error) {
	var target *float64
	err := tx.QueryRow(ctx,
		`SELECT target_amount FROM savings_goals
		 WHERE id = $1 AND group_id = $2 FOR UPDATE`, goalID, groupID).Scan(&target)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSavingsGoalNotFound
		}
		return nil, fmt.Errorf("lock savings goal: %w", err)
	}
	return target, nil
}

func savedAmountTx(ctx context.Context, tx pgx.Tx, goalID string) (float64, error) {
	var saved float64
	err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(CASE WHEN direction = 'deposit' THEN amount ELSE -amount END), 0)
		 FROM savings_entries WHERE goal_id = $1`, goalID).Scan(&saved)
	if err != nil {
		return 0, fmt.Errorf("sum savings entries: %w", err)
	}
	return saved, nil
}

// syncStatusTx menyesuaikan status pot dengan saldo barunya: 'completed' saat
// target tercapai, kembali 'active' saat turun di bawah target lagi. Pot
// 'archived' tidak pernah disentuh — itu keputusan eksplisit user.
func syncStatusTx(ctx context.Context, tx pgx.Tx, goalID string, newSaved float64, target *float64) error {
	next := SavingsStatusActive
	if target != nil && newSaved >= *target-0.005 {
		next = SavingsStatusCompleted
	}
	_, err := tx.Exec(ctx,
		`UPDATE savings_goals SET status = $1, updated_at = NOW()
		 WHERE id = $2 AND status <> 'archived' AND status <> $1`, next, goalID)
	if err != nil {
		return fmt.Errorf("sync savings status: %w", err)
	}
	return nil
}
