package shared

// LogMaxChars is the maximum number of characters allowed in a log entry.
// Adjust this constant as needed for your domain.
const LogMaxChars = 10000 // ~2,000 words

// DailyLogMaxChars is the maximum number of characters allowed in a daily journal entry.
const DailyLogMaxChars = 10000 // ~2,000 words

// LimitExceededError represents a limit exceeded error with details.
// This is the canonical type used across all packages.
type LimitExceededError struct {
	ErrorType string `json:"error"`
	Message   string `json:"message"`
	Limit     int    `json:"limit"`
	Current   int    `json:"current"`
}

// Error implements the error interface.
func (e *LimitExceededError) Error() string {
	return e.Message
}

// NewLimitExceededError creates a new limit exceeded error.
func NewLimitExceededError(message string, limit, current int) *LimitExceededError {
	return &LimitExceededError{
		ErrorType: "limit_exceeded",
		Message:   message,
		Limit:     limit,
		Current:   current,
	}
}
