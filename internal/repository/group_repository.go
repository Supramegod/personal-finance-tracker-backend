package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GroupRepository menangani semua akses data terkait kelompok (groups),
// keanggotaan (group_members), dan seeding kategori default per group.
type GroupRepository struct {
	pool *pgxpool.Pool
}

func NewGroupRepository(pool *pgxpool.Pool) *GroupRepository {
	return &GroupRepository{pool: pool}
}

// Group merepresentasikan satu kelompok. Role diisi hanya pada query yang
// juga membawa keanggotaan user tertentu (mis. ListForUser).
type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	OwnerUserID string    `json:"owner_user_id"`
	CreatedAt   time.Time `json:"created_at"`
	Role        string    `json:"role,omitempty"`
}

// GroupMember merepresentasikan keanggotaan user pada sebuah group, lengkap
// dengan data user (email & full_name) hasil JOIN untuk kebutuhan tampilan.
type GroupMember struct {
	ID        string    `json:"id"`
	GroupID   string    `json:"group_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	CreatedAt time.Time `json:"created_at"`
}

// Create membuat group baru dan sekaligus menambahkan ownerUserID sebagai
// anggota dengan role 'owner'.
func (r *GroupRepository) Create(name, ownerUserID string) (*Group, error) {
	g := &Group{Name: name, OwnerUserID: ownerUserID}
	err := r.pool.QueryRow(context.Background(),
		`INSERT INTO groups (name, owner_user_id)
		 VALUES ($1, $2)
		 RETURNING id, created_at`,
		name, ownerUserID).Scan(&g.ID, &g.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}

	_, err = r.pool.Exec(context.Background(),
		`INSERT INTO group_members (group_id, user_id, role)
		 VALUES ($1, $2, 'owner')
		 ON CONFLICT (group_id, user_id) DO NOTHING`,
		g.ID, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("create group owner membership: %w", err)
	}

	g.Role = "owner"
	return g, nil
}

// ListForUser mengembalikan semua group yang diikuti userID beserta role-nya.
func (r *GroupRepository) ListForUser(userID string) ([]Group, error) {
	rows, err := r.pool.Query(context.Background(),
		`SELECT g.id, g.name, g.owner_user_id, g.created_at, gm.role
		 FROM groups g
		 JOIN group_members gm ON gm.group_id = g.id
		 WHERE gm.user_id = $1
		 ORDER BY g.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("list groups for user: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.OwnerUserID, &g.CreatedAt, &g.Role); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// FindByID mencari group berdasarkan id.
func (r *GroupRepository) FindByID(id string) (*Group, error) {
	g := &Group{}
	err := r.pool.QueryRow(context.Background(),
		`SELECT id, name, owner_user_id, created_at
		 FROM groups WHERE id = $1`, id).Scan(
		&g.ID, &g.Name, &g.OwnerUserID, &g.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("find group by id: %w", err)
	}
	return g, nil
}

// IsMember mengecek apakah userID adalah anggota groupID.
func (r *GroupRepository) IsMember(userID, groupID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(context.Background(),
		`SELECT EXISTS (
			SELECT 1 FROM group_members WHERE user_id = $1 AND group_id = $2
		 )`, userID, groupID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check membership: %w", err)
	}
	return exists, nil
}

// RoleOf mengembalikan role user pada group tertentu, atau "" bila user
// bukan anggota group tersebut.
func (r *GroupRepository) RoleOf(userID, groupID string) (string, error) {
	var role string
	err := r.pool.QueryRow(context.Background(),
		`SELECT role FROM group_members WHERE user_id = $1 AND group_id = $2`,
		userID, groupID).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("role of user: %w", err)
	}
	return role, nil
}

// DefaultGroupForUser mengembalikan group_id default untuk userID: diutamakan
// group yang dia 'owner'; bila tidak ada, ambil membership pertama (paling lama).
// Mengembalikan error bila user tidak punya group sama sekali.
func (r *GroupRepository) DefaultGroupForUser(userID string) (string, error) {
	var groupID string
	err := r.pool.QueryRow(context.Background(),
		`SELECT gm.group_id
		 FROM group_members gm
		 JOIN groups g ON g.id = gm.group_id
		 WHERE gm.user_id = $1
		 ORDER BY (gm.role = 'owner') DESC, g.created_at
		 LIMIT 1`, userID).Scan(&groupID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("default group for user: user has no group")
		}
		return "", fmt.Errorf("default group for user: %w", err)
	}
	return groupID, nil
}

// AddMember menambahkan userID ke groupID dengan role tertentu. Idempotent:
// bila keanggotaan sudah ada, tidak melakukan apa-apa.
func (r *GroupRepository) AddMember(groupID, userID, role string) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO group_members (group_id, user_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (group_id, user_id) DO NOTHING`,
		groupID, userID, role)
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

// RemoveMember menghapus keanggotaan userID dari groupID.
func (r *GroupRepository) RemoveMember(groupID, userID string) error {
	_, err := r.pool.Exec(context.Background(),
		`DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`,
		groupID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	return nil
}

// ListMembers mengembalikan semua anggota groupID beserta email & full_name.
func (r *GroupRepository) ListMembers(groupID string) ([]GroupMember, error) {
	rows, err := r.pool.Query(context.Background(),
		`SELECT gm.id, gm.group_id, gm.user_id, gm.role, u.email, u.full_name, gm.created_at
		 FROM group_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.group_id = $1
		 ORDER BY gm.created_at`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var members []GroupMember
	for rows.Next() {
		var m GroupMember
		if err := rows.Scan(&m.ID, &m.GroupID, &m.UserID, &m.Role,
			&m.Email, &m.FullName, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}

// defaultCategory adalah blueprint kategori default yang di-seed ke setiap
// group baru. Daftar ini identik dengan seed lama (per-user) di db.go, tapi
// kini menargetkan group_id.
type defaultCategory struct {
	Name string
	Type string
	Icon string
}

var defaultCategories = []defaultCategory{
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

// SeedDefaultCategories mengisi kategori default (income & expense) untuk
// groupID. user_id diisi creatorUserID sebagai "dibuat oleh". Idempotent
// lewat ON CONFLICT (group_id, name).
func (r *GroupRepository) SeedDefaultCategories(groupID, creatorUserID string) error {
	for _, cat := range defaultCategories {
		_, err := r.pool.Exec(context.Background(),
			`INSERT INTO categories (group_id, user_id, name, type, icon, is_default)
			 VALUES ($1, $2, $3, $4, $5, true)
			 ON CONFLICT (group_id, name) DO NOTHING`,
			groupID, creatorUserID, cat.Name, cat.Type, cat.Icon)
		if err != nil {
			return fmt.Errorf("seed default category %s: %w", cat.Name, err)
		}
	}
	return nil
}

// ListManagedUsers mengembalikan semua user DISTINCT yang menjadi anggota
// salah satu group yang di-own ownerUserID. Dipakai sebagai "kolam" user di
// frontend untuk di-assign ke group.
func (r *GroupRepository) ListManagedUsers(ownerUserID string) ([]User, error) {
	rows, err := r.pool.Query(context.Background(),
		`SELECT DISTINCT u.id, u.email, u.password_hash, u.full_name, u.created_at, u.updated_at
		 FROM users u
		 JOIN group_members gm ON gm.user_id = u.id
		 JOIN groups g ON g.id = gm.group_id
		 WHERE g.owner_user_id = $1
		 ORDER BY u.created_at`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list managed users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName,
			&u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan managed user: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}
