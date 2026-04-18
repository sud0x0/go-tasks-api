package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"go-tasks-api/internal/shared"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// UserContextKey is the context key used to store the authenticated user ID.
// This is the canonical key used across all domains.
const UserContextKey contextKey = "user_id"

// JTIContextKey is the context key used to store the JWT ID.
const JTIContextKey contextKey = "jti"

// Middleware provides JWT authentication middleware.
type Middleware struct {
	service authService
	logger  authLogger
}

// NewAuthMiddleware creates a new auth middleware.
func NewAuthMiddleware(service authService, log authLogger) *Middleware {
	return &Middleware{
		service: service,
		logger:  log,
	}
}

// Handler returns the middleware handler function.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract access token from Authorization header.
		// Must be in format: "Bearer <token>" (case-sensitive, exactly one space).
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeUnauthorizedJSON(w)
			return
		}

		// Parse Bearer scheme - must start with exact prefix "Bearer " (case-sensitive, one space)
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			writeUnauthorizedJSON(w)
			return
		}

		// Extract token after the prefix
		tokenString := authHeader[len(bearerPrefix):]

		// Sanitize (Rule 1: sanitise before validate)
		tokenString = shared.SanitiseNullBytes(tokenString)
		if tokenString == "" {
			writeUnauthorizedJSON(w)
			return
		}

		// Validate token length (Rule 2: type and validate inputs)
		// Access tokens are JWTs which should not exceed 4096 bytes
		if len(tokenString) > 4096 {
			writeUnauthorizedJSON(w)
			return
		}

		// Validate the token
		userID, jti, _, err := m.service.validateAccessToken(ctx, tokenString)
		if err != nil {
			writeUnauthorizedJSON(w)
			return
		}

		// Set user ID and jti in context
		ctx = context.WithValue(ctx, UserContextKey, userID)
		ctx = context.WithValue(ctx, JTIContextKey, jti)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// writeUnauthorizedJSON writes a 401 Unauthorized JSON response.
func writeUnauthorizedJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}

// GetUserID extracts the authenticated user ID from the request context.
// Returns an empty string if no user ID is present (unauthenticated).
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserContextKey).(string); ok {
		return userID
	}
	return ""
}

// GetJTI extracts the JWT ID from the request context.
// Returns an empty string if no jti is present.
func GetJTI(ctx context.Context) string {
	if jti, ok := ctx.Value(JTIContextKey).(string); ok {
		return jti
	}
	return ""
}
