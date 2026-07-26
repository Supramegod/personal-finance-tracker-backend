package service

import (
	"errors"
	"strings"

	"personal-finance-tracker/internal/repository"
	"personal-finance-tracker/pkg/auth"
)

// GroupService berisi business rule untuk pengelolaan kelompok (groups),
// keanggotaan, dan pembuatan akun user oleh owner.
type GroupService struct {
	groupRepo *repository.GroupRepository
	userRepo  *repository.UserRepository
}

func NewGroupService(
	groupRepo *repository.GroupRepository,
	userRepo *repository.UserRepository,
) *GroupService {
	return &GroupService{
		groupRepo: groupRepo,
		userRepo:  userRepo,
	}
}

// Create membuat group baru milik userID (jadi owner) lalu meng-seed kategori
// default ke group tersebut.
func (s *GroupService) Create(userID, name string) (*repository.Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name is required")
	}

	group, err := s.groupRepo.Create(name, userID)
	if err != nil {
		return nil, err
	}

	if err := s.groupRepo.SeedDefaultCategories(group.ID, userID); err != nil {
		return nil, err
	}

	return group, nil
}

// ListForUser mengembalikan semua group yang diikuti userID.
func (s *GroupService) ListForUser(userID string) ([]repository.Group, error) {
	return s.groupRepo.ListForUser(userID)
}

// CreateUser membuat akun user baru (dilakukan oleh owner). Tidak otomatis
// menempelkannya ke group mana pun — assignment dilakukan terpisah lewat
// AddMember.
// CreateUser membuat akun user baru DAN langsung menambahkannya sebagai anggota
// group aktif (activeGroupID). Tanpa ini, user baru tidak jadi anggota group mana
// pun sehingga tidak muncul di "kolam" (ListManagedUsers) dan tak bisa di-drag.
// Hanya owner group aktif yang boleh membuat user.
func (s *GroupService) CreateUser(ownerUserID, activeGroupID, email, password, fullName string) (*repository.User, error) {
	// Hanya owner group aktif yang boleh membuat & menempatkan user.
	role, err := s.groupRepo.RoleOf(ownerUserID, activeGroupID)
	if err != nil {
		return nil, err
	}
	if role != "owner" {
		return nil, errors.New("only owner can create users")
	}

	email = strings.TrimSpace(strings.ToLower(email))
	fullName = strings.TrimSpace(fullName)

	if email == "" {
		return nil, errors.New("email is required")
	}
	if !strings.Contains(email, "@") {
		return nil, errors.New("invalid email format")
	}
	if len(password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}
	if fullName == "" {
		return nil, errors.New("full_name is required")
	}

	hashed, err := auth.HashPassword(password)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	user := &repository.User{
		Email:        email,
		PasswordHash: hashed,
		FullName:     fullName,
	}

	if err := s.userRepo.Create(user); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, errors.New("email already registered")
		}
		return nil, err
	}

	// Masukkan ke group aktif sebagai member (idempotent) agar langsung tampil
	// di kolam dan bisa dipindah lewat drag-and-drop.
	if err := s.groupRepo.AddMember(activeGroupID, user.ID, "member"); err != nil {
		return nil, err
	}

	return user, nil
}

// ListManagedUsers mengembalikan "kolam" user: semua anggota dari group-group
// yang di-own ownerUserID.
func (s *GroupService) ListManagedUsers(ownerUserID string) ([]repository.User, error) {
	return s.groupRepo.ListManagedUsers(ownerUserID)
}

// AddMember menambahkan targetUserID ke groupID. Hanya owner group yang boleh.
// role default 'member'.
func (s *GroupService) AddMember(requesterUserID, groupID, targetUserID, role string) error {
	requesterRole, err := s.groupRepo.RoleOf(requesterUserID, groupID)
	if err != nil {
		return err
	}
	if requesterRole != "owner" {
		return errors.New("only owner can manage members")
	}

	if targetUserID == "" {
		return errors.New("user_id is required")
	}

	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		role = "member"
	}
	if role != "owner" && role != "member" {
		return errors.New("role must be 'owner' or 'member'")
	}

	return s.groupRepo.AddMember(groupID, targetUserID, role)
}

// RemoveMember menghapus targetUserID dari groupID. Hanya owner yang boleh, dan
// owner group tidak boleh dihapus.
func (s *GroupService) RemoveMember(requesterUserID, groupID, targetUserID string) error {
	requesterRole, err := s.groupRepo.RoleOf(requesterUserID, groupID)
	if err != nil {
		return err
	}
	if requesterRole != "owner" {
		return errors.New("only owner can manage members")
	}

	group, err := s.groupRepo.FindByID(groupID)
	if err != nil {
		return errors.New("group not found")
	}
	if group.OwnerUserID == targetUserID {
		return errors.New("cannot remove group owner")
	}

	return s.groupRepo.RemoveMember(groupID, targetUserID)
}

// ListMembers mengembalikan anggota groupID. Requester harus anggota group.
func (s *GroupService) ListMembers(requesterUserID, groupID string) ([]repository.GroupMember, error) {
	isMember, err := s.groupRepo.IsMember(requesterUserID, groupID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, errors.New("not a member of this group")
	}

	return s.groupRepo.ListMembers(groupID)
}
