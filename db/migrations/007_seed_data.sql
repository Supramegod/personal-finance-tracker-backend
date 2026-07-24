-- ============================================================
-- 007_seed_data.sql
-- Seed Data: User Admin + Kategori Default
-- Migrasi: UP
-- ============================================================
-- File ini menjalankan seeding data awal:
--   1. User admin untuk development
--   2. Kategori default (income + expense) untuk user admin
--
-- Prinsip:
--   - Idempotent: bisa dijalankan berulang tanpa duplikasi data
--   - Menggunakan pgcrypto crypt() untuk generate bcrypt hash di runtime
--   - Menggunakan DO block dengan PL/pgSQL agar UUID user dapat direferensi
-- ============================================================

DO $$
DECLARE
    v_user_id UUID;
    v_email CONSTANT VARCHAR(255) := 'admin@finance.local';
    v_password CONSTANT TEXT := 'admin123';
    v_full_name CONSTANT VARCHAR(255) := 'Admin Finance';
BEGIN
    -- ============================================================
    -- 1. Seed User Admin
    -- ============================================================
    -- Hanya insert jika email belum ada (idempotent)
    INSERT INTO users (email, password_hash, full_name)
    VALUES (
        v_email,
        crypt(v_password, gen_salt('bf', 12)),  -- bcrypt cost 12
        v_full_name
    )
    ON CONFLICT (email) DO UPDATE SET
        full_name = EXCLUDED.full_name,
        updated_at = NOW()
    RETURNING id INTO v_user_id;

    -- Jika user sudah ada dan RETURNING tidak menghasilkan baris (karena ON CONFLICT DO UPDATE
    -- tapi baris tidak berubah), ambil UUID yang sudah ada
    IF v_user_id IS NULL THEN
        SELECT id INTO v_user_id FROM users WHERE email = v_email;
    END IF;

    -- ============================================================
    -- 2. Seed Kategori Default — Income
    -- ============================================================
    INSERT INTO categories (user_id, name, type, icon, is_default) VALUES
        (v_user_id, 'Gaji',       'income',  'salary',          true),
        (v_user_id, 'Freelance',  'income',  'freelance',       true),
        (v_user_id, 'Bisnis',     'income',  'business',        true),
        (v_user_id, 'Investasi',  'income',  'investment',      true),
        (v_user_id, 'Hadiah',     'income',  'gift',            true),
        (v_user_id, 'Lainnya',    'income',  'other_income',    true)
    ON CONFLICT (user_id, name) DO NOTHING;

    -- ============================================================
    -- 3. Seed Kategori Default — Expense
    -- ============================================================
    INSERT INTO categories (user_id, name, type, icon, is_default) VALUES
        (v_user_id, 'Makan & Minum',    'expense', 'food',           true),
        (v_user_id, 'Transportasi',     'expense', 'transport',      true),
        (v_user_id, 'Belanja',          'expense', 'shopping',       true),
        (v_user_id, 'Hiburan',          'expense', 'entertainment',  true),
        (v_user_id, 'Kesehatan',        'expense', 'health',         true),
        (v_user_id, 'Pendidikan',       'expense', 'education',      true),
        (v_user_id, 'Tagihan',          'expense', 'bill',           true),
        (v_user_id, 'Tabungan',         'expense', 'savings',        true),
        (v_user_id, 'Lainnya',          'expense', 'other_expense',  true)
    ON CONFLICT (user_id, name) DO NOTHING;

END $$;

-- ============================================================
-- DOWN:
-- ============================================================
-- DELETE FROM categories WHERE is_default = true;
-- DELETE FROM users WHERE email = 'admin@finance.local';
-- ============================================================
