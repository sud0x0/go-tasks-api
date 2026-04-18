package occurrence

import (
	"errors"

	"go-tasks-api/internal/shared/logger"
)

// occurrenceLogger aliases the canonical logger interface from shared/logger.
type occurrenceLogger = logger.Logger

// Sentinel errors for the occurrence domain.
var (
	// ErrDatabase indicates a database operation failure.
	ErrDatabase = errors.New("database error")

	// ErrInvalidInput indicates the input is invalid.
	ErrInvalidInput = errors.New("invalid input")

	// ErrOccurrenceNotFound indicates an occurrence was not found.
	ErrOccurrenceNotFound = errors.New("occurrence not found")

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

	// ErrInvalidDate indicates an invalid date format.
	ErrInvalidDate = errors.New("invalid date: use YYYY-MM-DD format")

	// ErrInvalidDateRange indicates the date range is invalid.
	ErrInvalidDateRange = errors.New("invalid date range: start_date must be before end_date")

	// ErrInvalidAnswerType indicates the answer doesn't match the task's answer type.
	ErrInvalidAnswerType = errors.New("answer type doesn't match task's expected type")

	// ErrInvalidSelectOption indicates the select option is invalid.
	ErrInvalidSelectOption = errors.New("invalid select option for this task")

	// ErrAnswerStringTooLong indicates the answer string exceeds the maximum length.
	ErrAnswerStringTooLong = errors.New("answer_string exceeds maximum of 500 characters")

	// ErrOccurrenceAlreadySuppressed indicates the occurrence is already suppressed.
	ErrOccurrenceAlreadySuppressed = errors.New("occurrence is already suppressed")

	// ErrOccurrenceNotSuppressed indicates the occurrence is not suppressed (for unsuppress).
	ErrOccurrenceNotSuppressed = errors.New("occurrence is not suppressed")

	// ErrOccurrenceIsSuppressed indicates the occurrence is suppressed (for rejecting answers).
	ErrOccurrenceIsSuppressed = errors.New("cannot answer a suppressed occurrence")

	// ErrEmptyIDList indicates no IDs were provided for bulk operations.
	ErrEmptyIDList = errors.New("empty ID list")

	// ErrTooManyIDs indicates too many IDs were provided for bulk operations.
	ErrTooManyIDs = errors.New("too many IDs (max 100)")
)
