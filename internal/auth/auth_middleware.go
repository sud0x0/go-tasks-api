package auth

import (
	"context"
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

		// Extract Bearer token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			shared.WriteUnauthorised(w, "missing authorization header")
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			shared.WriteUnauthorised(w, "invalid authorization header format")
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == "" {
			shared.WriteUnauthorised(w, "missing token")
			return
		}

		// Validate the token
		userID, jti, _, err := m.service.validateAccessToken(ctx, tokenString)
		if err != nil {
			switch err {
			case ErrTokenRevoked:
				shared.WriteUnauthorised(w, "token has been revoked")
			case ErrInvalidToken:
				shared.WriteUnauthorised(w, "invalid or expired token")
			default:
				shared.WriteUnauthorised(w, "authentication failed")
			}
			return
		}

		// Set user ID and jti in context
		ctx = context.WithValue(ctx, UserContextKey, userID)
		ctx = context.WithValue(ctx, JTIContextKey, jti)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
