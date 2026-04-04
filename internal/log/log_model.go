package log

import "time"

// Log represents a log entry.
type Log struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	DateAndTime time.Time `json:"date_and_time"`
	Log         string    `json:"log"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Validate checks that data retrieved from the database meets expected constraints.
// This is defence-in-depth: even trusted database values are validated before use.
func (l *Log) Validate() error {
	if l.ID == "" {
		return ErrInvalidInput
	}
	if l.UserID == "" {
		return ErrInvalidInput
	}
	if l.DateAndTime.IsZero() {
		return ErrInvalidInput
	}
	if l.CreatedAt.IsZero() {
		return ErrInvalidInput
	}
	if l.UpdatedAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// Request is used for creating and updating log entries.
type Request struct {
	DateAndTime string `json:"date_and_time" validate:"required"`
	Log         string `json:"log"           validate:"required"`
}
