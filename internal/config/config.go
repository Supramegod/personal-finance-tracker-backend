package config

import (
	"log"
	"os"
	"strconv"
	"time"
)

// Config menyimpan semua konfigurasi aplikasi dari environment variable.
type Config struct {
	// Database
	DatabaseURL       string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	// Auth
	JWTSecret       string
	JWTAccessExpiry time.Duration
	JWTRefreshExpiry time.Duration

	// Admin Seed
	AdminEmail    string
	AdminPassword string

	// Server
	Port  string
	Env   string

	// Swagger
	SwaggerHost string

	// CORS
	CORSOrigins string

	// Rate Limiting
	RateLimitPerMinute int

	// Logging
	LogLevel string
}

// Load membaca konfigurasi dari environment variable.
func Load() *Config {
	cfg := &Config{
		DatabaseURL:        getEnv("DATABASE_URL", "postgresql://finance:finance123@localhost:5432/finance_tracker?sslmode=disable"),
		DBMaxOpenConns:     getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:     getEnvInt("DB_MAX_IDLE_CONNS", 5),
		DBConnMaxLifetime:  getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		JWTAccessExpiry:    GetJWTAccessExpiry(),
		JWTRefreshExpiry:   GetJWTRefreshExpiry(),
		AdminEmail:         getEnv("ADMIN_EMAIL", "admin@example.com"),
		AdminPassword:      getEnv("ADMIN_PASSWORD", "admin123"),
		Port:               getEnv("PORT", "8080"),
		Env:                getEnv("APP_ENV", "development"),
		SwaggerHost:        getEnv("SWAGGER_HOST", "localhost:8080"),
		CORSOrigins:        getEnv("CORS_ORIGINS", "http://localhost:3000,http://localhost:5173"),
		RateLimitPerMinute: getEnvInt("RATE_LIMIT_PER_MINUTE", 60),
		LogLevel:           getEnv("LOG_LEVEL", "debug"),
	}

	if cfg.JWTSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		i, err := strconv.Atoi(val)
		if err != nil {
			log.Printf("Warning: invalid %s=%s, using default %d", key, val, defaultVal)
			return defaultVal
		}
		return i
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			log.Printf("Warning: invalid %s=%s, using default %s", key, val, defaultVal)
			return defaultVal
		}
		return d
	}
	return defaultVal
}

// GetJWTAccessExpiry mengembalikan durasi expiry access token dari environment
// variable JWT_ACCESS_EXPIRY. Ini adalah satu-satunya tempat yang mem-parsing
// env var tersebut, sehingga package lain (mis. pkg/auth) tidak perlu
// mem-parsing ulang dan berpotensi berbeda hasil.
func GetJWTAccessExpiry() time.Duration {
	return getEnvDuration("JWT_ACCESS_EXPIRY", 15*time.Minute)
}

// GetJWTRefreshExpiry mengembalikan durasi expiry refresh token dari
// environment variable JWT_REFRESH_EXPIRY. Sama seperti GetJWTAccessExpiry,
// ini adalah satu-satunya sumber parsing agar JWT dan data yang tersimpan di
// DB (expires_at) tidak pernah drift satu sama lain.
func GetJWTRefreshExpiry() time.Duration {
	return getEnvDuration("JWT_REFRESH_EXPIRY", 7*24*time.Hour)
}
