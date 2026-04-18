package dailylog

import "time"

// DailyLog represents a daily journal entry.
type DailyLog struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	LogDate   time.Time `json:"log_date"`
	Entry     string    `json:"entry"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Validate checks that data retrieved from the database meets expected constraints.
func (d *DailyLog) Validate() error {
	if d.ID == "" {
		return ErrInvalidInput
	}
	if d.UserID == "" {
		return ErrInvalidInput
	}
	if d.LogDate.IsZero() {
		return ErrInvalidInput
	}
	if d.Entry == "" {
		return ErrInvalidInput
	}
	if d.CreatedAt.IsZero() {
		return ErrInvalidInput
	}
	if d.UpdatedAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// CreateRequest is used for creating a daily log.
type CreateRequest struct {
	LogDate string `json:"log_date" validate:"required"`
	Entry   string `json:"entry"    validate:"required,rune_max=10000"`
}

// UpdateRequest is used for updating a daily log.
type UpdateRequest struct {
	Entry string `json:"entry" validate:"required,rune_max=10000"`
}

// BulkDeleteRequest is used for bulk-deleting daily logs.
type BulkDeleteRequest struct {
	IDs []string `json:"ids" validate:"required,min=1,max=100,dive,uuid"`
}

// BulkDeleteResponse is returned by the bulk soft-delete endpoint.
type BulkDeleteResponse struct {
	Requested   int `json:"requested"`
	SoftDeleted int `json:"soft_deleted"`
}

// BulkPermanentDeleteResponse is returned by the bulk permanent-delete endpoint.
type BulkPermanentDeleteResponse struct {
	Requested          int `json:"requested"`
	PermanentlyDeleted int `json:"permanently_deleted"`
}
