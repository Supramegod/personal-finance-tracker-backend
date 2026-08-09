-- 013_create_savings.sql
-- Tabel: savings_goals + savings_entries, kolom transactions.is_transfer
-- Migrasi: UP
--
-- Modul tabungan. Tiap pot tabungan (savings_goals) punya nama, target nominal
-- opsional, dan target tanggal opsional. Tiap setoran/penarikan dicatat di
-- savings_entries DAN membuat satu transaksi di tabel transactions (atomik
-- lewat satu transaksi DB di repository) — persis pola 009_create_installments.
--
-- Bedanya dengan cicilan: transaksi tabungan adalah TRANSFER, bukan arus kas
-- operasional. Uangnya tidak hilang, hanya pindah tempat. Karena itu barisnya
-- ditandai is_transfer = true supaya:
--   - saldo kas tetap turun saat menabung (transaksi expense biasa),
--   - mutasinya tetap muncul di riwayat dan kalender,
--   - TAPI dikecualikan dari laporan pemasukan/pengeluaran, supaya bulan yang
--     rajin menabung tidak terlihat seperti bulan boros.
--
-- Saldo pot TIDAK disimpan sebagai kolom — dihitung dari SUM(setor) - SUM(tarik),
-- sama seperti paid_count pada cicilan. Satu sumber kebenaran, tidak bisa
-- melenceng dari mutasinya.
--
-- Pakai IF NOT EXISTS gaya 008/009/010 karena migrasi dijalankan otomatis saat
-- server start tanpa tabel versi migrasi — file ini jalan ulang setiap boot.

-- ============================================================
-- 1. savings_goals — pot / target tabungan
-- ============================================================
CREATE TABLE IF NOT EXISTS savings_goals (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id      UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,  -- pembuat
    name          VARCHAR(150) NOT NULL,
    -- NULL = celengan bebas tanpa target (mis. dana darurat). Memaksa user
    -- mengarang angka target hanya menghasilkan data sampah.
    target_amount DECIMAL(15,2) NULL CHECK (target_amount IS NULL OR target_amount > 0),
    target_date   DATE NULL,
    icon          VARCHAR(50) NULL,
    color         VARCHAR(20) NULL,
    status        VARCHAR(12) NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active', 'completed', 'archived')),
    note          TEXT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_savings_goals_group ON savings_goals(group_id);
CREATE INDEX IF NOT EXISTS idx_savings_goals_group_status
    ON savings_goals(group_id, status);

-- ============================================================
-- 2. savings_entries — satu baris per setoran / penarikan
-- ============================================================
CREATE TABLE IF NOT EXISTS savings_entries (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    goal_id        UUID NOT NULL REFERENCES savings_goals(id) ON DELETE CASCADE,
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,  -- yang menyetor
    -- Transaksi yang tercipta. SET NULL agar menghapus transaksi tidak
    -- menghancurkan riwayat tabungan — sama seperti installment_payments.
    transaction_id UUID NULL REFERENCES transactions(id) ON DELETE SET NULL,
    direction      VARCHAR(10) NOT NULL CHECK (direction IN ('deposit', 'withdraw')),
    amount         DECIMAL(15,2) NOT NULL CHECK (amount > 0),
    entry_date     DATE NOT NULL,
    note           TEXT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_savings_entries_goal
    ON savings_entries(goal_id, entry_date DESC);

-- ============================================================
-- 3. Penanda transfer di transactions
-- ============================================================
-- Default false: semua data lama tidak berubah artinya. Pembayaran cicilan
-- memang pengeluaran sungguhan, jadi tetap false — jangan di-backfill.
ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS is_transfer BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_transactions_group_transfer
    ON transactions(group_id, is_transfer) WHERE deleted_at IS NULL;

-- ============================================================
-- 4. Kategori sistem untuk sisi penarikan
-- ============================================================
-- Kategori "Tabungan" (expense) sudah ada di seed default. Pasangan income-nya
-- belum, dan group yang terlanjur dibuat sebelum migrasi ini juga belum
-- punya — backfill keduanya ke semua group yang sudah ada.
--
-- Idempotent lewat ON CONFLICT (group_id, name) — constraint
-- uq_categories_group_name dari migrasi 010.
INSERT INTO categories (group_id, user_id, name, type, icon, is_default)
SELECT g.id, g.owner_user_id, 'Tarik Tabungan', 'income', 'savings_withdraw', true
FROM groups g
ON CONFLICT (group_id, name) DO NOTHING;

INSERT INTO categories (group_id, user_id, name, type, icon, is_default)
SELECT g.id, g.owner_user_id, 'Tabungan', 'expense', 'savings', true
FROM groups g
ON CONFLICT (group_id, name) DO NOTHING;

-- DOWN:
-- DROP TABLE IF EXISTS savings_entries;
-- DROP TABLE IF EXISTS savings_goals;
-- DROP INDEX IF EXISTS idx_transactions_group_transfer;
-- ALTER TABLE transactions DROP COLUMN IF EXISTS is_transfer;
-- DELETE FROM categories WHERE name = 'Tarik Tabungan' AND is_default = true;
