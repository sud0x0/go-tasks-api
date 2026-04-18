package task

import (
	"errors"

	"go-tasks-api/internal/shared/logger"
)

// taskLogger aliases the canonical logger interface from shared/logger.
type taskLogger = logger.Logger

// Sentinel errors for the task domain.
var (
	// ErrDatabase indicates a database operation failure.
	ErrDatabase = errors.New("database error")

	// ErrInvalidInput indicates the input is invalid.
	ErrInvalidInput = errors.New("invalid input")

	// ErrTaskNotFound indicates a task was not found.
	ErrTaskNotFound = errors.New("task not found")

	// ErrMissingParameters indicates required parameters are missing.
	ErrMissingParameters = errors.New("missing required parameters")

	// ErrValidation indicates a validation error.
	ErrValidation = errors.New("validation error")

	// ErrInvalidReqBody indicates the request body is invalid.
	ErrInvalidReqBody = errors.New("invalid request body")

	// ErrInternalServer indicates an internal server error.
	ErrInternalServer = errors.New("internal server error")

	// ErrUnauthorised indicates an unauthorised access attempt.
	ErrUnauthorised = errors.New("unauthorised access")

	// ErrInvalidAnswerType indicates an invalid answer type.
	ErrInvalidAnswerType = errors.New("invalid answer type: must be 'string', 'integer', 'boolean', or 'select'")

	// ErrInvalidRecurrenceType indicates an invalid recurrence type.
	ErrInvalidRecurrenceType = errors.New("invalid recurrence type")

	// ErrMissingSelectOptions indicates select options are required but missing.
	ErrMissingSelectOptions = errors.New("select options required for answer_type 'select' (2-10 options)")

	// ErrTooManySelectOptions indicates too many select options were provided.
	ErrTooManySelectOptions = errors.New("too many select options (maximum 10)")

	// ErrTooFewSelectOptions indicates too few select options were provided.
	ErrTooFewSelectOptions = errors.New("too few select options (minimum 2)")

	// ErrInvalidSchedule indicates the schedule configuration is invalid.
	ErrInvalidSchedule = errors.New("invalid schedule configuration")

	// ErrCategoryNotFound indicates the category was not found.
	ErrCategoryNotFound = errors.New("category not found")

	// ErrNameTooLong indicates the name exceeds the maximum length.
	ErrNameTooLong = errors.New("name exceeds maximum of 200 characters")

	// ErrDescriptionTooLong indicates the description exceeds the maximum length.
	ErrDescriptionTooLong = errors.New("description exceeds maximum of 1000 characters")

	// ErrOptionValueTooLong indicates the option value exceeds the maximum length.
	ErrOptionValueTooLong = errors.New("option value exceeds maximum of 100 characters")

	// ErrInvalidStartDate indicates the start date is invalid.
	ErrInvalidStartDate = errors.New("invalid start_date: use YYYY-MM-DD format")

	// ErrInvalidEndDate indicates the end date is invalid.
	ErrInvalidEndDate = errors.New("invalid end_date: use YYYY-MM-DD format")

	// ErrInvalidScheduledTime indicates a scheduled time is invalid.
	ErrInvalidScheduledTime = errors.New("invalid scheduled_time: use HH:MM format")

	// ErrMissingRecurrenceInterval indicates recurrence_interval is required but missing.
	ErrMissingRecurrenceInterval = errors.New("recurrence_interval required for 'every_n_days' or 'every_n_weeks'")

	// ErrMissingDaysOfWeek indicates days_of_week is required but missing.
	ErrMissingDaysOfWeek = errors.New("days_of_week required for 'weekly' or 'every_n_weeks'")

	// ErrMissingMonthDay indicates month_day is required but missing.
	ErrMissingMonthDay = errors.New("month_day required for 'monthly_date' or 'yearly'")

	// ErrMissingMonthlyWeekdayFields indicates month_week and month_weekday are required but missing.
	ErrMissingMonthlyWeekdayFields = errors.New("month_week and month_weekday required for 'monthly_weekday'")

	// ErrMissingMonthOfYear indicates month_of_year is required but missing.
	ErrMissingMonthOfYear = errors.New("month_of_year required for 'yearly'")

	// ErrMissingEndDate indicates end_date is required but missing.
	ErrMissingEndDate = errors.New("end_date required for end_type 'on_date'")

	// ErrMissingEndAfterN indicates end_after_n is required but missing.
	ErrMissingEndAfterN = errors.New("end_after_n required for end_type 'after_n'")

	// ErrTooManyIDs indicates too many IDs were provided for bulk operation.
	ErrTooManyIDs = errors.New("too many IDs: maximum 100 allowed")

	// ErrEmptyIDList indicates an empty ID list was provided.
	ErrEmptyIDList = errors.New("at least one ID is required")

	// ErrTaskAlreadyActive indicates the task is already active.
	ErrTaskAlreadyActive = errors.New("task is already active")

	// ErrTaskAlreadyInactive indicates the task is already inactive.
	ErrTaskAlreadyInactive = errors.New("task is already inactive")

	// ErrCategoryInactive indicates the task's category is inactive.
	ErrCategoryInactive = errors.New("cannot reactivate: category is inactive")

	// ErrCannotPermanentDeleteActiveTask indicates an attempt to permanently delete an active task.
	ErrCannotPermanentDeleteActiveTask = errors.New("cannot permanently delete an active task; deactivate it first")
)
