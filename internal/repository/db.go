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
		log.Printf("Admin user %s already exists, skipping user creation", adminEmail)
		// Admin sudah ada — pastikan group default-nya juga ada (idempotent).
		// Migrasi 010 mem-backfill data lama, tapi bila admin belum punya data
		// finansial ia bisa saja belum punya group; ensureGroupForAdmin
		// menanganinya dan tidak menduplikasi bila group sudah ada.
		if err := db.ensureGroupForAdmin(adminEmail); err != nil {
			return fmt.Errorf("ensure admin group: %w", err)
		}
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

	// Buat group default untuk admin + seed kategori default ke group itu.
	if err := db.ensureGroupForAdmin(adminEmail); err != nil {
		return fmt.Errorf("ensure admin group: %w", err)
	}

	return nil
}

// ensureGroupForAdmin memastikan admin punya satu group default dengan dirinya
// sebagai owner, lalu meng-seed kategori default ke group tersebut. Idempotent:
// bila admin sudah punya group (owner_user_id = admin), tidak menduplikasi.
func (db *DB) ensureGroupForAdmin(adminEmail string) error {
	// Ambil id & nama admin.
	var userID, fullName string
	err := db.Pool.QueryRow(context.Background(),
		"SELECT id, full_name FROM users WHERE email = $1", adminEmail).Scan(&userID, &fullName)
	if err != nil {
		return fmt.Errorf("get admin id: %w", err)
	}

	// Sudah punya group? Jangan duplikasi.
	var groupID string
	err = db.Pool.QueryRow(context.Background(),
		"SELECT id FROM groups WHERE owner_user_id = $1 LIMIT 1", userID).Scan(&groupID)
	if err == nil {
		log.Printf("Admin group already exists for %s, skipping group seed", adminEmail)
		return nil
	}

	// Buat group default.
	groupName := fullName
	if groupName == "" {
		groupName = "Keluarga"
	}
	err = db.Pool.QueryRow(context.Background(),
		`INSERT INTO groups (name, owner_user_id) VALUES ($1, $2) RETURNING id`,
		groupName, userID).Scan(&groupID)
	if err != nil {
		return fmt.Errorf("create admin group: %w", err)
	}

	_, err = db.Pool.Exec(context.Background(),
		`INSERT INTO group_members (group_id, user_id, role)
		 VALUES ($1, $2, 'owner')
		 ON CONFLICT (group_id, user_id) DO NOTHING`,
		groupID, userID)
	if err != nil {
		return fmt.Errorf("create admin group membership: %w", err)
	}

	log.Printf("Admin group %q created successfully", groupName)

	// Seed kategori default ke group.
	if err := db.seedDefaultCategories(groupID, userID); err != nil {
		return fmt.Errorf("seed categories: %w", err)
	}
	return nil
}

// seedDefaultCategories mengisi kategori default (income & expense) ke groupID.
// user_id diisi creatorUserID sebagai "dibuat oleh". Idempotent lewat
// ON CONFLICT (group_id, name).
func (db *DB) seedDefaultCategories(groupID, creatorUserID string) error {
	categories := []struct {
		Name string
		Type string
		Icon string
	}{
		// Income
		{"Gaji", "income", "salary"},
		{"Freelance", "income", "freelance"},
		{"Bisnis", "income", "business"},
		{"Investasi", "income", "investment"},
		{"Hadiah", "income", "gift"},
		{"Lainnya", "income", "other_income"},
		// Expense
		{"Makan & Minum", "expense", "food"},
		{"Transportasi", "expense", "transport"},
		{"Belanja", "expense", "shopping"},
		{"Hiburan", "expense", "entertainment"},
		{"Kesehatan", "expense", "health"},
		{"Pendidikan", "expense", "education"},
		{"Tagihan", "expense", "bill"},
		{"Tabungan", "expense", "savings"},
		{"Lainnya", "expense", "other_expense"},
	}

	for _, cat := range categories {
		_, err := db.Pool.Exec(context.Background(),
			`INSERT INTO categories (group_id, user_id, name, type, icon, is_default)
			 VALUES ($1, $2, $3, $4, $5, true)
			 ON CONFLICT (group_id, name) DO NOTHING`,
			groupID, creatorUserID, cat.Name, cat.Type, cat.Icon)
		if err != nil {
			return fmt.Errorf("seed category %s: %w", cat.Name, err)
		}
	}

	log.Println("Default categories seeded successfully")
	return nil
}
