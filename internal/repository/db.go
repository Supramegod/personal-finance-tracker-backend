package repository

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type DB struct {
	Pool *pgxpool.Pool
}

func NewDB(databaseURL string) (*DB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Println("Database connected successfully")
	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
	log.Println("Database connection closed")
}

func (db *DB) RunMigrations() error {
	migrationsDir := "db/migrations"
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	sort.Strings(files)

	// File di db/migrations/ HANYA boleh berisi perubahan skema (DDL).
	// Seeding user admin dan kategori default ditangani SeedAdmin(), bukan SQL —
	// supaya password di-hash oleh kode aplikasi dan kredensialnya berasal dari
	// environment variable, bukan tertulis di dalam file yang ikut ter-commit.
	// Direktori ini juga di-mount ke /docker-entrypoint-initdb.d oleh
	// docker-compose, jadi file seed di sini akan tetap jalan meski dilewati
	// di kode — itulah sebabnya file seed dihapus, bukan sekadar di-skip.
	for _, file := range files {
		sql, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", file, err)
		}

		if _, err := db.Pool.Exec(context.Background(), string(sql)); err != nil {
			// Skip error if object already exists (e.g., table, extension already created)
			if strings.Contains(err.Error(), "sudah ada") || strings.Contains(err.Error(), "already exists") {
				log.Printf("Migration skipped (already exists): %s", filepath.Base(file))
				continue
			}
			return fmt.Errorf("execute migration %s: %w", file, err)
		}

		log.Printf("Migration applied: %s", filepath.Base(file))
	}

	log.Println("All migrations applied successfully")
	return nil
}

func (db *DB) SeedAdmin() error {
	adminEmail := os.Getenv("ADMIN_EMAIL")
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminEmail == "" || adminPassword == "" {
		return fmt.Errorf("ADMIN_EMAIL and ADMIN_PASSWORD must be set")
	}

	// Check if admin already exists
	var count int
	err := db.Pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM users WHERE email = $1", adminEmail).Scan(&count)
	if err != nil {
		return fmt.Errorf("check admin exists: %w", err)
	}

	if count > 0 {
		log.Printf("Admin user %s already exists, skipping seed", adminEmail)
		return nil
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(adminPassword), 12)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Create admin user
	_, err = db.Pool.Exec(context.Background(),
		`INSERT INTO users (email, password_hash, full_name)
		 VALUES ($1, $2, 'Admin')`,
		adminEmail, string(hashedPassword))
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	log.Printf("Admin user %s created successfully", adminEmail)

	// Seed default categories for admin
	if err := db.seedDefaultCategories(adminEmail); err != nil {
		return fmt.Errorf("seed categories: %w", err)
	}

	return nil
}

func (db *DB) seedDefaultCategories(adminEmail string) error {
	// Get admin user ID
	var userID string
	err := db.Pool.QueryRow(context.Background(),
		"SELECT id FROM users WHERE email = $1", adminEmail).Scan(&userID)
	if err != nil {
		return fmt.Errorf("get admin id: %w", err)
	}

	// Default income categories
	incomeCategories := []struct {
		Name string
		Icon string
	}{
		{"Gaji", "salary"},
		{"Freelance", "freelance"},
		{"Bisnis", "business"},
		{"Investasi", "investment"},
		{"Hadiah", "gift"},
		{"Lainnya", "other_income"},
	}

	for _, cat := range incomeCategories {
		_, err := db.Pool.Exec(context.Background(),
			`INSERT INTO categories (user_id, name, type, icon, is_default)
			 VALUES ($1, $2, 'income', $3, true)
			 ON CONFLICT (user_id, name) DO NOTHING`,
			userID, cat.Name, cat.Icon)
		if err != nil {
			return fmt.Errorf("seed income category %s: %w", cat.Name, err)
		}
	}

	// Default expense categories
	expenseCategories := []struct {
		Name string
		Icon string
	}{
		{"Makan & Minum", "food"},
		{"Transportasi", "transport"},
		{"Belanja", "shopping"},
		{"Hiburan", "entertainment"},
		{"Kesehatan", "health"},
		{"Pendidikan", "education"},
		{"Tagihan", "bill"},
		{"Tabungan", "savings"},
		{"Lainnya", "other_expense"},
	}

	for _, cat := range expenseCategories {
		_, err := db.Pool.Exec(context.Background(),
			`INSERT INTO categories (user_id, name, type, icon, is_default)
			 VALUES ($1, $2, 'expense', $3, true)
			 ON CONFLICT (user_id, name) DO NOTHING`,
			userID, cat.Name, cat.Icon)
		if err != nil {
			return fmt.Errorf("seed expense category %s: %w", cat.Name, err)
		}
	}

	log.Println("Default categories seeded successfully")
	return nil
}
