package category

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"

	"go-tasks-api/internal/auth"
	"go-tasks-api/internal/shared"
	"go-tasks-api/internal/shared/logger"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
)

// Handler handles HTTP requests for categories.
type Handler struct {
	service   categoryService
	logger    categoryLogger
	validate  *validator.Validate
	sanitiser *bluemonday.Policy
}

// NewCategoryHandler creates a new Handler.
func NewCategoryHandler(service categoryService, log categoryLogger) *Handler {
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

// sanitisePtr sanitises a string pointer.
func (h *Handler) sanitisePtr(input *string) *string {
	if input == nil {
		return nil
	}
	sanitised := h.sanitise(*input)
	return &sanitised
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
	case errors.Is(err, ErrCategoryNotFound):
		http.Error(w, "category not found", http.StatusNotFound)
	case errors.Is(err, ErrCategoryInUse):
		http.Error(w, "category is in use by active tasks", http.StatusConflict)
	case errors.Is(err, ErrNameTooLong):
		http.Error(w, "name exceeds maximum of 100 characters", http.StatusBadRequest)
	case errors.Is(err, ErrDescriptionTooLong):
		http.Error(w, "description exceeds maximum of 500 characters", http.StatusBadRequest)
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

// GetCategory handles GET /categories/{id}.
func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
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

	cat, err := h.service.getCategory(ctx, id, userID)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, cat, http.StatusOK)
}

// ListCategories handles GET /categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	limitStr := h.sanitise(r.URL.Query().Get("limit"))
	offsetStr := h.sanitise(r.URL.Query().Get("offset"))

	limit, offset, err := shared.ValidatePagination(limitStr, offsetStr)
	if err != nil {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}

	categories, err := h.service.getCategories(ctx, userID, limit, offset)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, categories, http.StatusOK)
}

// CreateCategory handles POST /categories.
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
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
	req.Name = h.sanitise(req.Name)
	req.Description = h.sanitisePtr(req.Description)

	if err := h.validate.Struct(req); err != nil {
		h.handleError(ctx, w, ErrValidation)
		return
	}

	cat, err := h.service.createCategory(ctx, userID, req)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, cat, http.StatusCreated)
}

// UpdateCategory handles PUT /categories/{id}.
func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
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
	req.Name = h.sanitise(req.Name)
	req.Description = h.sanitisePtr(req.Description)

	if err := h.validate.Struct(req); err != nil {
		h.handleError(ctx, w, ErrValidation)
		return
	}

	cat, err := h.service.updateCategory(ctx, id, userID, req)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, cat, http.StatusOK)
}

// DeleteCategory handles DELETE /categories/{id}.
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.deleteCategory(ctx, id, userID); err != nil {
		h.handleError(ctx, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
