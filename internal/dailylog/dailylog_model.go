package dailylog

import "time"

// DailyLog represents a daily journal entry.
type DailyLog struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	LogDate   time.Time `json:"log_date"`
	Entry     string    `json:"entry"`
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
	Entry   string `json:"entry"    validate:"required,max=10000"`
}

// UpdateRequest is used for updating a daily log.
type UpdateRequest struct {
	Entry string `json:"entry" validate:"required,max=10000"`
}
