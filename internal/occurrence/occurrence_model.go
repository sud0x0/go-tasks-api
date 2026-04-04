package occurrence

import (
	"time"

	"go-tasks-api/internal/task"
)

// TaskOccurrence represents a single occurrence of a task on a specific day and time.
type TaskOccurrence struct {
	ID             string     `json:"id"`
	TaskID         string     `json:"task_id"`
	ScheduleID     string     `json:"schedule_id"`
	UserID         string     `json:"user_id"`
	OccurrenceDate time.Time  `json:"occurrence_date"`
	ScheduledTime  *time.Time `json:"scheduled_time,omitempty"`
	IsSuppressed   bool       `json:"is_suppressed"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Validate checks that data retrieved from the database meets expected constraints.
func (o *TaskOccurrence) Validate() error {
	if o.ID == "" {
		return ErrInvalidInput
	}
	if o.TaskID == "" {
		return ErrInvalidInput
	}
	if o.ScheduleID == "" {
		return ErrInvalidInput
	}
	if o.UserID == "" {
		return ErrInvalidInput
	}
	if o.OccurrenceDate.IsZero() {
		return ErrInvalidInput
	}
	if o.CreatedAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// TaskAnswer represents an answer to a task occurrence.
type TaskAnswer struct {
	ID            string    `json:"id"`
	OccurrenceID  string    `json:"occurrence_id"`
	UserID        string    `json:"user_id"`
	AnswerString  *string   `json:"answer_string,omitempty"`
	AnswerInteger *int      `json:"answer_integer,omitempty"`
	AnswerBoolean *bool     `json:"answer_boolean,omitempty"`
	AnswerSelect  *string   `json:"answer_select,omitempty"`
	AnsweredAt    time.Time `json:"answered_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Validate checks that data retrieved from the database meets expected constraints.
func (a *TaskAnswer) Validate() error {
	if a.ID == "" {
		return ErrInvalidInput
	}
	if a.OccurrenceID == "" {
		return ErrInvalidInput
	}
	if a.UserID == "" {
		return ErrInvalidInput
	}
	if a.AnsweredAt.IsZero() {
		return ErrInvalidInput
	}
	if a.CreatedAt.IsZero() {
		return ErrInvalidInput
	}
	if a.UpdatedAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// WithDetails represents an occurrence with its task details and answer.
type WithDetails struct {
	Occurrence    TaskOccurrence      `json:"occurrence"`
	Task          task.Task           `json:"task"`
	SelectOptions []task.SelectOption `json:"select_options,omitempty"`
	Answer        *TaskAnswer         `json:"answer,omitempty"`
}

// AnswerRequest is used for submitting an answer.
type AnswerRequest struct {
	AnswerString  *string `json:"answer_string"  validate:"omitempty,max=500"`
	AnswerInteger *int    `json:"answer_integer"`
	AnswerBoolean *bool   `json:"answer_boolean"`
	AnswerSelect  *string `json:"answer_select"`
}
