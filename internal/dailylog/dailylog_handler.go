package dailylog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
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
	case errors.Is(err, ErrUnauthorised):
		shared.WriteUnauthorised(w, "unauthorised access")
	case errors.Is(err, ErrMissingParameters):
		http.Error(w, "missing required parameters", http.StatusBadRequest)
	case errors.Is(err, ErrDailyLogNotFound):
		http.Error(w, "daily log not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalidDate):
		http.Error(w, "invalid date: use YYYY-MM-DD format", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidDateRange):
		http.Error(w, "invalid date range: start_date must be before end_date", http.StatusBadRequest)
	case errors.Is(err, ErrDailyLogExists):
		http.Error(w, "daily log already exists for this date", http.StatusConflict)
	case errors.Is(err, ErrEntryTooLong):
		http.Error(w, "entry exceeds maximum of 10000 characters", http.StatusBadRequest)
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

// ListDailyLogs handles GET /daily-logs.
// Query params: date (single day) OR start_date and end_date (range).
func (h *Handler) ListDailyLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

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

	id := h.sanitise(chi.URLParam(r, "id"))
	if id == "" {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}
	if !shared.IsValidUUID(id) {
		http.Error(w, "invalid id: must be a valid UUID", http.StatusBadRequest)
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
