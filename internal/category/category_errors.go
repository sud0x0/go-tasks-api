package category

import (
	"errors"

	"go-tasks-api/internal/shared/logger"
)

// categoryLogger aliases the canonical logger interface from shared/logger.
type categoryLogger = logger.Logger

// Sentinel errors for the category domain.
var (
	// ErrDatabase indicates a database operation failure.
	ErrDatabase = errors.New("database error")

	// ErrInvalidInput indicates the input is invalid.
	ErrInvalidInput = errors.New("invalid input")

	// ErrCategoryNotFound indicates a category was not found.
	ErrCategoryNotFound = errors.New("category not found")

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

	// ErrCategoryInUse indicates the category is being used by active tasks.
	ErrCategoryInUse = errors.New("category is in use by active tasks")

	// ErrNameTooLong indicates the name exceeds the maximum length.
	ErrNameTooLong = errors.New("name exceeds maximum of 100 characters")

	// ErrDescriptionTooLong indicates the description exceeds the maximum length.
	ErrDescriptionTooLong = errors.New("description exceeds maximum of 500 characters")
)
