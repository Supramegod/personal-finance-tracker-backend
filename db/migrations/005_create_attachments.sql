-- 005_create_attachments.sql
-- Tabel: attachments (P2)
-- Migrasi: UP
-- Lampiran file (mis. foto struk). Implementasi ditunda ke P2.
-- Skema sudah siap agar tidak perlu migrasi besar saat fitur ini dikerjakan.

CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    file_url VARCHAR(500) NOT NULL,
    file_name VARCHAR(255) NULL,
    file_size INTEGER NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_attachments_transaction ON attachments(transaction_id);

-- DOWN:
-- DROP TABLE IF EXISTS attachments;
