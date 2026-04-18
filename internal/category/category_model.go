package category

import (
	"regexp"
	"time"
)

// colourRegex validates lower-case hex colour format.
var colourRegex = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// Category represents a task category.
type Category struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Colour      string    `json:"colour"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Validate checks that data retrieved from the database meets expected constraints.
func (c *Category) Validate() error {
	if c.ID == "" {
		return ErrInvalidInput
	}
	if c.UserID == "" {
		return ErrInvalidInput
	}
	if c.Name == "" {
		return ErrInvalidInput
	}
	if !colourRegex.MatchString(c.Colour) {
		return ErrInvalidInput
	}
	if c.CreatedAt.IsZero() {
		return ErrInvalidInput
	}
	if c.UpdatedAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// CreateRequest is used for creating a category.
type CreateRequest struct {
	Name        string  `json:"name"        validate:"required,rune_max=100"`
	Description *string `json:"description" validate:"omitempty,rune_max=500"`
	Colour      *string `json:"colour"      validate:"omitempty,len=7,startswith=#"`
}

// UpdateRequest is used for updating a category.
type UpdateRequest struct {
	Name        string  `json:"name"        validate:"required,rune_max=100"`
	Description *string `json:"description" validate:"omitempty,rune_max=500"`
	Colour      *string `json:"colour"      validate:"omitempty,len=7,startswith=#"`
}

// BulkDeleteRequest is used for bulk deleting categories.
type BulkDeleteRequest struct {
	IDs []string `json:"ids" validate:"required,min=1,max=100,dive,required"`
}

// BulkDeleteResponse is the response for bulk soft-delete operations.
type BulkDeleteResponse struct {
	Requested   int `json:"requested"`
	SoftDeleted int `json:"soft_deleted"`
}

// BulkPermanentDeleteResponse is the response for bulk permanent-delete operations.
type BulkPermanentDeleteResponse struct {
	Requested          int `json:"requested"`
	PermanentlyDeleted int `json:"permanently_deleted"`
}
