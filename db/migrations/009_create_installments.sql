-- 009_create_installments.sql
-- Tabel: installments + installment_payments
-- Migrasi: UP
--
-- Modul cicilan bulanan. Tiap cicilan (installments) punya judul, nominal per
-- bulan, dan tenor (jumlah bulan). Tiap pembayaran bulanan dicatat di
-- installment_payments DAN membuat satu transaksi expense di tabel transactions
-- (dibuat atomik lewat satu transaksi DB di repository).
--
-- Pakai IF NOT EXISTS gaya 008_fix_schema.sql karena migrasi dijalankan otomatis
-- saat server start tanpa tabel versi migrasi.

-- ============================================================
-- 1. installments — rencana cicilan
-- ============================================================
CREATE TABLE IF NOT EXISTS installments (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id    UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    title          VARCHAR(150) NOT NULL,
    monthly_amount DECIMAL(15,2) NOT NULL CHECK (monthly_amount > 0),
    tenor_months   INT NOT NULL CHECK (tenor_months > 0),
    start_date     DATE NOT NULL,              -- bulan pertama jatuh tempo (anchor)
    note           TEXT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_installments_user ON installments(user_id);

-- ============================================================
-- 2. installment_payments — satu baris per bulan yang sudah dibayar
-- ============================================================
CREATE TABLE IF NOT EXISTS installment_payments (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    installment_id UUID NOT NULL REFERENCES installments(id) ON DELETE CASCADE,
    -- Transaksi expense yang tercipta. SET NULL agar menghapus transaksi tidak
    -- menghancurkan riwayat pembayaran cicilan.
    transaction_id UUID NULL REFERENCES transactions(id) ON DELETE SET NULL,
    period_index   INT NOT NULL CHECK (period_index > 0),  -- bulan ke-berapa (1..tenor)
    amount         DECIMAL(15,2) NOT NULL CHECK (amount > 0),
    paid_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Cegah membayar bulan (period_index) yang sama dua kali untuk satu cicilan.
    CONSTRAINT uq_installment_period UNIQUE (installment_id, period_index)
);

CREATE INDEX IF NOT EXISTS idx_installment_payments_installment
    ON installment_payments(installment_id);

-- DOWN:
-- DROP TABLE IF EXISTS installment_payments;
-- DROP TABLE IF EXISTS installments;
