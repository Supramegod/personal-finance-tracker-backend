package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

// HashToken produces a SHA-256 hash of the token for secure storage
func (r *RefreshTokenRepository) HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// Store saves a new refresh token hash for a user
func (r *RefreshTokenRepository) Store(userID, tokenHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt)
	return err
}

// IsValid checks if a token hash exists, is not revoked, and is not expired
func (r *RefreshTokenRepository) IsValid(tokenHash string) (bool, string, error) {
	var userID string
	var expiresAt time.Time
	err := r.pool.QueryRow(context.Background(),
		`SELECT user_id, expires_at FROM refresh_tokens
		 WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > NOW()`,
		tokenHash).Scan(&userID, &expiresAt)
	if err != nil {
		return false, "", err
	}
	return true, userID, nil
}

// Revoke sets revoked_at for a token hash (used during logout and token rotation)
func (r *RefreshTokenRepository) Revoke(tokenHash string) error {
	_, err := r.pool.Exec(context.Background(),
		`UPDATE refresh_tokens SET revoked_at = NOW()
		 WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash)
	return err
}

// RevokeAllByUserID revokes all active tokens for a user (force logout all sessions)
func (r *RefreshTokenRepository) RevokeAllByUserID(userID string) error {
	_, err := r.pool.Exec(context.Background(),
		`UPDATE refresh_tokens SET revoked_at = NOW()
		 WHERE user_id = $1 AND revoked_at IS NULL`,
		userID)
	return err
}

// CleanupExpired removes expired or revoked tokens older than 30 days
func (r *RefreshTokenRepository) CleanupExpired() error {
	_, err := r.pool.Exec(context.Background(),
		`DELETE FROM refresh_tokens
		 WHERE expires_at < NOW() - INTERVAL '30 days'
		    OR (revoked_at IS NOT NULL AND revoked_at < NOW() - INTERVAL '30 days')`)
	return err
}
