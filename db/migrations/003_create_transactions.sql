-- 003_create_transactions.sql
-- Tabel: transactions
-- Migrasi: UP
-- Tabel inti transaksi. Semua query laporan dan saldo berasal dari sini.
-- Soft delete: deleted_at diisi saat delete, bukan hapus fisik baris.
-- Semua query WAJIB filter WHERE deleted_at IS NULL (kecuali untuk audit).

CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    type VARCHAR(10) NOT NULL CHECK (type IN ('income', 'expense')),
    amount DECIMAL(15,2) NOT NULL CHECK (amount > 0),
    transaction_date DATE NOT NULL,
    note TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL             -- soft delete: NULL = aktif, terisi = sudah dihapus
);

-- Index utama untuk query laporan & filter (P0) — mencakup deleted_at
CREATE INDEX idx_transactions_user_date ON transactions(user_id, transaction_date DESC)
    WHERE deleted_at IS NULL;

-- Index pendukung
CREATE INDEX idx_transactions_category ON transactions(category_id);
CREATE INDEX idx_transactions_type ON transactions(type);
CREATE INDEX idx_transactions_deleted_at ON transactions(deleted_at);

-- Composite index untuk query soft-delete + user
CREATE INDEX idx_transactions_user_deleted ON transactions(user_id, deleted_at);

-- Trigger untuk auto-update updated_at
CREATE TRIGGER update_transactions_updated_at
    BEFORE UPDATE ON transactions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- DOWN:
-- DROP TRIGGER IF EXISTS update_transactions_updated_at ON transactions;
-- DROP TABLE IF EXISTS transactions;
