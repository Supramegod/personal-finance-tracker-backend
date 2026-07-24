// Package domain berisi entity/model structs untuk Personal Finance Tracker.
// Struct di sini adalah representasi domain murni (tanpa tag json/db).
// Untuk struct dengan tag dan query, gunakan package repository.
package domain

import "time"

// User merepresentasikan pengguna aplikasi.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	FullName     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Category merepresentasikan kategori transaksi.
type Category struct {
	ID        string
	UserID    string
	Name      string
	Type      string // "income" or "expense"
	Icon      string
	IsDefault bool
	CreatedAt time.Time
}

// Transaction merepresentasikan transaksi keuangan.
type Transaction struct {
	ID              string
	UserID          string
	CategoryID      string
	Type            string // "income" or "expense"
	Amount          float64
	TransactionDate time.Time
	Note            string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

// Budget merepresentasikan anggaran per kategori (P1).
type Budget struct {
	ID          string
	UserID      string
	CategoryID  string
	Period      string // "weekly" or "monthly"
	StartDate   time.Time
	LimitAmount float64
	CreatedAt   time.Time
}

// RefreshToken merepresentasikan token refresh yang disimpan di server.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}
