package task

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

// Handler handles HTTP requests for tasks.
type Handler struct {
	service   taskService
	logger    taskLogger
	validate  *validator.Validate
	sanitiser *bluemonday.Policy
}

// NewTaskHandler creates a new Handler.
func NewTaskHandler(service taskService, log taskLogger) *Handler {
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
	case errors.Is(err, ErrTaskNotFound):
		http.Error(w, "task not found", http.StatusNotFound)
	case errors.Is(err, ErrCategoryNotFound):
		http.Error(w, "category not found", http.StatusNotFound)
	case errors.Is(err, ErrInvalidAnswerType):
		http.Error(w, "invalid answer_type: must be 'string', 'integer', 'boolean', or 'select'", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidRecurrenceType):
		http.Error(w, "invalid recurrence_type", http.StatusBadRequest)
	case errors.Is(err, ErrMissingSelectOptions):
		http.Error(w, "select options required for answer_type 'select' (2-10 options)", http.StatusBadRequest)
	case errors.Is(err, ErrTooManySelectOptions):
		http.Error(w, "too many select options (maximum 10)", http.StatusBadRequest)
	case errors.Is(err, ErrTooFewSelectOptions):
		http.Error(w, "too few select options (minimum 2)", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidSchedule):
		http.Error(w, "invalid schedule configuration", http.StatusBadRequest)
	case errors.Is(err, ErrNameTooLong):
		http.Error(w, "name exceeds maximum of 200 characters", http.StatusBadRequest)
	case errors.Is(err, ErrDescriptionTooLong):
		http.Error(w, "description exceeds maximum of 1000 characters", http.StatusBadRequest)
	case errors.Is(err, ErrOptionValueTooLong):
		http.Error(w, "option value exceeds maximum of 100 characters", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidStartDate):
		http.Error(w, "invalid start_date: use YYYY-MM-DD format", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidEndDate):
		http.Error(w, "invalid end_date: use YYYY-MM-DD format", http.StatusBadRequest)
	case errors.Is(err, ErrInvalidScheduledTime):
		http.Error(w, "invalid scheduled_time: use HH:MM format", http.StatusBadRequest)
	case errors.Is(err, ErrMissingRecurrenceInterval):
		http.Error(w, "recurrence_interval required for 'every_n_days' or 'every_n_weeks'", http.StatusBadRequest)
	case errors.Is(err, ErrMissingDaysOfWeek):
		http.Error(w, "days_of_week required for 'weekly' or 'every_n_weeks'", http.StatusBadRequest)
	case errors.Is(err, ErrMissingMonthDay):
		http.Error(w, "month_day required for 'monthly_date' or 'yearly'", http.StatusBadRequest)
	case errors.Is(err, ErrMissingMonthlyWeekdayFields):
		http.Error(w, "month_week and month_weekday required for 'monthly_weekday'", http.StatusBadRequest)
	case errors.Is(err, ErrMissingMonthOfYear):
		http.Error(w, "month_of_year required for 'yearly'", http.StatusBadRequest)
	case errors.Is(err, ErrMissingEndDate):
		http.Error(w, "end_date required for end_type 'on_date'", http.StatusBadRequest)
	case errors.Is(err, ErrMissingEndAfterN):
		http.Error(w, "end_after_n required for end_type 'after_n'", http.StatusBadRequest)
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

// GetTask handles GET /tasks/{id}.
func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
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

	task, err := h.service.getTask(ctx, id, userID)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, task, http.StatusOK)
}

// ListTasks handles GET /tasks.
func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	limitStr := h.sanitise(r.URL.Query().Get("limit"))
	offsetStr := h.sanitise(r.URL.Query().Get("offset"))
	categoryID := h.sanitise(r.URL.Query().Get("category_id"))
	if categoryID != "" && !shared.IsValidUUID(categoryID) {
		http.Error(w, "invalid category_id: must be a valid UUID", http.StatusBadRequest)
		return
	}
	activeStr := h.sanitise(r.URL.Query().Get("active"))

	limit, offset, err := shared.ValidatePagination(limitStr, offsetStr)
	if err != nil {
		h.handleError(ctx, w, ErrMissingParameters)
		return
	}

	// Default to active tasks only
	isActive := activeStr != "false" && activeStr != "0"

	var catIDPtr *string
	if categoryID != "" {
		catIDPtr = &categoryID
	}

	tasks, err := h.service.getTasks(ctx, userID, catIDPtr, isActive, limit, offset)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, tasks, http.StatusOK)
}

// CreateTask handles POST /tasks.
func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		h.handleError(ctx, w, ErrUnauthorised)
		return
	}

	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.handleError(ctx, w, ErrInvalidReqBody)
		return
	}

	// Sanitise all fields before validation
	req.CategoryID = h.sanitise(req.CategoryID)
	req.Name = h.sanitise(req.Name)
	req.Description = h.sanitisePtr(req.Description)
	req.AnswerType = h.sanitise(req.AnswerType)
	req.Schedule.RecurrenceType = h.sanitise(req.Schedule.RecurrenceType)
	req.Schedule.StartDate = h.sanitise(req.Schedule.StartDate)
	req.Schedule.EndType = h.sanitise(req.Schedule.EndType)
	req.Schedule.EndDate = h.sanitisePtr(req.Schedule.EndDate)
	for i := range req.Schedule.ScheduledTimes {
		req.Schedule.ScheduledTimes[i] = h.sanitise(req.Schedule.ScheduledTimes[i])
	}
	for i := range req.SelectOptions {
		req.SelectOptions[i].Value = h.sanitise(req.SelectOptions[i].Value)
	}

	if err := h.validate.Struct(req); err != nil {
		h.handleError(ctx, w, ErrValidation)
		return
	}

	task, err := h.service.createTask(ctx, userID, req)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, task, http.StatusCreated)
}

// UpdateTask handles PUT /tasks/{id}.
func (h *Handler) UpdateTask(w http.ResponseWriter, r *http.Request) {
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

	var req UpdateTaskRequest
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

	task, err := h.service.updateTask(ctx, id, userID, req)
	if err != nil {
		h.handleError(ctx, w, err)
		return
	}

	h.responseJSON(ctx, w, task, http.StatusOK)
}

// DeleteTask handles DELETE /tasks/{id}.
func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.deleteTask(ctx, id, userID); err != nil {
		h.handleError(ctx, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
