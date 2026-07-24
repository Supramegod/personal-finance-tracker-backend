-- 006_create_refresh_tokens.sql
-- Tabel: refresh_tokens
-- Migrasi: UP
-- State server-side untuk refresh token, memungkinkan revoke sesi (logout/rotation).
-- Jangan simpan plain refresh token — hanya hash-nya (token_hash UNIQUE).
-- Saat logout: set revoked_at = NOW()
-- Saat refresh token rotation: revoke token lama + insert baris baru

CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,     -- hash refresh token (bcrypt atau SHA-256)
    expires_at TIMESTAMPTZ NOT NULL,              -- masa berlaku refresh token
    revoked_at TIMESTAMPTZ NULL,                  -- NULL = masih aktif, terisi = sudah direvoke
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_active ON refresh_tokens(user_id, revoked_at)
    WHERE revoked_at IS NULL;

-- DOWN:
-- DROP TABLE IF EXISTS refresh_tokens;
