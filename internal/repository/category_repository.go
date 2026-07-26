package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CategoryRepository struct {
	pool *pgxpool.Pool
}

func NewCategoryRepository(pool *pgxpool.Pool) *CategoryRepository {
	return &CategoryRepository{pool: pool}
}

// FindByGroupID mengembalikan kategori milik groupID, opsional difilter tipe.
// Scoping data kategori kini berbasis group, bukan user.
func (r *CategoryRepository) FindByGroupID(groupID string, categoryType string) ([]Category, error) {
	query := `SELECT id, group_id, user_id, name, type, icon, is_default, created_at
			  FROM categories WHERE group_id = $1`
	args := []interface{}{groupID}

	if categoryType != "" {
		query += ` AND type = $2 ORDER BY name`
		args = append(args, categoryType)
	} else {
		query += ` ORDER BY type, name`
	}

	rows, err := r.pool.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.GroupID, &c.UserID, &c.Name, &c.Type, &c.Icon, &c.IsDefault, &c.CreatedAt); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}

	return categories, nil
}

func (r *CategoryRepository) Create(category *Category) error {
	err := r.pool.QueryRow(context.Background(),
		`INSERT INTO categories (group_id, user_id, name, type, icon, is_default)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`,
		category.GroupID, category.UserID, category.Name, category.Type, category.Icon, false).Scan(
		&category.ID, &category.CreatedAt)
	return err
}

func (r *CategoryRepository) FindByID(id string) (*Category, error) {
	c := &Category{}
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, group_id, user_id, name, type, icon, is_default, created_at
		 FROM categories WHERE id = $1`, id).Scan(
		&c.ID, &c.GroupID, &c.UserID, &c.Name, &c.Type, &c.Icon, &c.IsDefault, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// FindByIDAndGroupID mencari kategori berdasarkan id dan memastikan kategori
// tersebut milik groupID. Digunakan untuk memvalidasi kepemilikan category_id
// sebelum dipakai pada transaksi/cicilan, supaya group A tidak bisa memakai
// category_id milik group B (juga menangkap id UUID tidak valid, karena query
// akan gagal/no-rows).
func (r *CategoryRepository) FindByIDAndGroupID(id string, groupID string) (*Category, error) {
	c := &Category{}
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, group_id, user_id, name, type, icon, is_default, created_at
		 FROM categories WHERE id = $1 AND group_id = $2`, id, groupID).Scan(
		&c.ID, &c.GroupID, &c.UserID, &c.Name, &c.Type, &c.Icon, &c.IsDefault, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// FindByIDAndUserID mencari kategori berdasarkan id dan memastikan kategori
// tersebut milik userID. Digunakan untuk memvalidasi kepemilikan category_id
// sebelum dipakai pada transaksi, supaya user A tidak bisa memakai
// category_id milik user B (juga menangkap id UUID yang tidak valid,
// karena query akan gagal/no-rows).
func (r *CategoryRepository) FindByIDAndUserID(id string, userID string) (*Category, error) {
	c := &Category{}
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, user_id, name, type, icon, is_default, created_at
		 FROM categories WHERE id = $1 AND user_id = $2`, id, userID).Scan(
		&c.ID, &c.UserID, &c.Name, &c.Type, &c.Icon, &c.IsDefault, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

type Category struct {
	ID        string `json:"id"`
	GroupID   string `json:"group_id"`
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Icon      string `json:"icon"`
	IsDefault bool   `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}
