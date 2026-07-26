package service

import (
	"errors"
	"strings"

	"personal-finance-tracker/internal/repository"
)

type CategoryService struct {
	repo *repository.CategoryRepository
}

func NewCategoryService(repo *repository.CategoryRepository) *CategoryService {
	return &CategoryService{repo: repo}
}

func (s *CategoryService) List(groupID, categoryType string) ([]repository.Category, error) {
	if categoryType != "" {
		categoryType = strings.ToLower(categoryType)
		if categoryType != "income" && categoryType != "expense" {
			return nil, errors.New("type must be 'income' or 'expense'")
		}
	}
	return s.repo.FindByGroupID(groupID, categoryType)
}

type CreateCategoryInput struct {
	GroupID string
	UserID  string
	Name    string
	Type    string
	Icon    string
}

func (s *CategoryService) Create(input CreateCategoryInput) (*repository.Category, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return nil, errors.New("name is required")
	}

	input.Type = strings.ToLower(input.Type)
	if input.Type != "income" && input.Type != "expense" {
		return nil, errors.New("type must be 'income' or 'expense'")
	}

	category := &repository.Category{
		GroupID: input.GroupID,
		UserID:  input.UserID,
		Name:    input.Name,
		Type:    input.Type,
		Icon:    input.Icon,
	}

	if err := s.repo.Create(category); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, errors.New("category with this name already exists")
		}
		return nil, err
	}

	return category, nil
}
