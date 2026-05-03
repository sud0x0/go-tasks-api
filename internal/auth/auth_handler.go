package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"go-tasks-api/internal/shared"
	"go-tasks-api/internal/shared/logger"

	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
)

// Handler handles HTTP requests for auth.
type Handler struct {
	service   authService
	logger    authLogger
	validate  *validator.Validate
	sanitiser *bluemonday.Policy
}

// formatValidationErrors converts validator errors to a human-readable message.
func formatValidationErrors(err error) string {
	var errs validator.ValidationErrors
	if !errors.As(err, &errs) {
		return "validation error"
	}

	var messages []string
	for _, e := range errs {
		field := strings.ToLower(e.Field())
		switch e.Tag() {
		case "required":
			messages = append(messages, field+" is required")
		case "min", "rune_min":
			messages = append(messages, field+" must be at least "+e.Param()+" characters")
		case "max", "rune_max":
			messages = append(messages, field+" must be at most "+e.Param()+" characters")
		case "rune_len":
			messages = append(messages, field+" must be exactly "+e.Param()+" characters")
		case "email":
			messages = append(messages, field+" must be a valid email address")
		default:
			messages = append(messages, field+" is invalid")
		}
	}
	return strings.Join(messages, "; ")
}

// NewAuthHandler creates a new Handler.
func NewAuthHandler(service authService, log authLogger) *Handler {
	v := validator.New()
	// Register rune-based length validators for Unicode-aware validation.
	// This panics on startup if registration fails, which is appropriate
	// since the handler cannot function correctly without these validators.
	if err := shared.RegisterRuneLenValidators(v); err != nil {
		panic("auth: failed to register rune validators: " + err.Error())
	}
	return &Handler{
		service:   service,
		logger:    log,
		validate:  v,
		sanitiser: bluemonday.StrictPolicy(),
	}
}

// sanitise removes HTML tags, null bytes, and unescapes HTML entities.
func (h *Handler) sanitise(input string) string {
	return shared.SanitiseHTML(h.sanitiser.Sanitize(input))
}

// handleError maps domain errors to HTTP responses (JSON format).
func (h *Handler) handleError(ctx context.Context, w http.ResponseWriter, err error) {
	log := logger.FromContext(ctx, h.logger)

	switch {
	case errors.Is(err, ErrDatabase):
		log.LogError(ErrDatabase, err)
		shared.WriteErrorJSON(w, "database error occurred", http.StatusInternalServerError)
	case errors.Is(err, ErrUserExists):
		shared.WriteErrorJSON(w, "username already exists", http.StatusConflict)
	case errors.Is(err, ErrInvalidCredentials):
		shared.WriteErrorJSON(w, "invalid credentials", http.StatusUnauthorized)
	case errors.Is(err, ErrInvalidToken):
		shared.WriteErrorJSON(w, "invalid or expired token", http.StatusUnauthorized)
	case errors.Is(err, ErrTokenRevoked):
		shared.WriteErrorJSON(w, "token has been revoked", http.StatusUnauthorized)
	case errors.Is(err, ErrTokenOwnershipMismatch):
		shared.WriteUnauthorised(w)
	case errors.Is(err, ErrValkeyUnavailable):
		log.LogError(ErrValkeyUnavailable, err)
		shared.WriteErrorJSON(w, "service temporarily unavailable", http.StatusServiceUnavailable)
	case errors.Is(err, ErrUsernameTooLong):
		shared.WriteErrorJSON(w, "username exceeds maximum of 50 characters", http.StatusBadRequest)
	case errors.Is(err, ErrPasswordTooLong):
		shared.WriteErrorJSON(w, "password exceeds maximum of 128 characters", http.StatusBadRequest)
	case errors.Is(err, ErrPasswordTooShort):
		shared.WriteErrorJSON(w, "password must be at least 8 characters", http.StatusBadRequest)
	case errors.Is(err, ErrPasswordInvalidChars):
		shared.WriteErrorJSON(w, "password contains invalid control characters", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidUsername):
		shared.WriteErrorJSON(w, "username cannot be empty or whitespace", http.StatusBadRequest)
	case errors.Is(err, ErrMissingParameters):
		shared.WriteErrorJSON(w, "missing required parameters", http.StatusBadRequest)
	case errors.Is(err, ErrValidation):
		shared.WriteErrorJSON(w, "validation error", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidReqBody):
		shared.WriteErrorJSON(w, "invalid request body", http.StatusBadRequest)
	default:
		log.LogError(ErrInternalServer, err)
		shared.WriteErrorJSON(w, "internal server error", http.StatusInternalServerError)
	}
}

// responseJSON marshals data to a buffer before writing the header and body.
func (h *Handler) responseJSON(ctx context.Context, w http.ResponseWriter, data any, status int) {
	log := logger.FromContext(ctx, h.logger)
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(data); err != nil {
		log.LogError(ErrInternalServer, err)
		http.Error(w, "error encoding response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
	_, _ = w.Write(buf.Bytes())
}

// Register handles POST /api/v1/auth/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise all fields before validation
	req.Username = h.sanitise(req.Username)
	// SECURITY: Do NOT sanitise password - bluemonday would mangle legitimate input
	// and HTML-unescape could change the password. Control character validation
	// is handled separately in the service layer.

	if err := h.validate.Struct(req); err != nil {
		shared.WriteErrorJSON(w, formatValidationErrors(err), http.StatusBadRequest)
		return
	}

	user, err := h.service.register(ctx, req)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	log := logger.FromContext(ctx, h.logger)
	log.LogInfo("user registered", "username", req.Username)

	h.responseJSON(ctx, w, user.ToResponse(), http.StatusCreated)
}

// Login handles POST /api/v1/auth/login.
// Returns user info and tokens in the response body.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise username only - not password
	// SECURITY: Do NOT sanitise password - bluemonday would mangle legitimate input
	// and HTML-unescape could change the password. Control character validation
	// is handled separately in the service layer.
	req.Username = h.sanitise(req.Username)

	if err := h.validate.Struct(req); err != nil {
		shared.WriteErrorJSON(w, formatValidationErrors(err), http.StatusBadRequest)
		return
	}

	// Login and get tokens + user
	tokens, user, err := h.service.loginWithUser(ctx, req)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			log := logger.FromContext(ctx, h.logger)
			log.LogInfo("login failed", "username", req.Username, "reason", "invalid credentials")
		}
		h.handleError(ctx, w, err)
		return
	}

	log := logger.FromContext(ctx, h.logger)
	log.LogInfo("login succeeded", "username", req.Username)

	// Calculate expires_at from ExpiresIn (15 min = 900 seconds)
	expiresAt := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)

	// Return user info and tokens in body
	h.responseJSON(ctx, w, LoginResponse{
		User:         user.ToResponse(),
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    expiresAt,
		TokenType:    tokens.TokenType,
	}, http.StatusOK)
}

// Refresh handles POST /api/v1/auth/refresh.
// Reads refresh token from X-Refresh-Token header, rotates tokens, returns new tokens in body.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read refresh token from X-Refresh-Token header
	refreshToken := r.Header.Get("X-Refresh-Token")
	if refreshToken == "" {
		h.handleError(ctx, w, ErrInvalidToken)
		return
	}

	// Sanitise the refresh token (Rule 1: sanitise before validate)
	refreshToken = h.sanitise(refreshToken)
	if refreshToken == "" {
		h.handleError(ctx, w, ErrInvalidToken)
		return
	}

	// Validate refresh token format (Rule 2: type and validate inputs)
	// Refresh token is 32 bytes base64url encoded = 43 characters
	if len(refreshToken) > 64 {
		h.handleError(ctx, w, ErrInvalidToken)
		return
	}

	// Extract old access token from Authorization header (if provided) to blocklist its JTI
	var oldAccessToken string
	authHeader := r.Header.Get("Authorization")
	const bearerPrefix = "Bearer "
	if strings.HasPrefix(authHeader, bearerPrefix) {
		oldAccessToken = h.sanitise(authHeader[len(bearerPrefix):])
	}

	// Perform token rotation
	tokens, err := h.service.refresh(ctx, refreshToken, oldAccessToken)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	log := logger.FromContext(ctx, h.logger)
	log.LogInfo("token refreshed")

	// Calculate expires_at from ExpiresIn
	expiresAt := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)

	// Return new tokens in body
	h.responseJSON(ctx, w, RefreshResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    expiresAt,
		TokenType:    tokens.TokenType,
	}, http.StatusOK)
}

// Logout handles POST /api/v1/auth/logout.
// Reads tokens from headers, verifies ownership, revokes tokens, returns 204 No Content.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx, h.logger)

	// Read refresh token from X-Refresh-Token header
	refreshToken := r.Header.Get("X-Refresh-Token")
	if refreshToken == "" {
		// No refresh token - just return 204 (no-op)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Sanitise the refresh token (Rule 1: sanitise before validate)
	refreshToken = h.sanitise(refreshToken)
	if refreshToken == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Read access token from Authorization header
	authHeader := r.Header.Get("Authorization")
	var userID, jti string
	var tokenExp time.Time

	const bearerPrefix = "Bearer "
	if strings.HasPrefix(authHeader, bearerPrefix) {
		accessToken := authHeader[len(bearerPrefix):]
		accessToken = h.sanitise(accessToken)
		// Extract user ID, JTI, and expiration from access token
		userID, jti, tokenExp, _ = h.service.validateAccessToken(ctx, accessToken)
	}

	// Logout requires a valid access token so we can verify the refresh token's
	// ownership before revoking it. Without a verifiable userID we return 204
	// without acting — the legitimate user can re-authenticate and try again.
	if userID == "" {
		log.LogInfo("logout: ignored, no valid access token")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	tokenHash := hashRefreshToken(refreshToken)
	if err := h.service.logoutWithOwnershipCheck(ctx, tokenHash, userID, jti, tokenExp); err != nil {
		if errors.Is(err, ErrTokenOwnershipMismatch) {
			log.LogInfo("logout ownership mismatch", "user_id", userID)
			shared.WriteUnauthorised(w)
			return
		}
		log.LogInfo("logout error", "error", err.Error())
	}

	log.LogInfo("logout succeeded")
	w.WriteHeader(http.StatusNoContent)
}
