-- 004_create_budgets.sql
-- Tabel: budgets (P1)
-- Migrasi: UP
-- Anggaran per kategori per periode. Implementasi ditunda ke P1.
-- Skema sudah siap agar tidak perlu migrasi besar saat fitur ini dikerjakan.
-- Catatan: start_date adalah anchor periode (bukan asumsi "tanggal 1").
--   Contoh: start_date = '2026-06-15', period = 'monthly' → periode 15 Juni - 14 Juli.

CREATE TABLE budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    period VARCHAR(10) NOT NULL CHECK (period IN ('weekly', 'monthly')),
    start_date DATE NOT NULL,               -- anchor periode budget
    limit_amount DECIMAL(15,2) NOT NULL CHECK (limit_amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_budgets_user_category ON budgets(user_id, category_id);
CREATE INDEX idx_budgets_period_start ON budgets(period, start_date);

-- DOWN:
-- DROP TABLE IF EXISTS budgets;
