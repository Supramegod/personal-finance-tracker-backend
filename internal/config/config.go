package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// minJWTSecretLength adalah panjang minimum JWT_SECRET yang diterima,
// sesuai aturan keamanan di stack-conventions.md.
const minJWTSecretLength = 32

// Config menyimpan semua konfigurasi aplikasi dari environment variable.
type Config struct {
	// Database
	DatabaseURL       string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	// Auth
	JWTSecret        string
	JWTAccessExpiry  time.Duration
	JWTRefreshExpiry time.Duration

	// Admin Seed
	AdminEmail    string
	AdminPassword string

	// Server
	Port string
	Env  string

	// Swagger
	SwaggerHost string

	// CORS
	CORSOrigins string

	// Rate Limiting
	RateLimitPerMinute int

	// Logging
	LogLevel string

	// AI Insights (optional; server tetap berjalan bila dinonaktifkan).
	AIInsightsEnabled bool
	GeminiAPIKey      string
	AIModel           string
	AIPromptVersion   string
	AITimeout         time.Duration
}

// Load membaca konfigurasi dari environment variable.
func Load() *Config {
	cfg := &Config{
		// Kredensial WAJIB diisi lewat environment variable — tidak ada
		// nilai default. Default yang lemah (mis. "admin123") ikut terbaca
		// publik lewat source code dan akan terpakai diam-diam kalau env
		// var lupa diset saat deploy.
		DatabaseURL:        getEnv("DATABASE_URL", ""),
		DBMaxOpenConns:     getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:     getEnvInt("DB_MAX_IDLE_CONNS", 5),
		DBConnMaxLifetime:  getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		JWTAccessExpiry:    GetJWTAccessExpiry(),
		JWTRefreshExpiry:   GetJWTRefreshExpiry(),
		AdminEmail:         getEnv("ADMIN_EMAIL", ""),
		AdminPassword:      getEnv("ADMIN_PASSWORD", ""),
		Port:               getEnv("PORT", "8080"),
		Env:                getEnv("APP_ENV", "development"),
		SwaggerHost:        getEnv("SWAGGER_HOST", "localhost:8080"),
		CORSOrigins:        getEnv("CORS_ORIGINS", "http://localhost:3000,http://localhost:5173"),
		RateLimitPerMinute: getEnvInt("RATE_LIMIT_PER_MINUTE", 60),
		LogLevel:           getEnv("LOG_LEVEL", "debug"),
		AIInsightsEnabled:  getEnvBool("AI_INSIGHTS_ENABLED", false),
		GeminiAPIKey:       getEnv("GEMINI_API_KEY", ""),
		AIModel:            getEnv("AI_MODEL", "gemini-2.5-flash-lite"),
		AIPromptVersion:    getEnv("AI_PROMPT_VERSION", "v1"),
		AITimeout:          getEnvDuration("AI_TIMEOUT", 30*time.Second),
	}

	// Kumpulkan semua yang kosong dulu, baru lapor sekaligus — supaya tidak
	// perlu start-gagal berulang kali satu env var per percobaan.
	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if cfg.AdminEmail == "" {
		missing = append(missing, "ADMIN_EMAIL")
	}
	if cfg.AdminPassword == "" {
		missing = append(missing, "ADMIN_PASSWORD")
	}
	if len(missing) > 0 {
		log.Fatalf("environment variable wajib belum diset: %s "+
			"(lihat .env.example, atau salin: cp .env.example .env)",
			strings.Join(missing, ", "))
	}

	// JWT_SECRET pendek membuat HMAC-SHA256 rentan brute force.
	if len(cfg.JWTSecret) < minJWTSecretLength {
		log.Fatalf("JWT_SECRET terlalu pendek: %d karakter, minimal %d "+
			"(generate: openssl rand -base64 48)",
			len(cfg.JWTSecret), minJWTSecretLength)
	}

	return cfg
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.ParseBool(val)
		if err == nil {
			return parsed
		}
		log.Printf("Warning: invalid %s=%s, using default %t", key, val, defaultVal)
	}
	return defaultVal
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
