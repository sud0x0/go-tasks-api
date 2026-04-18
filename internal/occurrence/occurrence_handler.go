package occurrence

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

// Handler handles HTTP requests for occurrences.
type Handler struct {
	service   occurrenceService
	logger    occurrenceLogger
	validate  *validator.Validate
	sanitiser *bluemonday.Policy
}

// NewOccurrenceHandler creates a new Handler.
func NewOccurrenceHandler(service occurrenceService, log occurrenceLogger) *Handler {
	v := validator.New()
	// Register rune-based length validators for Unicode-aware validation.
	// This panics on startup if registration fails, which is appropriate
	// since the handler cannot function correctly without these validators.
	if err := shared.RegisterRuneLenValidators(v); err != nil {
		panic("occurrence: failed to register rune validators: " + err.Error())
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

// sanitisePtr sanitises a string pointer.
func (h *Handler) sanitisePtr(input *string) *string {
	if input == nil {
		return nil
	}
	sanitised := h.sanitise(*input)
	return &sanitised
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
	case errors.Is(err, ErrOccurrenceNotFound):
		shared.WriteErrorJSON(w, "occurrence not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalidDate):
		shared.WriteErrorJSON(w, "invalid date: use YYYY-MM-DD format", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidDateRange):
		shared.WriteErrorJSON(w, "invalid date range: start_date must be before end_date", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidAnswerType):
		shared.WriteErrorJSON(w, "answer type doesn't match task's expected type", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidSelectOption):
		shared.WriteErrorJSON(w, "invalid select option for this task", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidInput):
		shared.WriteErrorJSON(w, "invalid input", http.StatusBadRequest)
	case errors.Is(err, ErrAnswerStringTooLong):
		shared.WriteErrorJSON(w, "answer_string exceeds maximum of 500 characters", http.StatusBadRequest)
	case errors.Is(err, ErrOccurrenceAlreadySuppressed):
		shared.WriteErrorJSON(w, "occurrence is already suppressed", http.StatusConflict)
	case errors.Is(err, ErrOccurrenceNotSuppressed):
		shared.WriteErrorJSON(w, "occurrence is not suppressed", http.StatusConflict)
	case errors.Is(err, ErrOccurrenceIsSuppressed):
		shared.WriteErrorJSON(w, "cannot answer a suppressed occurrence", http.StatusConflict)
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
		shared.WriteErrorJSON(w, "error encoding response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
	_, _ = w.Write(buf.Bytes())
}

// ListOccurrences handles GET /occurrences.
// Query params: date (single day) OR start_date and end_date (range).
// At least one date parameter is required.
func (h *Handler) ListOccurrences(w http.ResponseWriter, r *http.Request) {
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

	var occurrences []WithDetails
	var err error

	if dateStr != "" {
		// Single day query
		date, parseErr := time.Parse("2006-01-02", dateStr)
		if parseErr != nil {
			h.handleError(ctx, w, ErrInvalidDate)
			return
		}
		occurrences, err = h.service.getOccurrencesByDate(ctx, userID, date)
	} else if startDateStr != "" && endDateStr != "" {
		// Range query
		startDate, parseErr := time.Parse("2006-01-02", startDateStr)
		if parseErr != nil {
			h.handleError(ctx, w, ErrInvalidDate)
			return
		}
		endDate, parseErr := time.Parse("2006-01-02", endDateStr)
		if parseErr != nil {
			h.handleError(ctx, w, ErrInvalidDate)
			return
		}
		occurrences, err = h.service.getOccurrencesByDateRange(ctx, userID, startDate, endDate)
	} else {
		// Missing required date parameters
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}

	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, occurrences, http.StatusOK)
}

// SuppressOccurrence handles POST /occurrences/{id}/suppress.
func (h *Handler) SuppressOccurrence(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.suppressOccurrence(ctx, id, userID); err != nil {
		h.handleError(ctx, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UnsuppressOccurrence handles POST /occurrences/{id}/unsuppress.
func (h *Handler) UnsuppressOccurrence(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.unsuppressOccurrence(ctx, id, userID); err != nil {
		h.handleError(ctx, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SubmitAnswer handles POST /occurrences/{id}/answer.
func (h *Handler) SubmitAnswer(w http.ResponseWriter, r *http.Request) {
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

	var req AnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise all fields before validation
	req.AnswerString = h.sanitisePtr(req.AnswerString)
	req.AnswerSelect = h.sanitisePtr(req.AnswerSelect)

	if err := h.validate.Struct(req); err != nil {
		h.handleError(ctx, w, ErrValidation)
		return
	}

	answer, err := h.service.submitAnswer(ctx, id, userID, req)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, answer, http.StatusOK)
}
