package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"personal-finance-tracker/internal/config"
)

// Tipe token yang didukung. Disimpan pada claim "typ" agar refresh token
// tidak bisa dipakai sebagai access token (dan sebaliknya) meskipun
// struktur JWT-nya identik.
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	// GroupID adalah kelompok aktif (scope kepemilikan data) dari token ini.
	// Semua query data difilter berdasarkan group_id ini. Bisa kosong bila
	// user belum punya group.
	GroupID string `json:"group_id"`
	// TokenType membedakan access token dan refresh token. Token lama
	// (sebelum field ini ada) tidak punya claim ini dan harus dianggap
	// tidak valid, bukan default ke "access".
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

func GenerateAccessToken(userID, email, groupID string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "", fmt.Errorf("JWT_SECRET is required")
	}

	expiry := config.GetJWTAccessExpiry()

	claims := Claims{
		UserID:    userID,
		Email:     email,
		GroupID:   groupID,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "personal-finance-tracker",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func GenerateRefreshToken(userID, email, groupID string) (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return "", fmt.Errorf("JWT_SECRET is required")
	}

	expiry := config.GetJWTRefreshExpiry()

	claims := Claims{
		UserID:    userID,
		Email:     email,
		GroupID:   groupID,
		TokenType: TokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "personal-finance-tracker",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// RefreshTokenExpiry mengembalikan durasi expiry refresh token yang sama
// dengan yang dipakai saat generate JWT refresh token, sehingga masa
// berlaku token di DB (expires_at) tidak pernah drift dari masa berlaku JWT.
func RefreshTokenExpiry() time.Duration {
	return config.GetJWTRefreshExpiry()
}

// ValidateToken memvalidasi signature dan masa berlaku token tanpa
// memeriksa tipe token (access/refresh). Dipertahankan untuk kompatibilitas
// dengan kode/test lain yang hanya butuh validasi umum. Untuk endpoint yang
// butuh tipe token spesifik, gunakan ValidateAccessToken/ValidateRefreshToken.
func ValidateToken(tokenString string) (*Claims, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("validate token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// ValidateAccessToken memvalidasi token dan memastikan tipe-nya adalah
// access token. Token tanpa claim "typ" (mis. token lama) ditolak.
func ValidateAccessToken(tokenString string) (*Claims, error) {
	claims, err := ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeAccess {
		return nil, fmt.Errorf("token is not an access token")
	}
	return claims, nil
}

// ValidateRefreshToken memvalidasi token dan memastikan tipe-nya adalah
// refresh token. Token tanpa claim "typ" (mis. token lama) ditolak.
func ValidateRefreshToken(tokenString string) (*Claims, error) {
	claims, err := ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeRefresh {
		return nil, fmt.Errorf("token is not a refresh token")
	}
	return claims, nil
}
