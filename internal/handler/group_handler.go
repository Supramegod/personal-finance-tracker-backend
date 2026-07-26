package handler

import (
	"github.com/gofiber/fiber/v2"

	"personal-finance-tracker/internal/repository"
	"personal-finance-tracker/internal/service"
)

// GroupHandler menangani endpoint terkait kelompok (groups), keanggotaan,
// pengelolaan user oleh owner, dan switch-group. SwitchGroup memanggil
// authService karena butuh menerbitkan ulang token dengan scope group baru.
type GroupHandler struct {
	groupService *service.GroupService
	authService  *service.AuthService
}

func NewGroupHandler(groupService *service.GroupService, authService *service.AuthService) *GroupHandler {
	return &GroupHandler{
		groupService: groupService,
		authService:  authService,
	}
}

// List godoc
// @Summary Daftar kelompok
// @Description Mendapatkan daftar kelompok yang diikuti user
// @Tags Groups
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /groups [get]
func (h *GroupHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)

	groups, err := h.groupService.ListForUser(userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if groups == nil {
		groups = []repository.Group{}
	}
	return c.JSON(fiber.Map{"data": groups})
}

type createGroupRequest struct {
	Name string `json:"name"`
}

// Create godoc
// @Summary Buat kelompok baru
// @Tags Groups
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createGroupRequest true "Data kelompok"
// @Success 201 {object} repository.Group
// @Router /groups [post]
func (h *GroupHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)

	var req createGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	group, err := h.groupService.Create(userID, req.Name)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(group)
}

// ListManagedUsers godoc
// @Summary Daftar user yang dikelola (kolam)
// @Description Semua user anggota dari group-group milik owner
// @Tags Users
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /users [get]
func (h *GroupHandler) ListManagedUsers(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)

	users, err := h.groupService.ListManagedUsers(userID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if users == nil {
		users = []repository.User{}
	}
	return c.JSON(fiber.Map{"data": users})
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

// CreateUser godoc
// @Summary Buat akun user baru (oleh owner)
// @Tags Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createUserRequest true "Data user"
// @Success 201 {object} repository.User
// @Router /users [post]
func (h *GroupHandler) CreateUser(c *fiber.Ctx) error {
	ownerUserID := c.Locals("user_id").(string)

	var req createUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	user, err := h.groupService.CreateUser(ownerUserID, req.Email, req.Password, req.FullName)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(user)
}

// ListMembers godoc
// @Summary Daftar anggota kelompok
// @Tags Groups
// @Produce json
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Router /groups/{id}/members [get]
func (h *GroupHandler) ListMembers(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	groupID := c.Params("id")

	members, err := h.groupService.ListMembers(userID, groupID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if members == nil {
		members = []repository.GroupMember{}
	}
	return c.JSON(fiber.Map{"data": members})
}

type addMemberRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// AddMember godoc
// @Summary Tambah anggota kelompok (owner-only)
// @Tags Groups
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Param request body addMemberRequest true "Anggota"
// @Success 201 {object} map[string]string
// @Router /groups/{id}/members [post]
func (h *GroupHandler) AddMember(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	groupID := c.Params("id")

	var req addMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.groupService.AddMember(userID, groupID, req.UserID, req.Role); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "member added"})
}

// RemoveMember godoc
// @Summary Hapus anggota kelompok (owner-only)
// @Tags Groups
// @Produce json
// @Security BearerAuth
// @Param id path string true "Group ID"
// @Param userId path string true "User ID"
// @Success 200 {object} map[string]string
// @Router /groups/{id}/members/{userId} [delete]
func (h *GroupHandler) RemoveMember(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)
	groupID := c.Params("id")
	targetUserID := c.Params("userId")

	if err := h.groupService.RemoveMember(userID, groupID, targetUserID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "member removed"})
}

type switchGroupRequest struct {
	GroupID string `json:"group_id"`
}

// SwitchGroup godoc
// @Summary Ganti kelompok aktif
// @Description Menerbitkan ulang token dengan scope group yang dipilih
// @Tags Groups
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body switchGroupRequest true "Group tujuan"
// @Success 200 {object} service.LoginResponse
// @Router /auth/switch-group [post]
func (h *GroupHandler) SwitchGroup(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(string)

	var req switchGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	resp, err := h.authService.SwitchGroup(userID, req.GroupID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(resp)
}
