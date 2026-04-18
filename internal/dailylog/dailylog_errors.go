package dailylog

import (
	"errors"

	"go-tasks-api/internal/shared/logger"
)

// dailylogLogger aliases the canonical logger interface from shared/logger.
type dailylogLogger = logger.Logger

// Sentinel errors for the dailylog domain.
var (
	// ErrDatabase indicates a database operation failure.
	ErrDatabase = errors.New("database error")

	// ErrInvalidInput indicates the input is invalid.
	ErrInvalidInput = errors.New("invalid input")

	// ErrDailyLogNotFound indicates a daily log was not found.
	ErrDailyLogNotFound = errors.New("daily log not found")

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

	// ErrDailyLogExists indicates a daily log already exists for this date.
	ErrDailyLogExists = errors.New("daily log already exists for this date")

	// ErrEntryTooLong indicates the entry exceeds the maximum length.
	ErrEntryTooLong = errors.New("entry exceeds maximum of 10000 characters")

	// ErrTooManyIDs indicates too many IDs were provided in a bulk operation.
	ErrTooManyIDs = errors.New("too many ids: maximum 100 per request")

	// ErrEmptyIDList indicates the IDs list must not be empty.
	ErrEmptyIDList = errors.New("ids list must not be empty")

	// ErrDailyLogAlreadyActive indicates the daily log is already active.
	ErrDailyLogAlreadyActive = errors.New("daily log is already active")

	// ErrDailyLogAlreadyInactive indicates the daily log is already inactive.
	ErrDailyLogAlreadyInactive = errors.New("daily log is already inactive")

	// ErrCannotPermanentDeleteActiveDailyLog indicates an attempt to permanently delete an active daily log.
	ErrCannotPermanentDeleteActiveDailyLog = errors.New("cannot permanently delete an active daily log; deactivate it first")
)
