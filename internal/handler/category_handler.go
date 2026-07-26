package handler

import (
	"github.com/gofiber/fiber/v2"

	"personal-finance-tracker/internal/repository"
	"personal-finance-tracker/internal/service"
)

type CategoryHandler struct {
	categoryService *service.CategoryService
}

func NewCategoryHandler(categoryService *service.CategoryService) *CategoryHandler {
	return &CategoryHandler{categoryService: categoryService}
}

// ListCategories godoc
// @Summary Daftar kategori
// @Description Mendapatkan daftar kategori transaksi milik user yang login
// @Tags Categories
// @Produce json
// @Security BearerAuth
// @Param type query string false "Filter by type: income / expense"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]string
// @Router /categories [get]
func (h *CategoryHandler) List(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	categoryType := c.Query("type")

	categories, err := h.categoryService.List(groupID, categoryType)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Pastikan hasil JSON tetap [] bukan null saat kategori kosong
	if categories == nil {
		categories = []repository.Category{}
	}

	return c.JSON(fiber.Map{
		"data": categories,
	})
}

type createCategoryRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Icon string `json:"icon"`
}

// CreateCategory godoc
// @Summary Buat kategori baru
// @Description Membuat kategori transaksi custom (P1)
// @Tags Categories
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createCategoryRequest true "Data kategori"
// @Success 201 {object} repository.Category
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Router /categories [post]
func (h *CategoryHandler) Create(c *fiber.Ctx) error {
	groupID := c.Locals("group_id").(string)
	userID := c.Locals("user_id").(string)

	var req createCategoryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	category, err := h.categoryService.Create(service.CreateCategoryInput{
		GroupID: groupID,
		UserID:  userID,
		Name:    req.Name,
		Type:    req.Type,
		Icon:    req.Icon,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(category)
}
