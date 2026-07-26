-- 010_create_groups.sql
-- Multi-user berbasis kelompok (groups).
-- Migrasi: UP
--
-- Kepemilikan data finansial bergeser dari user -> kelompok. user_id tetap ada
-- di tabel data (arti: "dibuat oleh"), group_id baru jadi kunci scoping/isolasi.
-- Semua query data memfilter group_id, bukan lagi user_id.
--
-- Pakai IF NOT EXISTS + backfill idempotent (WHERE group_id IS NULL) karena
-- migrasi jalan otomatis saat start tanpa tabel versi migrasi.

-- ============================================================
-- 1. Tabel groups
-- ============================================================
CREATE TABLE IF NOT EXISTS groups (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          VARCHAR(150) NOT NULL,
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_groups_owner ON groups(owner_user_id);

-- ============================================================
-- 2. Tabel group_members (user <-> group, dengan role)
-- ============================================================
CREATE TABLE IF NOT EXISTS group_members (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       VARCHAR(10) NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_group_member UNIQUE (group_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_group_members_user ON group_members(user_id);
CREATE INDEX IF NOT EXISTS idx_group_members_group ON group_members(group_id);

-- ============================================================
-- 3. Tambah kolom group_id ke tabel data finansial (nullable dulu)
-- ============================================================
ALTER TABLE categories   ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES groups(id) ON DELETE CASCADE;
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES groups(id) ON DELETE CASCADE;
ALTER TABLE budgets      ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES groups(id) ON DELETE CASCADE;
ALTER TABLE installments ADD COLUMN IF NOT EXISTS group_id UUID REFERENCES groups(id) ON DELETE CASCADE;

-- ============================================================
-- 4. Backfill: buat group default untuk tiap user yang punya data,
--    jadikan mereka owner, lalu isi group_id di semua data lama.
--    Idempotent: hanya jalan untuk baris yang group_id-nya masih NULL.
-- ============================================================
DO $$
DECLARE
    u RECORD;
    v_group_id UUID;
BEGIN
    -- Semua user yang punya kepemilikan data lama (union dari 4 tabel).
    FOR u IN
        SELECT DISTINCT owner_id AS user_id FROM (
            SELECT user_id AS owner_id FROM categories   WHERE group_id IS NULL
            UNION SELECT user_id FROM transactions WHERE group_id IS NULL
            UNION SELECT user_id FROM budgets      WHERE group_id IS NULL
            UNION SELECT user_id FROM installments WHERE group_id IS NULL
        ) src
    LOOP
        -- Pakai group milik user ini kalau sudah ada; kalau belum, buat.
        SELECT id INTO v_group_id FROM groups WHERE owner_user_id = u.user_id LIMIT 1;

        IF v_group_id IS NULL THEN
            INSERT INTO groups (name, owner_user_id)
            SELECT COALESCE(NULLIF(full_name, ''), 'Keluarga'), u.user_id
            FROM users WHERE id = u.user_id
            RETURNING id INTO v_group_id;

            INSERT INTO group_members (group_id, user_id, role)
            VALUES (v_group_id, u.user_id, 'owner')
            ON CONFLICT (group_id, user_id) DO NOTHING;
        END IF;

        UPDATE categories   SET group_id = v_group_id WHERE user_id = u.user_id AND group_id IS NULL;
        UPDATE transactions SET group_id = v_group_id WHERE user_id = u.user_id AND group_id IS NULL;
        UPDATE budgets      SET group_id = v_group_id WHERE user_id = u.user_id AND group_id IS NULL;
        UPDATE installments SET group_id = v_group_id WHERE user_id = u.user_id AND group_id IS NULL;
    END LOOP;
END $$;

-- ============================================================
-- 5. Jadikan group_id NOT NULL setelah backfill.
--    Aman diulang: kalau sudah NOT NULL, ALTER ini no-op secara efektif.
-- ============================================================
DO $$
BEGIN
    -- Hanya set NOT NULL bila tidak ada baris yang masih NULL (menghindari error
    -- kalau ada data tanpa user/group — seharusnya tidak terjadi setelah backfill).
    IF NOT EXISTS (SELECT 1 FROM categories WHERE group_id IS NULL) THEN
        ALTER TABLE categories ALTER COLUMN group_id SET NOT NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM transactions WHERE group_id IS NULL) THEN
        ALTER TABLE transactions ALTER COLUMN group_id SET NOT NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM budgets WHERE group_id IS NULL) THEN
        ALTER TABLE budgets ALTER COLUMN group_id SET NOT NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM installments WHERE group_id IS NULL) THEN
        ALTER TABLE installments ALTER COLUMN group_id SET NOT NULL;
    END IF;
END $$;

-- ============================================================
-- 6. Index scoping + ganti unik kategori ke (group_id, name)
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_categories_group   ON categories(group_id);
CREATE INDEX IF NOT EXISTS idx_transactions_group ON transactions(group_id);
CREATE INDEX IF NOT EXISTS idx_installments_group ON installments(group_id);

ALTER TABLE categories DROP CONSTRAINT IF EXISTS uq_categories_user_name;
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_categories_group_name'
    ) THEN
        ALTER TABLE categories ADD CONSTRAINT uq_categories_group_name UNIQUE (group_id, name);
    END IF;
END $$;

-- DOWN:
-- ALTER TABLE categories DROP CONSTRAINT IF EXISTS uq_categories_group_name;
-- ALTER TABLE installments DROP COLUMN IF EXISTS group_id;
-- ALTER TABLE budgets DROP COLUMN IF EXISTS group_id;
-- ALTER TABLE transactions DROP COLUMN IF EXISTS group_id;
-- ALTER TABLE categories DROP COLUMN IF EXISTS group_id;
-- DROP TABLE IF EXISTS group_members;
-- DROP TABLE IF EXISTS groups;
