package category

import "time"

// Category represents a task category.
type Category struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
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
	Name        string  `json:"name"        validate:"required,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
}

// UpdateRequest is used for updating a category.
type UpdateRequest struct {
	Name        string  `json:"name"        validate:"required,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
}
