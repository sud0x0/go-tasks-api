package shared

import (
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
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

// SanitiseHTML strips HTML tags and unescapes HTML entities.
// Use this for API text fields that should not contain HTML.
// The bluemonday sanitizer encodes special chars like & to &amp;
// so we unescape them after stripping tags.
func SanitiseHTML(sanitized string) string {
	// Unescape HTML entities (e.g., &amp; -> &, &lt; -> <)
	return html.UnescapeString(SanitiseNullBytes(sanitized))
}

// WriteUnauthorised writes a 401 Unauthorised JSON response.
func WriteUnauthorised(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}

// WriteErrorJSON writes a JSON error response with the given message and status code.
func WriteErrorJSON(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// IsValidUUID returns true if the string is a valid UUID.
func IsValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// RuneCountLen returns the number of Unicode code points (runes) in a string.
// Use this instead of len() for user-facing text length validation.
// This matches PostgreSQL's VARCHAR(n) character counting semantics.
func RuneCountLen(s string) int {
	return utf8.RuneCountInString(s)
}

// RegisterRuneLenValidators registers custom validators for rune-based length checks:
//   - rune_max=N: string must have at most N runes
//   - rune_min=N: string must have at least N runes
//   - rune_len=N: string must have exactly N runes
//
// Call this in handler constructors before using the validator.
func RegisterRuneLenValidators(v *validator.Validate) error {
	// rune_max=N
	if err := v.RegisterValidation("rune_max", func(fl validator.FieldLevel) bool {
		maxStr := fl.Param()
		max, err := strconv.Atoi(maxStr)
		if err != nil {
			return false
		}
		return utf8.RuneCountInString(fl.Field().String()) <= max
	}); err != nil {
		return err
	}

	// rune_min=N
	if err := v.RegisterValidation("rune_min", func(fl validator.FieldLevel) bool {
		minStr := fl.Param()
		min, err := strconv.Atoi(minStr)
		if err != nil {
			return false
		}
		return utf8.RuneCountInString(fl.Field().String()) >= min
	}); err != nil {
		return err
	}

	// rune_len=N
	if err := v.RegisterValidation("rune_len", func(fl validator.FieldLevel) bool {
		lenStr := fl.Param()
		expected, err := strconv.Atoi(lenStr)
		if err != nil {
			return false
		}
		return utf8.RuneCountInString(fl.Field().String()) == expected
	}); err != nil {
		return err
	}

	return nil
}
