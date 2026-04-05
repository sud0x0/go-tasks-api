package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
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
		case "min":
			messages = append(messages, field+" must be at least "+e.Param()+" characters")
		case "max":
			messages = append(messages, field+" must be at most "+e.Param()+" characters")
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
	return &Handler{
		service:   service,
		logger:    log,
		validate:  validator.New(),
		sanitiser: bluemonday.StrictPolicy(),
	}
}

// sanitise removes HTML tags and null bytes from any user input.
func (h *Handler) sanitise(input string) string {
	return shared.SanitiseNullBytes(html.UnescapeString(h.sanitiser.Sanitize(input)))
}

// handleError maps domain errors to HTTP responses.
func (h *Handler) handleError(ctx context.Context, w http.ResponseWriter, err error) {
	log := logger.FromContext(ctx, h.logger)

	switch {
	case errors.Is(err, ErrDatabase):
		log.LogError(ErrDatabase, err)
		http.Error(w, "database error occurred", http.StatusInternalServerError)
	case errors.Is(err, ErrUserExists):
		http.Error(w, "username already exists", http.StatusConflict)
	case errors.Is(err, ErrInvalidCredentials):
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
	case errors.Is(err, ErrInvalidToken):
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
	case errors.Is(err, ErrTokenRevoked):
		http.Error(w, "token has been revoked", http.StatusUnauthorized)
	case errors.Is(err, ErrUsernameTooLong):
		http.Error(w, "username exceeds maximum of 50 characters", http.StatusBadRequest)
	case errors.Is(err, ErrPasswordTooLong):
		http.Error(w, "password exceeds maximum of 72 characters", http.StatusBadRequest)
	case errors.Is(err, ErrPasswordTooShort):
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidUsername):
		http.Error(w, "username cannot be empty or whitespace", http.StatusBadRequest)
	case errors.Is(err, ErrMissingParameters):
		http.Error(w, "missing required parameters", http.StatusBadRequest)
	case errors.Is(err, ErrValidation):
		http.Error(w, "validation error", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidReqBody):
		http.Error(w, "invalid request body", http.StatusBadRequest)
	default:
		log.LogError(ErrInternalServer, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
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
	// Note: Do NOT sanitise password - it may legitimately contain special characters

	if err := h.validate.Struct(req); err != nil {
		http.Error(w, formatValidationErrors(err), http.StatusBadRequest)
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
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise username only - not password
	req.Username = h.sanitise(req.Username)

	if err := h.validate.Struct(req); err != nil {
		http.Error(w, formatValidationErrors(err), http.StatusBadRequest)
		return
	}

	tokens, err := h.service.login(ctx, req)
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

	h.responseJSON(ctx, w, tokens, http.StatusOK)
}

// Refresh handles POST /api/v1/auth/refresh.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise all fields before validation
	req.RefreshToken = h.sanitise(req.RefreshToken)

	if err := h.validate.Struct(req); err != nil {
		http.Error(w, formatValidationErrors(err), http.StatusBadRequest)
		return
	}

	tokens, err := h.service.refresh(ctx, req.RefreshToken)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	log := logger.FromContext(ctx, h.logger)
	log.LogInfo("token refreshed")

	h.responseJSON(ctx, w, tokens, http.StatusOK)
}

// Logout handles POST /api/v1/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req LogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise all fields before validation
	req.RefreshToken = h.sanitise(req.RefreshToken)

	if err := h.validate.Struct(req); err != nil {
		http.Error(w, formatValidationErrors(err), http.StatusBadRequest)
		return
	}

	// Extract jti and exp from access token if present in Authorization header
	var jti string
	var tokenExp time.Time
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		// Try to extract jti and exp even if token is expired/invalid
		_, extractedJti, extractedExp, _ := h.service.validateAccessToken(ctx, tokenString)
		jti = extractedJti
		tokenExp = extractedExp
	}

	if err := h.service.logout(ctx, req.RefreshToken, jti, tokenExp); err != nil {
		h.handleError(ctx, w, err)
		return
	}

	log := logger.FromContext(ctx, h.logger)
	log.LogInfo("logout succeeded")

	w.WriteHeader(http.StatusNoContent)
}
