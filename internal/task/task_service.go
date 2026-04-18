package task

import (
	"context"
	"errors"
	"regexp"
	"time"

	"go-tasks-api/internal/shared"

	"github.com/google/uuid"
)

// Field limits.
const (
	maxNameLength        = 200
	maxDescriptionLength = 1000
	maxOptionValueLength = 100
	minSelectOptions     = 2
	maxSelectOptions     = 10
	maxBulkIDs           = 100
)

// timeRegex validates HH:MM format.
var timeRegex = regexp.MustCompile(`^([01]?[0-9]|2[0-3]):[0-5][0-9]$`)

// taskService defines the interface for task business logic.
type taskService interface {
	getTask(ctx context.Context, id, userID string) (WithDetails, error)
	getTasks(ctx context.Context, userID string, categoryID *string, isActive bool, limit, offset int) ([]Task, error)
	getInactiveTasks(ctx context.Context, userID string, limit, offset int) ([]Task, error)
	createTask(ctx context.Context, userID string, req CreateTaskRequest) (WithDetails, error)
	updateTask(ctx context.Context, id, userID string, req UpdateTaskRequest) (Task, error)
	deleteTask(ctx context.Context, id, userID string) error
	permanentDeleteTask(ctx context.Context, id, userID string) error
	bulkDeleteTasks(ctx context.Context, userID string, ids []string) (int, int, error)
	bulkPermanentDeleteTasks(ctx context.Context, userID string, ids []string) (int, int, error)
	reactivateTask(ctx context.Context, id, userID string) (Task, error)
}

// defaultTaskService implements taskService.
type defaultTaskService struct {
	repo   taskRepository
	logger taskLogger
}

// NewTaskService creates a new taskService.
func NewTaskService(repo taskRepository, log taskLogger) *defaultTaskService {
	return &defaultTaskService{repo: repo, logger: log}
}

func (s *defaultTaskService) getTask(ctx context.Context, id, userID string) (WithDetails, error) {
	if id == "" || userID == "" {
		return WithDetails{}, ErrMissingParameters
	}

	task, err := s.repo.getTask(ctx, id, userID)
	if err != nil {
		return WithDetails{}, err
	}

	result := WithDetails{Task: task}

	// Get schedule
	schedule, err := s.repo.getScheduleByTaskID(ctx, id, userID)
	if err != nil {
		return WithDetails{}, err
	}
	result.Schedule = schedule

	// Get select options if answer_type is select
	if task.AnswerType == AnswerTypeSelect {
		options, err := s.repo.getSelectOptionsByTaskID(ctx, id, userID)
		if err != nil {
			return WithDetails{}, err
		}
		result.SelectOptions = options
	}

	return result, nil
}

func (s *defaultTaskService) getTasks(ctx context.Context, userID string, categoryID *string, isActive bool, limit, offset int) ([]Task, error) {
	if userID == "" {
		return nil, ErrMissingParameters
	}

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	if categoryID != nil && *categoryID != "" {
		return s.repo.getTasksByCategoryID(ctx, userID, *categoryID, isActive, limit, offset)
	}
	return s.repo.getTasks(ctx, userID, isActive, limit, offset)
}

func (s *defaultTaskService) getInactiveTasks(ctx context.Context, userID string, limit, offset int) ([]Task, error) {
	if userID == "" {
		return nil, ErrMissingParameters
	}

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	return s.repo.getInactiveTasks(ctx, userID, limit, offset)
}

func (s *defaultTaskService) createTask(ctx context.Context, userID string, req CreateTaskRequest) (WithDetails, error) {
	if userID == "" {
		return WithDetails{}, ErrMissingParameters
	}

	// Validate field lengths (rune count for Unicode-aware validation)
	if shared.RuneCountLen(req.Name) > maxNameLength {
		return WithDetails{}, ErrNameTooLong
	}
	if req.Description != nil && shared.RuneCountLen(*req.Description) > maxDescriptionLength {
		return WithDetails{}, ErrDescriptionTooLong
	}

	// Validate answer type
	if !IsValidAnswerType(req.AnswerType) {
		return WithDetails{}, ErrInvalidAnswerType
	}

	// Validate select options for select type
	if req.AnswerType == string(AnswerTypeSelect) {
		if len(req.SelectOptions) < minSelectOptions {
			return WithDetails{}, ErrTooFewSelectOptions
		}
		if len(req.SelectOptions) > maxSelectOptions {
			return WithDetails{}, ErrTooManySelectOptions
		}
		for _, opt := range req.SelectOptions {
			if shared.RuneCountLen(opt.Value) > maxOptionValueLength {
				return WithDetails{}, ErrOptionValueTooLong
			}
		}
	} else if len(req.SelectOptions) > 0 {
		// Select options provided for non-select type - ignore them
		req.SelectOptions = nil
	}

	// Validate recurrence type
	if !IsValidRecurrenceType(req.Schedule.RecurrenceType) {
		return WithDetails{}, ErrInvalidRecurrenceType
	}

	// Validate schedule fields based on recurrence type
	if err := s.validateSchedule(req.Schedule); err != nil {
		return WithDetails{}, err
	}

	// Verify category exists
	exists, err := s.repo.categoryExists(ctx, req.CategoryID, userID)
	if err != nil {
		s.logger.LogError(errors.New("categoryExists check failed"), err)
		return WithDetails{}, err
	}
	if !exists {
		return WithDetails{}, ErrCategoryNotFound
	}

	// Build schedule from request
	schedule := s.buildScheduleFromRequest(req.Schedule)

	// Create task, schedule, and options atomically
	return s.repo.createTaskWithScheduleAndOptions(ctx, userID, req.CategoryID, req.Name, req.Description, req.AnswerType, schedule, req.SelectOptions)
}

func (s *defaultTaskService) updateTask(ctx context.Context, id, userID string, req UpdateTaskRequest) (Task, error) {
	if id == "" || userID == "" {
		return Task{}, ErrMissingParameters
	}

	if shared.RuneCountLen(req.Name) > maxNameLength {
		return Task{}, ErrNameTooLong
	}
	if req.Description != nil && shared.RuneCountLen(*req.Description) > maxDescriptionLength {
		return Task{}, ErrDescriptionTooLong
	}

	return s.repo.updateTask(ctx, id, userID, req.Name, req.Description)
}

// deleteTask performs soft-delete only.
// Returns ErrTaskNotFound if the task does not exist.
// Returns ErrTaskAlreadyInactive (409) if the task is already inactive.
func (s *defaultTaskService) deleteTask(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrMissingParameters
	}

	// Check existence and get current status
	isActive, err := s.repo.getTaskIsActive(ctx, id, userID)
	if err != nil {
		return err // returns ErrTaskNotFound if not found
	}

	if !isActive {
		return ErrTaskAlreadyInactive
	}

	// Soft delete: deactivate the task
	return s.repo.deactivateTask(ctx, id, userID)
}

// permanentDeleteTask performs hard-delete on an inactive task.
// Returns ErrTaskNotFound if the task does not exist.
// Returns ErrCannotPermanentDeleteActiveTask (409) if the task is still active.
func (s *defaultTaskService) permanentDeleteTask(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrMissingParameters
	}

	// Check existence and get current status
	isActive, err := s.repo.getTaskIsActive(ctx, id, userID)
	if err != nil {
		return err // returns ErrTaskNotFound if not found
	}

	if isActive {
		return ErrCannotPermanentDeleteActiveTask
	}

	// Hard delete: task is inactive
	return s.repo.hardDeleteTask(ctx, id, userID)
}

// bulkDeleteTasks performs bulk soft-delete only.
// Inactive IDs in the list are ignored (not hard-deleted).
// Returns (requested, softDeleted, error) where requested is the pre-dedup input length.
func (s *defaultTaskService) bulkDeleteTasks(ctx context.Context, userID string, ids []string) (int, int, error) {
	requested := len(ids)
	if userID == "" {
		return 0, 0, ErrMissingParameters
	}
	if requested == 0 {
		return 0, 0, ErrEmptyIDList
	}
	if requested > maxBulkIDs {
		return 0, 0, ErrTooManyIDs
	}

	// Validate and deduplicate IDs
	seen := make(map[string]struct{}, len(ids))
	validIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return 0, 0, ErrInvalidInput
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			validIDs = append(validIDs, id)
		}
	}

	// Soft delete active tasks only
	softDeleted, err := s.repo.bulkDeactivateTasks(ctx, userID, validIDs)
	if err != nil {
		return requested, 0, err
	}

	return requested, softDeleted, nil
}

// bulkPermanentDeleteTasks performs bulk hard-delete on inactive tasks only.
// Active IDs in the list are ignored.
// Returns (requested, permanentlyDeleted, error) where requested is the pre-dedup input length.
func (s *defaultTaskService) bulkPermanentDeleteTasks(ctx context.Context, userID string, ids []string) (int, int, error) {
	requested := len(ids)
	if userID == "" {
		return 0, 0, ErrMissingParameters
	}
	if requested == 0 {
		return 0, 0, ErrEmptyIDList
	}
	if requested > maxBulkIDs {
		return 0, 0, ErrTooManyIDs
	}

	// Validate and deduplicate IDs
	seen := make(map[string]struct{}, len(ids))
	validIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return 0, 0, ErrInvalidInput
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			validIDs = append(validIDs, id)
		}
	}

	// Hard delete inactive tasks only
	permanentlyDeleted, err := s.repo.bulkHardDeleteTasks(ctx, userID, validIDs)
	if err != nil {
		return requested, 0, err
	}

	return requested, permanentlyDeleted, nil
}

func (s *defaultTaskService) reactivateTask(ctx context.Context, id, userID string) (Task, error) {
	if id == "" || userID == "" {
		return Task{}, ErrMissingParameters
	}

	// Check that the task's category is active
	categoryID, err := s.repo.getTaskCategoryID(ctx, id, userID)
	if err != nil {
		return Task{}, err
	}

	catActive, err := s.repo.categoryIsActive(ctx, categoryID, userID)
	if err != nil {
		return Task{}, err
	}
	if !catActive {
		return Task{}, ErrCategoryInactive
	}

	return s.repo.reactivateTask(ctx, id, userID)
}

func (s *defaultTaskService) validateSchedule(sched ScheduleRequest) error {
	// Parse and validate start date
	_, err := time.Parse("2006-01-02", sched.StartDate)
	if err != nil {
		return ErrInvalidStartDate
	}

	// Validate end type
	endType := EndType(sched.EndType)
	if endType == "" {
		endType = EndTypeNever
	}

	switch endType {
	case EndTypeOnDate:
		if sched.EndDate == nil || *sched.EndDate == "" {
			return ErrMissingEndDate
		}
		_, err := time.Parse("2006-01-02", *sched.EndDate)
		if err != nil {
			return ErrInvalidEndDate
		}
	case EndTypeAfterN:
		if sched.EndAfterN == nil || *sched.EndAfterN < 1 {
			return ErrMissingEndAfterN
		}
	case EndTypeNever:
		// No additional validation needed
	default:
		return ErrInvalidSchedule
	}

	// Validate scheduled times format
	for _, t := range sched.ScheduledTimes {
		if !timeRegex.MatchString(t) {
			return ErrInvalidScheduledTime
		}
	}

	// Validate recurrence-specific fields
	recType := RecurrenceType(sched.RecurrenceType)
	switch recType {
	case RecurrenceOnce, RecurrenceDaily:
		// No additional fields required

	case RecurrenceEveryNDays:
		if sched.RecurrenceInterval == nil || *sched.RecurrenceInterval < 1 {
			return ErrMissingRecurrenceInterval
		}

	case RecurrenceWeekly:
		if len(sched.DaysOfWeek) == 0 {
			return ErrMissingDaysOfWeek
		}

	case RecurrenceEveryNWeeks:
		if sched.RecurrenceInterval == nil || *sched.RecurrenceInterval < 1 {
			return ErrMissingRecurrenceInterval
		}
		if len(sched.DaysOfWeek) == 0 {
			return ErrMissingDaysOfWeek
		}

	case RecurrenceMonthlyDate:
		if sched.MonthDay == nil || *sched.MonthDay < 1 || *sched.MonthDay > 31 {
			return ErrMissingMonthDay
		}

	case RecurrenceMonthlyWeekday:
		if sched.MonthWeek == nil || *sched.MonthWeek < 1 || *sched.MonthWeek > 5 {
			return ErrMissingMonthlyWeekdayFields
		}
		if sched.MonthWeekday == nil || *sched.MonthWeekday < 0 || *sched.MonthWeekday > 6 {
			return ErrMissingMonthlyWeekdayFields
		}

	case RecurrenceYearly:
		if sched.MonthDay == nil || *sched.MonthDay < 1 || *sched.MonthDay > 31 {
			return ErrMissingMonthDay
		}
		if sched.MonthOfYear == nil || *sched.MonthOfYear < 1 || *sched.MonthOfYear > 12 {
			return ErrMissingMonthOfYear
		}
	}

	return nil
}

func (s *defaultTaskService) buildScheduleFromRequest(req ScheduleRequest) *Schedule {
	startDate, _ := time.Parse("2006-01-02", req.StartDate) // Already validated

	endType := EndType(req.EndType)
	if endType == "" {
		endType = EndTypeNever
	}

	var endDate *time.Time
	if req.EndDate != nil && *req.EndDate != "" {
		parsed, _ := time.Parse("2006-01-02", *req.EndDate) // Already validated
		endDate = &parsed
	}

	// Convert days of week to int64 slice
	var daysOfWeek []int64
	for _, d := range req.DaysOfWeek {
		daysOfWeek = append(daysOfWeek, int64(d))
	}

	return &Schedule{
		RecurrenceType:     RecurrenceType(req.RecurrenceType),
		RecurrenceInterval: req.RecurrenceInterval,
		DaysOfWeek:         daysOfWeek,
		MonthDay:           req.MonthDay,
		MonthWeek:          req.MonthWeek,
		MonthWeekday:       req.MonthWeekday,
		MonthOfYear:        req.MonthOfYear,
		ScheduledTimes:     req.ScheduledTimes,
		StartDate:          startDate,
		EndType:            endType,
		EndDate:            endDate,
		EndAfterN:          req.EndAfterN,
	}
}
