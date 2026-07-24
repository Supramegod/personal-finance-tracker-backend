-- 002_create_categories.sql
-- Tabel: categories
-- Migrasi: UP
-- Kategori transaksi. Memiliki data seed default.

CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(10) NOT NULL CHECK (type IN ('income', 'expense')),
    icon VARCHAR(50) NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_categories_user_id ON categories(user_id);

-- Unique constraint: satu user tidak boleh punya 2 kategori dengan nama sama
ALTER TABLE categories ADD CONSTRAINT uq_categories_user_name UNIQUE (user_id, name);

-- DOWN:
-- DROP TABLE IF EXISTS categories;
