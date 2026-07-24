package handler

import (
	"github.com/gofiber/fiber/v2"

	"personal-finance-tracker/internal/service"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login godoc
// @Summary Login user
// @Description Autentikasi user dengan email dan password, mengembalikan access token + refresh token
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body loginRequest true "Email dan Password"
// @Success 200 {object} service.LoginResponse
// @Failure 401 {object} map[string]string
// @Router /auth/login [post]
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	result, err := h.authService.Login(req.Email, req.Password)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh godoc
// @Summary Refresh access token
// @Description Mendapatkan access token baru menggunakan refresh token
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body refreshRequest true "Refresh Token"
// @Success 200 {object} service.LoginResponse
// @Failure 401 {object} map[string]string
// @Router /auth/refresh [post]
func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	result, err := h.authService.Refresh(req.RefreshToken)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}

// Logout godoc
// @Summary Logout user
// @Description Menghapus sesi user (invalidasi refresh token)
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body refreshRequest true "Refresh Token to revoke"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if err := h.authService.Logout(req.RefreshToken); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "logged out successfully",
	})
}
