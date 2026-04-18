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

	// ErrNameTooLong indicates the name exceeds the maximum length.
	ErrNameTooLong = errors.New("name exceeds maximum of 100 characters")

	// ErrDescriptionTooLong indicates the description exceeds the maximum length.
	ErrDescriptionTooLong = errors.New("description exceeds maximum of 500 characters")

	// ErrDuplicateName indicates a category with this name already exists for the user.
	ErrDuplicateName = errors.New("category with this name already exists")

	// ErrInvalidColour indicates the colour format is invalid.
	ErrInvalidColour = errors.New("invalid colour: must be in the form #RRGGBB")

	// ErrTooManyIDs indicates too many IDs were provided for bulk operation.
	ErrTooManyIDs = errors.New("too many IDs: maximum 100 allowed")

	// ErrEmptyIDList indicates an empty ID list was provided.
	ErrEmptyIDList = errors.New("at least one ID is required")

	// ErrCategoryAlreadyActive indicates the category is already active.
	ErrCategoryAlreadyActive = errors.New("category is already active")

	// ErrCategoryAlreadyInactive indicates the category is already inactive.
	ErrCategoryAlreadyInactive = errors.New("category is already inactive")

	// ErrReactivateNameCollision indicates reactivation would cause a duplicate name.
	ErrReactivateNameCollision = errors.New("cannot reactivate: another active category has this name")

	// ErrCannotPermanentDeleteActive indicates an attempt to permanently delete an active category.
	ErrCannotPermanentDeleteActive = errors.New("cannot permanently delete an active category; deactivate it first")

	// ErrCategoryHasActiveTasks indicates the category cannot be deleted because it has active tasks.
	ErrCategoryHasActiveTasks = errors.New("category has active tasks")
)
