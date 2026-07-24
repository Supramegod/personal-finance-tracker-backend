-- name: StoreRefreshToken :exec
INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3);

-- name: FindValidRefreshToken :one
SELECT user_id, expires_at
FROM refresh_tokens
WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > NOW();

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeAllUserRefreshTokens :exec
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: CleanupExpiredTokens :exec
DELETE FROM refresh_tokens
WHERE expires_at < NOW() - INTERVAL '30 days'
   OR (revoked_at IS NOT NULL AND revoked_at < NOW() - INTERVAL '30 days');
