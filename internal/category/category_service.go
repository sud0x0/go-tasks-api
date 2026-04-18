package category

import (
	"context"
	"regexp"
	"strings"

	"go-tasks-api/internal/shared"

	"github.com/google/uuid"
)

// Field limits.
const (
	maxNameLength        = 100
	maxDescriptionLength = 500
	maxBulkIDs           = 100
)

// defaultColour is used when no colour is provided on create.
const defaultColour = "#808080"

// colourInputRegex validates hex colour format (accepts both cases for input validation).
var colourInputRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// categoryService defines the interface for category business logic.
type categoryService interface {
	getCategory(ctx context.Context, id, userID string) (Category, error)
	getCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error)
	getInactiveCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error)
	createCategory(ctx context.Context, userID string, req CreateRequest) (Category, error)
	updateCategory(ctx context.Context, id, userID string, req UpdateRequest) (Category, error)
	deleteCategory(ctx context.Context, id, userID string) error
	permanentDeleteCategory(ctx context.Context, id, userID string) error
	bulkDeleteCategories(ctx context.Context, userID string, ids []string) (int, int, error)
	bulkPermanentDeleteCategories(ctx context.Context, userID string, ids []string) (int, int, error)
	reactivateCategory(ctx context.Context, id, userID string) (Category, error)
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

func (s *defaultCategoryService) getInactiveCategories(ctx context.Context, userID string, limit, offset int) ([]Category, error) {
	if userID == "" {
		return nil, ErrMissingParameters
	}

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	return s.repo.getInactiveCategories(ctx, userID, limit, offset)
}

func (s *defaultCategoryService) createCategory(ctx context.Context, userID string, req CreateRequest) (Category, error) {
	if userID == "" {
		return Category{}, ErrMissingParameters
	}

	// Trim whitespace from name
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Category{}, ErrInvalidInput
	}

	if shared.RuneCountLen(name) > maxNameLength {
		return Category{}, ErrNameTooLong
	}
	if req.Description != nil && shared.RuneCountLen(*req.Description) > maxDescriptionLength {
		return Category{}, ErrDescriptionTooLong
	}

	// Handle colour: use default if nil, otherwise validate and lower-case
	colour := defaultColour
	if req.Colour != nil {
		if !colourInputRegex.MatchString(*req.Colour) {
			return Category{}, ErrInvalidColour
		}
		colour = strings.ToLower(*req.Colour)
	}

	return s.repo.createCategory(ctx, userID, name, req.Description, colour)
}

func (s *defaultCategoryService) updateCategory(ctx context.Context, id, userID string, req UpdateRequest) (Category, error) {
	if id == "" || userID == "" {
		return Category{}, ErrMissingParameters
	}

	// Check existence and active state before updating (fix B: update on inactive returns 409)
	isActive, err := s.repo.getCategoryIsActive(ctx, id, userID)
	if err != nil {
		return Category{}, err // returns ErrCategoryNotFound if not found
	}
	if !isActive {
		return Category{}, ErrCategoryAlreadyInactive
	}

	// Trim whitespace from name
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Category{}, ErrInvalidInput
	}

	if shared.RuneCountLen(name) > maxNameLength {
		return Category{}, ErrNameTooLong
	}
	if req.Description != nil && shared.RuneCountLen(*req.Description) > maxDescriptionLength {
		return Category{}, ErrDescriptionTooLong
	}

	// Handle colour: nil means keep existing, otherwise validate and lower-case
	var colour *string
	if req.Colour != nil {
		if !colourInputRegex.MatchString(*req.Colour) {
			return Category{}, ErrInvalidColour
		}
		lc := strings.ToLower(*req.Colour)
		colour = &lc
	}

	return s.repo.updateCategory(ctx, id, userID, name, req.Description, colour)
}

// deleteCategory performs soft-delete only.
// Returns ErrCategoryNotFound if the category does not exist.
// Returns ErrCategoryAlreadyInactive (409) if the category is already inactive.
// Returns ErrCategoryHasActiveTasks (409) if the category has active tasks.
func (s *defaultCategoryService) deleteCategory(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrMissingParameters
	}

	// Check existence and get current status
	isActive, err := s.repo.getCategoryIsActive(ctx, id, userID)
	if err != nil {
		return err // returns ErrCategoryNotFound if not found
	}

	if !isActive {
		return ErrCategoryAlreadyInactive
	}

	// Check if category has active tasks
	hasTasks, err := s.repo.hasActiveTasks(ctx, id, userID)
	if err != nil {
		return err
	}
	if hasTasks {
		return ErrCategoryHasActiveTasks
	}

	// Soft delete: deactivate the category
	return s.repo.deactivateCategory(ctx, id, userID)
}

// permanentDeleteCategory performs hard-delete on an inactive category.
// Returns ErrCategoryNotFound if the category does not exist.
// Returns ErrCannotPermanentDeleteActive (409) if the category is still active.
func (s *defaultCategoryService) permanentDeleteCategory(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrMissingParameters
	}

	// Check existence and get current status
	isActive, err := s.repo.getCategoryIsActive(ctx, id, userID)
	if err != nil {
		return err // returns ErrCategoryNotFound if not found
	}

	if isActive {
		return ErrCannotPermanentDeleteActive
	}

	// Hard delete: category is inactive
	return s.repo.hardDeleteCategory(ctx, id, userID)
}

// bulkDeleteCategories performs bulk soft-delete only.
// Inactive IDs in the list are ignored (not hard-deleted).
// Returns (requested, softDeleted, error) where requested is the pre-dedup input length.
func (s *defaultCategoryService) bulkDeleteCategories(ctx context.Context, userID string, ids []string) (int, int, error) {
	requested := len(ids)
	if userID == "" {
		return 0, 0, ErrMissingParameters
	}
	if requested == 0 {
		return 0, 0, ErrEmptyIDList
	}
	if requested > maxBulkIDs {
		return 0, 0, ErrTooManyIDs
	}

	// Validate and deduplicate IDs
	seen := make(map[string]struct{}, len(ids))
	validIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return 0, 0, ErrInvalidInput
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			validIDs = append(validIDs, id)
		}
	}

	// Soft delete active categories only
	softDeleted, err := s.repo.bulkDeactivateCategories(ctx, userID, validIDs)
	if err != nil {
		return requested, 0, err
	}

	return requested, softDeleted, nil
}

// bulkPermanentDeleteCategories performs bulk hard-delete on inactive categories only.
// Active IDs in the list are ignored.
// Returns (requested, permanentlyDeleted, error) where requested is the pre-dedup input length.
func (s *defaultCategoryService) bulkPermanentDeleteCategories(ctx context.Context, userID string, ids []string) (int, int, error) {
	requested := len(ids)
	if userID == "" {
		return 0, 0, ErrMissingParameters
	}
	if requested == 0 {
		return 0, 0, ErrEmptyIDList
	}
	if requested > maxBulkIDs {
		return 0, 0, ErrTooManyIDs
	}

	// Validate and deduplicate IDs
	seen := make(map[string]struct{}, len(ids))
	validIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return 0, 0, ErrInvalidInput
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			validIDs = append(validIDs, id)
		}
	}

	// Hard delete inactive categories only
	permanentlyDeleted, err := s.repo.bulkHardDeleteCategories(ctx, userID, validIDs)
	if err != nil {
		return requested, 0, err
	}

	return requested, permanentlyDeleted, nil
}

func (s *defaultCategoryService) reactivateCategory(ctx context.Context, id, userID string) (Category, error) {
	if id == "" || userID == "" {
		return Category{}, ErrMissingParameters
	}

	return s.repo.reactivateCategory(ctx, id, userID)
}
