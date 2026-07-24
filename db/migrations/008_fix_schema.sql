-- 008_fix_schema.sql
-- Migration perbaikan untuk memastikan schema sesuai dengan kode terbaru.
-- Menambahkan kolom yang mungkin hilang jika database sudah ada sebelumnya.
-- Menambahkan index yang diperlukan.

-- ============================================================
-- 1. Pastikan tabel refresh_tokens ada
-- ============================================================
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_active ON refresh_tokens(user_id, revoked_at)
    WHERE revoked_at IS NULL;

-- ============================================================
-- 2. Pastikan kolom deleted_at ada di transactions
-- ============================================================
ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

-- ============================================================
-- 3. Pastikan index yang diperlukan ada
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_transactions_user_date ON transactions(user_id, transaction_date DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_transactions_category ON transactions(category_id);
CREATE INDEX IF NOT EXISTS idx_transactions_type ON transactions(type);
CREATE INDEX IF NOT EXISTS idx_transactions_deleted_at ON transactions(deleted_at);
CREATE INDEX IF NOT EXISTS idx_transactions_user_deleted ON transactions(user_id, deleted_at);

-- ============================================================
-- 4. Pastikan trigger updated_at ada di transactions
-- ============================================================
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_transactions_updated_at'
    ) THEN
        CREATE TRIGGER update_transactions_updated_at
            BEFORE UPDATE ON transactions
            FOR EACH ROW
            EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;
