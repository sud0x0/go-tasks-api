package task

import (
	"time"
)

// AnswerType represents the type of answer a task expects.
type AnswerType string

// AnswerType constants define the valid answer types for tasks.
const (
	AnswerTypeString  AnswerType = "string"
	AnswerTypeInteger AnswerType = "integer"
	AnswerTypeBoolean AnswerType = "boolean"
	AnswerTypeSelect  AnswerType = "select"
)

// ValidAnswerTypes is the list of valid answer types.
var ValidAnswerTypes = []AnswerType{AnswerTypeString, AnswerTypeInteger, AnswerTypeBoolean, AnswerTypeSelect}

// IsValidAnswerType checks if the given answer type is valid.
func IsValidAnswerType(at string) bool {
	for _, valid := range ValidAnswerTypes {
		if string(valid) == at {
			return true
		}
	}
	return false
}

// RecurrenceType represents the type of recurrence for a task schedule.
type RecurrenceType string

// RecurrenceType constants define how a task repeats.
const (
	RecurrenceOnce           RecurrenceType = "once"
	RecurrenceDaily          RecurrenceType = "daily"
	RecurrenceEveryNDays     RecurrenceType = "every_n_days"
	RecurrenceWeekly         RecurrenceType = "weekly"
	RecurrenceEveryNWeeks    RecurrenceType = "every_n_weeks"
	RecurrenceMonthlyDate    RecurrenceType = "monthly_date"
	RecurrenceMonthlyWeekday RecurrenceType = "monthly_weekday"
	RecurrenceYearly         RecurrenceType = "yearly"
)

// ValidRecurrenceTypes is the list of valid recurrence types.
var ValidRecurrenceTypes = []RecurrenceType{
	RecurrenceOnce, RecurrenceDaily, RecurrenceEveryNDays, RecurrenceWeekly,
	RecurrenceEveryNWeeks, RecurrenceMonthlyDate, RecurrenceMonthlyWeekday, RecurrenceYearly,
}

// IsValidRecurrenceType checks if the given recurrence type is valid.
func IsValidRecurrenceType(rt string) bool {
	for _, valid := range ValidRecurrenceTypes {
		if string(valid) == rt {
			return true
		}
	}
	return false
}

// EndType represents the end condition for a task schedule.
type EndType string

// EndType constants define when a recurring task stops.
const (
	EndTypeNever  EndType = "never"
	EndTypeOnDate EndType = "on_date"
	EndTypeAfterN EndType = "after_n"
)

// Task represents a task.
type Task struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	CategoryID  string     `json:"category_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	AnswerType  AnswerType `json:"answer_type"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Validate checks that data retrieved from the database meets expected constraints.
func (t *Task) Validate() error {
	if t.ID == "" {
		return ErrInvalidInput
	}
	if t.UserID == "" {
		return ErrInvalidInput
	}
	if t.CategoryID == "" {
		return ErrInvalidInput
	}
	if t.Name == "" {
		return ErrInvalidInput
	}
	if !IsValidAnswerType(string(t.AnswerType)) {
		return ErrInvalidInput
	}
	if t.CreatedAt.IsZero() {
		return ErrInvalidInput
	}
	if t.UpdatedAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// SelectOption represents a select option for a task.
type SelectOption struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Value     string    `json:"value"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// Validate checks that data retrieved from the database meets expected constraints.
func (o *SelectOption) Validate() error {
	if o.ID == "" {
		return ErrInvalidInput
	}
	if o.TaskID == "" {
		return ErrInvalidInput
	}
	if o.Value == "" {
		return ErrInvalidInput
	}
	if o.CreatedAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// Schedule represents a schedule for a task.
type Schedule struct {
	ID                 string         `json:"id"`
	TaskID             string         `json:"task_id"`
	RecurrenceType     RecurrenceType `json:"recurrence_type"`
	RecurrenceInterval *int           `json:"recurrence_interval,omitempty"`
	DaysOfWeek         []int64        `json:"days_of_week,omitempty"`
	MonthDay           *int           `json:"month_day,omitempty"`
	MonthWeek          *int           `json:"month_week,omitempty"`
	MonthWeekday       *int           `json:"month_weekday,omitempty"`
	MonthOfYear        *int           `json:"month_of_year,omitempty"`
	ScheduledTimes     []string       `json:"scheduled_times,omitempty"`
	StartDate          time.Time      `json:"start_date"`
	EndType            EndType        `json:"end_type"`
	EndDate            *time.Time     `json:"end_date,omitempty"`
	EndAfterN          *int           `json:"end_after_n,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
}

// Validate checks that data retrieved from the database meets expected constraints.
func (s *Schedule) Validate() error {
	if s.ID == "" {
		return ErrInvalidInput
	}
	if s.TaskID == "" {
		return ErrInvalidInput
	}
	if !IsValidRecurrenceType(string(s.RecurrenceType)) {
		return ErrInvalidInput
	}
	if s.StartDate.IsZero() {
		return ErrInvalidInput
	}
	if s.CreatedAt.IsZero() {
		return ErrInvalidInput
	}
	return nil
}

// WithDetails represents a task with its schedule and select options.
type WithDetails struct {
	Task          Task           `json:"task"`
	Schedule      *Schedule      `json:"schedule,omitempty"`
	SelectOptions []SelectOption `json:"select_options,omitempty"`
}

// CreateTaskRequest is used for creating a task.
type CreateTaskRequest struct {
	CategoryID    string                `json:"category_id"    validate:"required"`
	Name          string                `json:"name"           validate:"required,max=200"`
	Description   *string               `json:"description"    validate:"omitempty,max=1000"`
	AnswerType    string                `json:"answer_type"    validate:"required"`
	Schedule      ScheduleRequest       `json:"schedule"       validate:"required"`
	SelectOptions []SelectOptionRequest `json:"select_options" validate:"omitempty,min=2,max=10,dive"`
}

// ScheduleRequest is used for creating a schedule.
type ScheduleRequest struct {
	RecurrenceType     string   `json:"recurrence_type"      validate:"required"`
	RecurrenceInterval *int     `json:"recurrence_interval"  validate:"omitempty,min=1"`
	DaysOfWeek         []int    `json:"days_of_week"         validate:"omitempty,dive,min=0,max=6"`
	MonthDay           *int     `json:"month_day"            validate:"omitempty,min=1,max=31"`
	MonthWeek          *int     `json:"month_week"           validate:"omitempty,min=1,max=5"`
	MonthWeekday       *int     `json:"month_weekday"        validate:"omitempty,min=0,max=6"`
	MonthOfYear        *int     `json:"month_of_year"        validate:"omitempty,min=1,max=12"`
	ScheduledTimes     []string `json:"scheduled_times"      validate:"omitempty,dive"`
	StartDate          string   `json:"start_date"           validate:"required"`
	EndType            string   `json:"end_type"             validate:"omitempty"`
	EndDate            *string  `json:"end_date"             validate:"omitempty"`
	EndAfterN          *int     `json:"end_after_n"          validate:"omitempty,min=1"`
}

// SelectOptionRequest is used for creating a select option.
type SelectOptionRequest struct {
	Value string `json:"value" validate:"required,max=100"`
}

// UpdateTaskRequest is used for updating a task.
type UpdateTaskRequest struct {
	Name        string  `json:"name"        validate:"required,max=200"`
	Description *string `json:"description" validate:"omitempty,max=1000"`
}
