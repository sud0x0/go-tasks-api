package category

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	v := validator.New()
	// Register rune-based length validators for Unicode-aware validation.
	// This panics on startup if registration fails, which is appropriate
	// since the handler cannot function correctly without these validators.
	if err := shared.RegisterRuneLenValidators(v); err != nil {
		panic("category: failed to register rune validators: " + err.Error())
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
	case errors.Is(err, ErrCategoryNotFound):
		shared.WriteErrorJSON(w, "category not found", http.StatusNotFound)
	case errors.Is(err, ErrCategoryAlreadyActive):
		shared.WriteErrorJSON(w, "category is already active", http.StatusConflict)
	case errors.Is(err, ErrCategoryAlreadyInactive):
		shared.WriteErrorJSON(w, "category is already inactive; use DELETE /permanent to destroy it", http.StatusConflict)
	case errors.Is(err, ErrCannotPermanentDeleteActive):
		shared.WriteErrorJSON(w, "cannot permanently delete an active category; deactivate it first", http.StatusConflict)
	case errors.Is(err, ErrCategoryHasActiveTasks):
		shared.WriteErrorJSON(w, "category has active tasks; delete or move tasks first", http.StatusConflict)
	case errors.Is(err, ErrReactivateNameCollision):
		shared.WriteErrorJSON(w, "cannot reactivate: another active category has this name", http.StatusConflict)
	case errors.Is(err, ErrTooManyIDs):
		shared.WriteErrorJSON(w, "too many IDs: maximum 100 allowed", http.StatusBadRequest)
	case errors.Is(err, ErrEmptyIDList):
		shared.WriteErrorJSON(w, "at least one ID is required", http.StatusBadRequest)
	case errors.Is(err, ErrNameTooLong):
		shared.WriteErrorJSON(w, "name exceeds maximum of 100 characters", http.StatusBadRequest)
	case errors.Is(err, ErrDescriptionTooLong):
		shared.WriteErrorJSON(w, "description exceeds maximum of 500 characters", http.StatusBadRequest)
	case errors.Is(err, ErrDuplicateName):
		shared.WriteErrorJSON(w, "category with this name already exists", http.StatusConflict)
	case errors.Is(err, ErrInvalidColour):
		shared.WriteErrorJSON(w, "invalid colour: must be in the form #RRGGBB", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidInput):
		shared.WriteErrorJSON(w, "invalid input", http.StatusBadRequest)
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

// GetCategory handles GET /categories/{id}.
func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
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

	// Sanitise all fields before validation
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
	req.Colour = h.sanitisePtr(req.Colour)

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
	req.Name = h.sanitise(req.Name)
	req.Description = h.sanitisePtr(req.Description)
	req.Colour = h.sanitisePtr(req.Colour)

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
// Performs soft-delete only. Returns 409 if already inactive.
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.deleteCategory(ctx, id, userID); err != nil {
		h.handleError(ctx, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PermanentDeleteCategory handles DELETE /categories/{id}/permanent.
// Performs hard-delete on inactive categories only. Returns 409 if still active.
func (h *Handler) PermanentDeleteCategory(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.permanentDeleteCategory(ctx, id, userID); err != nil {
		h.handleError(ctx, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// BulkDeleteCategories handles POST /categories/bulk-delete.
// Performs bulk soft-delete only. Inactive IDs are ignored.
func (h *Handler) BulkDeleteCategories(w http.ResponseWriter, r *http.Request) {
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

	// Sanitise all IDs before validation
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

	requested, softDeleted, err := h.service.bulkDeleteCategories(ctx, userID, req.IDs)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, BulkDeleteResponse{Requested: requested, SoftDeleted: softDeleted}, http.StatusOK)
}

// BulkPermanentDeleteCategories handles POST /categories/bulk-permanent-delete.
// Performs bulk hard-delete on inactive categories only. Active IDs are ignored.
func (h *Handler) BulkPermanentDeleteCategories(w http.ResponseWriter, r *http.Request) {
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

	// Sanitise all IDs before validation
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

	requested, permanentlyDeleted, err := h.service.bulkPermanentDeleteCategories(ctx, userID, req.IDs)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, BulkPermanentDeleteResponse{Requested: requested, PermanentlyDeleted: permanentlyDeleted}, http.StatusOK)
}

// ListInactiveCategories handles GET /categories/inactive.
func (h *Handler) ListInactiveCategories(w http.ResponseWriter, r *http.Request) {
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

	categories, err := h.service.getInactiveCategories(ctx, userID, limit, offset)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, categories, http.StatusOK)
}

// ReactivateCategory handles POST /categories/{id}/reactivate.
func (h *Handler) ReactivateCategory(w http.ResponseWriter, r *http.Request) {
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

	cat, err := h.service.reactivateCategory(ctx, id, userID)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, cat, http.StatusOK)
}
