package auth

import (
	"errors"

	"go-tasks-api/internal/shared/logger"
)

// authLogger aliases the canonical logger interface from shared/logger.
type authLogger = logger.Logger

// Sentinel errors for the auth domain.
var (
	// ErrDatabase indicates a database operation failure.
	ErrDatabase = errors.New("database error")

	// ErrInvalidInput indicates the input is invalid.
	ErrInvalidInput = errors.New("invalid input")

	// ErrUserNotFound indicates a user was not found.
	ErrUserNotFound = errors.New("user not found")

	// ErrUserExists indicates a user already exists.
	ErrUserExists = errors.New("username already exists")

	// ErrInvalidCredentials indicates invalid login credentials.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrInvalidToken indicates an invalid or expired token.
	ErrInvalidToken = errors.New("invalid or expired token")

	// ErrTokenRevoked indicates the token has been revoked.
	ErrTokenRevoked = errors.New("token has been revoked")

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

	// ErrUsernameTooLong indicates the username exceeds the maximum length.
	ErrUsernameTooLong = errors.New("username exceeds maximum of 50 characters")

	// ErrPasswordTooLong indicates the password exceeds the maximum length (128 code points after NFKC).
	ErrPasswordTooLong = errors.New("password exceeds maximum of 128 characters")

	// ErrPasswordTooShort indicates the password is too short (minimum 8 code points after NFKC).
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")

	// ErrPasswordInvalidChars indicates the password contains invalid control characters.
	ErrPasswordInvalidChars = errors.New("password contains invalid control characters")

	// ErrInvalidUsername indicates the username is invalid (e.g., empty or whitespace-only).
	ErrInvalidUsername = errors.New("username cannot be empty or whitespace")

	// ErrValkey indicates a Valkey operation failure.
	ErrValkey = errors.New("valkey error")

	// ErrTokenOwnershipMismatch indicates the token does not belong to the claimed user.
	ErrTokenOwnershipMismatch = errors.New("token ownership mismatch")

	// ErrValkeyUnavailable indicates the Valkey service is unavailable.
	ErrValkeyUnavailable = errors.New("valkey service unavailable")
)
