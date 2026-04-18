package dailylog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go-tasks-api/internal/auth"
	"go-tasks-api/internal/shared"
	"go-tasks-api/internal/shared/logger"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
)

// Handler handles HTTP requests for daily logs.
type Handler struct {
	service   dailylogService
	logger    dailylogLogger
	validate  *validator.Validate
	sanitiser *bluemonday.Policy
}

// NewDailyLogHandler creates a new Handler.
func NewDailyLogHandler(service dailylogService, log dailylogLogger) *Handler {
	v := validator.New()
	// Register rune-based length validators for Unicode-aware validation.
	// This panics on startup if registration fails, which is appropriate
	// since the handler cannot function correctly without these validators.
	if err := shared.RegisterRuneLenValidators(v); err != nil {
		panic("dailylog: failed to register rune validators: " + err.Error())
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
	case errors.Is(err, ErrUnauthorised):
		shared.WriteUnauthorised(w)
	case errors.Is(err, ErrMissingParameters):
		shared.WriteErrorJSON(w, "missing required parameters", http.StatusBadRequest)
	case errors.Is(err, ErrDailyLogNotFound):
		shared.WriteErrorJSON(w, "daily log not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalidDate):
		shared.WriteErrorJSON(w, "invalid date: use YYYY-MM-DD format", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidDateRange):
		shared.WriteErrorJSON(w, "invalid date range: start_date must be before end_date", http.StatusBadRequest)
	case errors.Is(err, ErrDailyLogExists):
		shared.WriteErrorJSON(w, "daily log already exists for this date", http.StatusConflict)
	case errors.Is(err, ErrEntryTooLong):
		shared.WriteErrorJSON(w, "entry exceeds maximum of 10000 characters", http.StatusBadRequest)
	case errors.Is(err, ErrDailyLogAlreadyActive):
		shared.WriteErrorJSON(w, "daily log is already active", http.StatusConflict)
	case errors.Is(err, ErrDailyLogAlreadyInactive):
		shared.WriteErrorJSON(w, "daily log is already inactive; use DELETE /permanent to destroy it", http.StatusConflict)
	case errors.Is(err, ErrCannotPermanentDeleteActiveDailyLog):
		shared.WriteErrorJSON(w, "cannot permanently delete an active daily log; deactivate it first", http.StatusConflict)
	case errors.Is(err, ErrValidation):
		shared.WriteErrorJSON(w, "validation error", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidReqBody):
		shared.WriteErrorJSON(w, "invalid request body", http.StatusBadRequest)
	case errors.Is(err, ErrTooManyIDs):
		shared.WriteErrorJSON(w, "too many ids: maximum 100 per request", http.StatusBadRequest)
	case errors.Is(err, ErrEmptyIDList):
		shared.WriteErrorJSON(w, "ids list must not be empty", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidInput):
		shared.WriteErrorJSON(w, "invalid input", http.StatusBadRequest)
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

// ListDailyLogs handles GET /daily-logs.
// Query params: date (single day) OR start_date and end_date (range).
func (h *Handler) ListDailyLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	// Sanitise all fields before validation
	dateStr := h.sanitise(r.URL.Query().Get("date"))
	startDateStr := h.sanitise(r.URL.Query().Get("start_date"))
	endDateStr := h.sanitise(r.URL.Query().Get("end_date"))

	if dateStr != "" {
		// Single day query
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			h.handleError(ctx, w, ErrInvalidDate)
			return
		}
		log, err := h.service.getDailyLogByDate(ctx, userID, date)
		if err != nil {
			if errors.Is(err, ErrDailyLogNotFound) {
				// Return empty array instead of 404 for single day query
				h.responseJSON(ctx, w, []DailyLog{}, http.StatusOK)
				return
			}
			h.handleError(ctx, w, err)
			return
		}
		h.responseJSON(ctx, w, []DailyLog{log}, http.StatusOK)
		return
	}

	if startDateStr != "" && endDateStr != "" {
		// Range query
		startDate, err := time.Parse("2006-01-02", startDateStr)
		if err != nil {
			h.handleError(ctx, w, ErrInvalidDate)
			return
		}
		endDate, err := time.Parse("2006-01-02", endDateStr)
		if err != nil {
			h.handleError(ctx, w, ErrInvalidDate)
			return
		}
		logs, err := h.service.getDailyLogsByDateRange(ctx, userID, startDate, endDate)
		if err != nil {
			h.handleError(ctx, w, err)
			return
		}
		h.responseJSON(ctx, w, logs, http.StatusOK)
		return
	}

	// Default: return today's log if exists
	today := time.Now().UTC().Truncate(24 * time.Hour)
	log, err := h.service.getDailyLogByDate(ctx, userID, today)
	if err != nil {
		if errors.Is(err, ErrDailyLogNotFound) {
			h.responseJSON(ctx, w, []DailyLog{}, http.StatusOK)
			return
		}
		h.handleError(ctx, w, err)
		return
	}
	h.responseJSON(ctx, w, []DailyLog{log}, http.StatusOK)
}

// CreateDailyLog handles POST /daily-logs.
func (h *Handler) CreateDailyLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise all fields before validation
	req.LogDate = h.sanitise(req.LogDate)
	req.Entry = h.sanitise(req.Entry)

	if err := h.validate.Struct(req); err != nil {
		h.handleError(ctx, w, ErrValidation)
		return
	}

	log, err := h.service.createDailyLog(ctx, userID, req)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, log, http.StatusCreated)
}

// UpdateDailyLog handles PUT /daily-logs/{id}.
func (h *Handler) UpdateDailyLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	// Sanitise all fields before validation
	id := h.sanitise(chi.URLParam(r, "id"))
	if id == "" {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}
	if !shared.IsValidUUID(id) {
		shared.WriteErrorJSON(w, "invalid id: must be a valid UUID", http.StatusBadRequest)
		return
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise all fields before validation
	req.Entry = h.sanitise(req.Entry)

	if err := h.validate.Struct(req); err != nil {
		h.handleError(ctx, w, ErrValidation)
		return
	}

	log, err := h.service.updateDailyLog(ctx, id, userID, req)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, log, http.StatusOK)
}

// DeleteDailyLog handles DELETE /daily-logs/{id}.
// Performs soft-delete only. Returns 409 if already inactive.
func (h *Handler) DeleteDailyLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	// Sanitise all fields before validation
	id := h.sanitise(chi.URLParam(r, "id"))
	if id == "" {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}
	if !shared.IsValidUUID(id) {
		shared.WriteErrorJSON(w, "invalid id: must be a valid UUID", http.StatusBadRequest)
		return
	}

	if err := h.service.deleteDailyLog(ctx, id, userID); err != nil {
		h.handleError(ctx, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PermanentDeleteDailyLog handles DELETE /daily-logs/{id}/permanent.
// Performs hard-delete on inactive daily logs only. Returns 409 if still active.
func (h *Handler) PermanentDeleteDailyLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	// Sanitise all fields before validation
	id := h.sanitise(chi.URLParam(r, "id"))
	if id == "" {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}
	if !shared.IsValidUUID(id) {
		shared.WriteErrorJSON(w, "invalid id: must be a valid UUID", http.StatusBadRequest)
		return
	}

	if err := h.service.permanentDeleteDailyLog(ctx, id, userID); err != nil {
		h.handleError(ctx, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// BulkDeleteDailyLogs handles POST /daily-logs/bulk-delete.
// Performs bulk soft-delete only. Inactive IDs are ignored.
func (h *Handler) BulkDeleteDailyLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	var req BulkDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise each ID before validation
	for i, id := range req.IDs {
		req.IDs[i] = h.sanitise(id)
	}

	if err := h.validate.Struct(req); err != nil {
		h.handleError(ctx, w, ErrValidation)
		return
	}

	// Validate each ID is a valid UUID
	for _, id := range req.IDs {
		if !shared.IsValidUUID(id) {
			shared.WriteErrorJSON(w, "invalid id: all IDs must be valid UUIDs", http.StatusBadRequest)
			return
		}
	}

	requested, softDeleted, err := h.service.bulkDeleteDailyLogs(ctx, userID, req.IDs)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, BulkDeleteResponse{Requested: requested, SoftDeleted: softDeleted}, http.StatusOK)
}

// BulkPermanentDeleteDailyLogs handles POST /daily-logs/bulk-permanent-delete.
// Performs bulk hard-delete on inactive daily logs only. Active IDs are ignored.
func (h *Handler) BulkPermanentDeleteDailyLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	var req BulkDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise each ID before validation
	for i, id := range req.IDs {
		req.IDs[i] = h.sanitise(id)
	}

	if err := h.validate.Struct(req); err != nil {
		h.handleError(ctx, w, ErrValidation)
		return
	}

	// Validate each ID is a valid UUID
	for _, id := range req.IDs {
		if !shared.IsValidUUID(id) {
			shared.WriteErrorJSON(w, "invalid id: all IDs must be valid UUIDs", http.StatusBadRequest)
			return
		}
	}

	requested, permanentlyDeleted, err := h.service.bulkPermanentDeleteDailyLogs(ctx, userID, req.IDs)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, BulkPermanentDeleteResponse{Requested: requested, PermanentlyDeleted: permanentlyDeleted}, http.StatusOK)
}

// ListInactiveDailyLogs handles GET /daily-logs/inactive.
func (h *Handler) ListInactiveDailyLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	// Sanitise all fields before validation
	limitStr := h.sanitise(r.URL.Query().Get("limit"))
	offsetStr := h.sanitise(r.URL.Query().Get("offset"))

	limit, offset, err := shared.ValidatePagination(limitStr, offsetStr)
	if err != nil {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}

	logs, err := h.service.getInactiveDailyLogs(ctx, userID, limit, offset)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, logs, http.StatusOK)
}

// ReactivateDailyLog handles POST /daily-logs/{id}/reactivate.
func (h *Handler) ReactivateDailyLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	// Sanitise all fields before validation
	id := h.sanitise(chi.URLParam(r, "id"))
	if id == "" {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}
	if !shared.IsValidUUID(id) {
		shared.WriteErrorJSON(w, "invalid id: must be a valid UUID", http.StatusBadRequest)
		return
	}

	log, err := h.service.reactivateDailyLog(ctx, id, userID)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, log, http.StatusOK)
}
