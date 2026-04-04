package category

import (
	"context"
)

// Field limits.
const (
	maxNameLength        = 100
	maxDescriptionLength = 500
)

// categoryService defines the interface for category business logic.
type categoryService interface {
	getCategory(ctx context.Context, id, userID string) (Category, error)
	getCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error)
	createCategory(ctx context.Context, userID string, req CreateRequest) (Category, error)
	updateCategory(ctx context.Context, id, userID string, req UpdateRequest) (Category, error)
	deleteCategory(ctx context.Context, id, userID string) error
}

// defaultCategoryService implements categoryService.
type defaultCategoryService struct {
	repo categoryRepository
}

// NewCategoryService creates a new categoryService.
func NewCategoryService(repo categoryRepository, _ categoryLogger) *defaultCategoryService {
	return &defaultCategoryService{repo: repo}
}

func (s *defaultCategoryService) getCategory(ctx context.Context, id, userID string) (Category, error) {
	if id == "" || userID == "" {
		return Category{}, ErrMissingParameters
	}
	return s.repo.getCategory(ctx, id, userID)
}

func (s *defaultCategoryService) getCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error) {
	if userID == "" {
		return nil, ErrMissingParameters
	}

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	return s.repo.getCategories(ctx, userID, limit, offset)
}

func (s *defaultCategoryService) createCategory(ctx context.Context, userID string, req CreateRequest) (Category, error) {
	if userID == "" {
		return Category{}, ErrMissingParameters
	}

	if len(req.Name) > maxNameLength {
		return Category{}, ErrNameTooLong
	}
	if req.Description != nil && len(*req.Description) > maxDescriptionLength {
		return Category{}, ErrDescriptionTooLong
	}

	return s.repo.createCategory(ctx, userID, req.Name, req.Description)
}

func (s *defaultCategoryService) updateCategory(ctx context.Context, id, userID string, req UpdateRequest) (Category, error) {
	if id == "" || userID == "" {
		return Category{}, ErrMissingParameters
	}

	if len(req.Name) > maxNameLength {
		return Category{}, ErrNameTooLong
	}
	if req.Description != nil && len(*req.Description) > maxDescriptionLength {
		return Category{}, ErrDescriptionTooLong
	}

	return s.repo.updateCategory(ctx, id, userID, req.Name, req.Description)
}

func (s *defaultCategoryService) deleteCategory(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrMissingParameters
	}

	// Check existence and ownership first
	_, err := s.repo.getCategory(ctx, id, userID)
	if err != nil {
		return err // returns ErrCategoryNotFound if not owned by this user
	}

	// Only then check active tasks
	hasActiveTasks, err := s.repo.hasActiveTasks(ctx, id)
	if err != nil {
		return err
	}
	if hasActiveTasks {
		return ErrCategoryInUse
	}

	return s.repo.deleteCategory(ctx, id, userID)
}
