package log

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"

	"go-tasks-api/internal/shared"
	"go-tasks-api/internal/shared/logger"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
)

// contextKey is an unexported type for context keys in this package.
// Replace this with your own auth package's GetUserID once you add authentication.
type contextKey string

// UserContextKey is the context key used by the auth middleware to store the user ID.
// This must match the key set in your auth middleware.
const UserContextKey contextKey = "user_id"

// getUserID extracts the authenticated user ID from the request context.
// Returns an empty string if no user ID is present (unauthenticated).
func getUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserContextKey).(string); ok {
		return userID
	}
	return ""
}

// Handler handles HTTP requests for logs.
type Handler struct {
	service   logService
	logger    logLogger
	validate  *validator.Validate
	sanitiser *bluemonday.Policy
}

// NewLogHandler creates a new Handler.
// validate and sanitiser are initialised here rather than as package-level globals
// to keep the handler self-contained and testable.
func NewLogHandler(service logService, log logLogger) *Handler {
	return &Handler{
		service:   service,
		logger:    log,
		validate:  validator.New(),
		sanitiser: bluemonday.StrictPolicy(),
	}
}

// sanitise removes HTML tags and null bytes from any user input.
// Apply this to ALL user inputs before validation or use.
func (h *Handler) sanitise(input string) string {
	return shared.SanitiseNullBytes(html.UnescapeString(h.sanitiser.Sanitize(input)))
}

// handleError maps domain errors to HTTP responses. This is the single place
// where errors are logged — lower layers wrap and return, this layer decides.
func (h *Handler) handleError(ctx context.Context, w http.ResponseWriter, err error) {
	log := logger.FromContext(ctx, h.logger)
	// Structured limit errors get a JSON body.
	var limitErr *shared.LimitExceededError
	if errors.As(err, &limitErr) {
		h.responseJSON(ctx, w, limitErr, http.StatusBadRequest)
		return
	}

	switch {
	case errors.Is(err, ErrDatabase):
		// Log internal errors here — the only layer that does so.
		log.LogError(ErrDatabase, err)
		http.Error(w, "database error occurred", http.StatusInternalServerError)
	case errors.Is(err, ErrUnauthorised):
		shared.WriteUnauthorised(w, "unauthorised access")
	case errors.Is(err, ErrMissingParameters):
		http.Error(w, "missing required parameters", http.StatusBadRequest)
	case errors.Is(err, ErrLogNotFound):
		http.Error(w, "log not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalidDateTime):
		http.Error(w, "invalid date_and_time: use RFC3339 e.g. 2006-01-02T15:04:05Z", http.StatusBadRequest)
	case errors.Is(err, ErrDateTimeOutOfRange):
		http.Error(w, "date_and_time year out of supported range", http.StatusBadRequest)
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
// This prevents the partial-response bug where WriteHeader is sent before
// a marshal error is discovered, making a subsequent http.Error a no-op.
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

// GetLog handles GET /logs/{id}.
func (h *Handler) GetLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	// Sanitise all fields before validation.
	id := h.sanitise(chi.URLParam(r, "id"))
	if id == "" {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}
	if !shared.IsValidUUID(id) {
		http.Error(w, "invalid id: must be a valid UUID", http.StatusBadRequest)
		return
	}

	entry, err := h.service.getLog(ctx, id, userID)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, entry, http.StatusOK)
}

// ListLogs handles GET /logs.
// Optional query params: start_date, end_date (RFC3339), limit, offset.
// When start_date or end_date are omitted the service defaults to 1970-01-01 and
// 2099-12-31 respectively — always supply a date range in production.
func (h *Handler) ListLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	// Sanitise all fields before validation.
	limitStr := h.sanitise(r.URL.Query().Get("limit"))
	offsetStr := h.sanitise(r.URL.Query().Get("offset"))
	startDate := h.sanitise(r.URL.Query().Get("start_date"))
	endDate := h.sanitise(r.URL.Query().Get("end_date"))

	limit, offset, err := shared.ValidatePagination(limitStr, offsetStr)
	if err != nil {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}

	logs, err := h.service.getLogs(
		ctx, userID,
		startDate,
		endDate,
		limit, offset,
	)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, logs, http.StatusOK)
}

// CreateLog handles POST /logs.
func (h *Handler) CreateLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise all fields before validation.
	req.DateAndTime = h.sanitise(req.DateAndTime)
	req.Log = h.sanitise(req.Log)

	if err := h.validate.Struct(req); err != nil {
		h.handleError(ctx, w, ErrValidation)
		return
	}

	entry, err := h.service.createLog(ctx, userID, req)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, entry, http.StatusCreated)
}

// UpdateLog handles PUT /logs/{id}.
func (h *Handler) UpdateLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	// Sanitise all fields before validation.
	id := h.sanitise(chi.URLParam(r, "id"))
	if id == "" {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}
	if !shared.IsValidUUID(id) {
		http.Error(w, "invalid id: must be a valid UUID", http.StatusBadRequest)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	req.DateAndTime = h.sanitise(req.DateAndTime)
	req.Log = h.sanitise(req.Log)

	if err := h.validate.Struct(req); err != nil {
		h.handleError(ctx, w, ErrValidation)
		return
	}

	entry, err := h.service.updateLog(ctx, id, userID, req)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, entry, http.StatusOK)
}

// DeleteLog handles DELETE /logs/{id}.
func (h *Handler) DeleteLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := getUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	// Sanitise all fields before validation.
	id := h.sanitise(chi.URLParam(r, "id"))
	if id == "" {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}
	if !shared.IsValidUUID(id) {
		http.Error(w, "invalid id: must be a valid UUID", http.StatusBadRequest)
		return
	}

	if err := h.service.deleteLog(ctx, id, userID); err != nil {
		h.handleError(ctx, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
