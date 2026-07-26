package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create menyimpan user baru ke tabel users. password_hash diharapkan
// sudah di-hash oleh caller (service). Mengembalikan id, created_at,
// updated_at hasil generate DB via RETURNING.
func (r *UserRepository) Create(u *User) error {
	err := r.pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, full_name)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at, updated_at`,
		u.Email, u.PasswordHash, u.FullName).Scan(
		&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) FindByEmail(email string) (*User, error) {
	user := &User{}
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, email, password_hash, full_name, created_at, updated_at
		 FROM users WHERE email = $1`, email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.FullName,
		&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *UserRepository) ListAll() ([]User, error) {
	rows, err := r.pool.Query(context.Background(),
		`SELECT id, email, password_hash, full_name, created_at, updated_at
		 FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName,
			&u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *UserRepository) FindByID(id string) (*User, error) {
	user := &User{}
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, email, password_hash, full_name, created_at, updated_at
		 FROM users WHERE id = $1`, id).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.FullName,
		&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	FullName     string    `json:"full_name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
