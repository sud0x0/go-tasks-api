package log

import (
	"errors"
	"fmt"

	"go-tasks-api/internal/shared"
	"go-tasks-api/internal/shared/logger"
)

// logLogger aliases the canonical logger interface from shared/logger.
type logLogger = logger.Logger

// ErrDatabase indicates a database operation failure.
var ErrDatabase = errors.New("database error")

// ErrUnauthorised indicates an unauthorised access attempt.
var ErrUnauthorised = errors.New("unauthorised access")

// ErrMissingParameters indicates required parameters are missing.
var ErrMissingParameters = errors.New("missing required parameters")

// ErrLogNotFound indicates a log entry was not found.
var ErrLogNotFound = errors.New("log not found")

// ErrInvalidInput indicates the input is invalid.
var ErrInvalidInput = errors.New("invalid input")

// ErrInvalidDateTime indicates an invalid date/time format.
var ErrInvalidDateTime = errors.New("invalid date_and_time: use RFC3339 e.g. 2006-01-02T15:04:05Z")

// ErrDateTimeOutOfRange indicates the date/time year is outside the supported range.
var ErrDateTimeOutOfRange = errors.New("date_and_time year out of supported range")

// ErrValidation indicates a validation error.
var ErrValidation = errors.New("validation error")

// ErrInvalidReqBody indicates the request body is invalid.
var ErrInvalidReqBody = errors.New("invalid request body")

// ErrInternalServer indicates an internal server error.
var ErrInternalServer = errors.New("internal server error")

// NewLogTooLongError creates a structured limit exceeded error.
func NewLogTooLongError(currentLen int) *shared.LimitExceededError {
	return shared.NewLimitExceededError(
		fmt.Sprintf("log exceeds maximum of %d characters", shared.LogMaxChars),
		shared.LogMaxChars,
		currentLen,
	)
}
