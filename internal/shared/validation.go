package shared

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// ErrInvalidPagination indicates pagination parameters are invalid.
var ErrInvalidPagination = errors.New("invalid pagination parameters")

// Pagination limits.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
	MaxPageOffset   = 10000
)

// ValidatePagination validates and normalises pagination parameters.
// Returns (limit, offset, error).
func ValidatePagination(limitStr, offsetStr string) (int, int, error) {
	limit := DefaultPageSize
	offset := 0

	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 1 {
			return 0, 0, ErrInvalidPagination
		}
		if l > MaxPageSize {
			return 0, 0, ErrInvalidPagination
		}
		limit = l
	}

	if offsetStr != "" {
		o, err := strconv.Atoi(offsetStr)
		if err != nil || o < 0 {
			return 0, 0, ErrInvalidPagination
		}
		if o > MaxPageOffset {
			offset = MaxPageOffset
		} else {
			offset = o
		}
	}

	return limit, offset, nil
}

// SanitiseNullBytes removes null bytes from a string.
// Null bytes can cause issues with databases and should be stripped.
func SanitiseNullBytes(s string) string {
	return strings.ReplaceAll(s, "\x00", "")
}

// WriteUnauthorised writes a 401 Unauthorised response with WWW-Authenticate header.
func WriteUnauthorised(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
	http.Error(w, message, http.StatusUnauthorized)
}

// IsValidUUID returns true if the string is a valid UUID.
func IsValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
