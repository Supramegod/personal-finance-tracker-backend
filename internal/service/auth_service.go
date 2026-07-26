package service

import (
	"errors"
	"time"

	"personal-finance-tracker/internal/repository"
	"personal-finance-tracker/pkg/auth"
)

type AuthService struct {
	userRepo     *repository.UserRepository
	categoryRepo *repository.CategoryRepository
	refreshRepo  *repository.RefreshTokenRepository
	groupRepo    *repository.GroupRepository
}

func NewAuthService(
	userRepo *repository.UserRepository,
	categoryRepo *repository.CategoryRepository,
	refreshRepo *repository.RefreshTokenRepository,
	groupRepo *repository.GroupRepository,
) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		categoryRepo: categoryRepo,
		refreshRepo:  refreshRepo,
		groupRepo:    groupRepo,
	}
}

type LoginResponse struct {
	AccessToken   string             `json:"access_token"`
	RefreshToken  string             `json:"refresh_token"`
	User          repository.User    `json:"user"`
	ActiveGroupID string             `json:"active_group_id"`
	Groups        []repository.Group `json:"groups"`
}

func (s *AuthService) Login(email, password string) (*LoginResponse, error) {
	if email == "" || password == "" {
		return nil, errors.New("email and password are required")
	}

	user, err := s.userRepo.FindByEmail(email)
	if err != nil {
		return nil, errors.New("invalid email or password")
	}

	if !auth.CheckPassword(password, user.PasswordHash) {
		return nil, errors.New("invalid email or password")
	}

	// Tentukan group aktif default. Bila user belum punya group, tetap lanjut
	// dengan groupID kosong (scope kosong) — token tetap terbit.
	groupID, _ := s.groupRepo.DefaultGroupForUser(user.ID)

	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, groupID)
	if err != nil {
		return nil, errors.New("failed to generate access token")
	}

	refreshToken, err := auth.GenerateRefreshToken(user.ID, user.Email, groupID)
	if err != nil {
		return nil, errors.New("failed to generate refresh token")
	}

	// Store refresh token hash in DB. Masa berlaku mengikuti durasi yang
	// sama dengan yang dipakai untuk generate JWT refresh token (dari
	// JWT_REFRESH_EXPIRY), supaya keduanya tidak pernah drift.
	tokenHash := s.refreshRepo.HashToken(refreshToken)
	expiresAt := time.Now().Add(auth.RefreshTokenExpiry())
	if err := s.refreshRepo.Store(user.ID, tokenHash, expiresAt); err != nil {
		return nil, errors.New("failed to store refresh token")
	}

	groups, _ := s.groupRepo.ListForUser(user.ID)

	return &LoginResponse{
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
		User:          *user,
		ActiveGroupID: groupID,
		Groups:        groups,
	}, nil
}

func (s *AuthService) Refresh(refreshToken string) (*LoginResponse, error) {
	// Validate JWT signature dan pastikan tipe-nya memang refresh token
	// (bukan access token yang "dipinjam" untuk refresh).
	claims, err := auth.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, errors.New("invalid or expired refresh token")
	}

	// Check token hash exists in DB and is not revoked
	tokenHash := s.refreshRepo.HashToken(refreshToken)
	valid, userID, err := s.refreshRepo.IsValid(tokenHash)
	if err != nil || !valid {
		return nil, errors.New("refresh token has been revoked or expired")
	}

	// Ensure token belongs to the same user
	if claims.UserID != userID {
		return nil, errors.New("token mismatch")
	}

	// Revoke old token (rotation)
	if err := s.refreshRepo.Revoke(tokenHash); err != nil {
		return nil, errors.New("failed to rotate refresh token")
	}

	user, err := s.userRepo.FindByID(claims.UserID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Pertahankan scope group dari refresh token lama — refresh tidak boleh
	// mengubah kelompok aktif.
	groupID := claims.GroupID

	// Generate new tokens
	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, groupID)
	if err != nil {
		return nil, errors.New("failed to generate access token")
	}

	newRefreshToken, err := auth.GenerateRefreshToken(user.ID, user.Email, groupID)
	if err != nil {
		return nil, errors.New("failed to generate refresh token")
	}

	// Store new refresh token hash, dengan masa berlaku yang sama dengan JWT-nya.
	newTokenHash := s.refreshRepo.HashToken(newRefreshToken)
	expiresAt := time.Now().Add(auth.RefreshTokenExpiry())
	if err := s.refreshRepo.Store(user.ID, newTokenHash, expiresAt); err != nil {
		return nil, errors.New("failed to store new refresh token")
	}

	groups, _ := s.groupRepo.ListForUser(user.ID)

	return &LoginResponse{
		AccessToken:   accessToken,
		RefreshToken:  newRefreshToken,
		User:          *user,
		ActiveGroupID: groupID,
		Groups:        groups,
	}, nil
}

// SwitchGroup memindahkan kelompok aktif user ke groupID (harus anggota),
// menerbitkan access & refresh token baru dengan scope group tersebut, dan
// menyimpan hash refresh token baru (pola sama dengan Login).
func (s *AuthService) SwitchGroup(userID, groupID string) (*LoginResponse, error) {
	if groupID == "" {
		return nil, errors.New("group_id is required")
	}

	isMember, err := s.groupRepo.IsMember(userID, groupID)
	if err != nil {
		return nil, errors.New("failed to verify membership")
	}
	if !isMember {
		return nil, errors.New("not a member of this group")
	}

	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, groupID)
	if err != nil {
		return nil, errors.New("failed to generate access token")
	}

	refreshToken, err := auth.GenerateRefreshToken(user.ID, user.Email, groupID)
	if err != nil {
		return nil, errors.New("failed to generate refresh token")
	}

	tokenHash := s.refreshRepo.HashToken(refreshToken)
	expiresAt := time.Now().Add(auth.RefreshTokenExpiry())
	if err := s.refreshRepo.Store(user.ID, tokenHash, expiresAt); err != nil {
		return nil, errors.New("failed to store refresh token")
	}

	groups, _ := s.groupRepo.ListForUser(user.ID)

	return &LoginResponse{
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
		User:          *user,
		ActiveGroupID: groupID,
		Groups:        groups,
	}, nil
}

func (s *AuthService) Logout(refreshToken string) error {
	if refreshToken == "" {
		return errors.New("refresh_token is required")
	}

	tokenHash := s.refreshRepo.HashToken(refreshToken)
	if err := s.refreshRepo.Revoke(tokenHash); err != nil {
		return errors.New("failed to revoke refresh token")
	}

	return nil
}
