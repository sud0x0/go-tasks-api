package dailylog

import (
	"context"
	"time"

	"go-tasks-api/internal/shared"

	"github.com/google/uuid"
)

// Field limits.
const (
	maxEntryLength = 10000
)

// dailylogService defines the interface for daily log business logic.
type dailylogService interface {
	getDailyLog(ctx context.Context, id, userID string) (DailyLog, error)
	getDailyLogByDate(ctx context.Context, userID string, date time.Time) (DailyLog, error)
	getDailyLogsByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]DailyLog, error)
	getInactiveDailyLogs(ctx context.Context, userID string, limit, offset int) ([]DailyLog, error)
	createDailyLog(ctx context.Context, userID string, req CreateRequest) (DailyLog, error)
	updateDailyLog(ctx context.Context, id, userID string, req UpdateRequest) (DailyLog, error)
	deleteDailyLog(ctx context.Context, id, userID string) error
	permanentDeleteDailyLog(ctx context.Context, id, userID string) error
	bulkDeleteDailyLogs(ctx context.Context, userID string, ids []string) (int, int, error)
	bulkPermanentDeleteDailyLogs(ctx context.Context, userID string, ids []string) (int, int, error)
	reactivateDailyLog(ctx context.Context, id, userID string) (DailyLog, error)
}

// defaultDailyLogService implements dailylogService.
type defaultDailyLogService struct {
	repo dailylogRepository
}

// NewDailyLogService creates a new dailylogService.
func NewDailyLogService(repo dailylogRepository, _ dailylogLogger) *defaultDailyLogService {
	return &defaultDailyLogService{repo: repo}
}

func (s *defaultDailyLogService) getDailyLog(ctx context.Context, id, userID string) (DailyLog, error) {
	if id == "" || userID == "" {
		return DailyLog{}, ErrMissingParameters
	}
	return s.repo.getDailyLog(ctx, id, userID)
}

func (s *defaultDailyLogService) getDailyLogByDate(ctx context.Context, userID string, date time.Time) (DailyLog, error) {
	if userID == "" {
		return DailyLog{}, ErrMissingParameters
	}
	return s.repo.getDailyLogByDate(ctx, userID, date)
}

func (s *defaultDailyLogService) getDailyLogsByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]DailyLog, error) {
	if userID == "" {
		return nil, ErrMissingParameters
	}

	if startDate.After(endDate) {
		return nil, ErrInvalidDateRange
	}

	return s.repo.getDailyLogsByDateRange(ctx, userID, startDate, endDate)
}

func (s *defaultDailyLogService) getInactiveDailyLogs(ctx context.Context, userID string, limit, offset int) ([]DailyLog, error) {
	if userID == "" {
		return nil, ErrMissingParameters
	}

	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	return s.repo.getInactiveDailyLogs(ctx, userID, limit, offset)
}

func (s *defaultDailyLogService) createDailyLog(ctx context.Context, userID string, req CreateRequest) (DailyLog, error) {
	if userID == "" {
		return DailyLog{}, ErrMissingParameters
	}

	if shared.RuneCountLen(req.Entry) > maxEntryLength {
		return DailyLog{}, ErrEntryTooLong
	}

	// Parse the date
	date, err := time.Parse("2006-01-02", req.LogDate)
	if err != nil {
		return DailyLog{}, ErrInvalidDate
	}

	return s.repo.createDailyLog(ctx, userID, date, req.Entry)
}

func (s *defaultDailyLogService) updateDailyLog(ctx context.Context, id, userID string, req UpdateRequest) (DailyLog, error) {
	if id == "" || userID == "" {
		return DailyLog{}, ErrMissingParameters
	}

	if shared.RuneCountLen(req.Entry) > maxEntryLength {
		return DailyLog{}, ErrEntryTooLong
	}

	return s.repo.updateDailyLog(ctx, id, userID, req.Entry)
}

// deleteDailyLog performs soft-delete only.
// Returns ErrDailyLogNotFound if the daily log does not exist.
// Returns ErrDailyLogAlreadyInactive (409) if the daily log is already inactive.
func (s *defaultDailyLogService) deleteDailyLog(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrMissingParameters
	}

	// Check existence and get current status
	isActive, err := s.repo.getDailyLogIsActive(ctx, id, userID)
	if err != nil {
		return err // returns ErrDailyLogNotFound if not found
	}

	if !isActive {
		return ErrDailyLogAlreadyInactive
	}

	// Soft delete: deactivate the daily log
	return s.repo.deactivateDailyLog(ctx, id, userID)
}

// permanentDeleteDailyLog performs hard-delete on an inactive daily log.
// Returns ErrDailyLogNotFound if the daily log does not exist.
// Returns ErrCannotPermanentDeleteActiveDailyLog (409) if the daily log is still active.
func (s *defaultDailyLogService) permanentDeleteDailyLog(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrMissingParameters
	}

	// Check existence and get current status
	isActive, err := s.repo.getDailyLogIsActive(ctx, id, userID)
	if err != nil {
		return err // returns ErrDailyLogNotFound if not found
	}

	if isActive {
		return ErrCannotPermanentDeleteActiveDailyLog
	}

	// Hard delete: daily log is inactive
	return s.repo.hardDeleteDailyLog(ctx, id, userID)
}

// bulkDeleteDailyLogs performs bulk soft-delete only.
// Inactive IDs in the list are ignored (not hard-deleted).
// Returns (requested, softDeleted, error) where requested is the pre-dedup input length.
func (s *defaultDailyLogService) bulkDeleteDailyLogs(ctx context.Context, userID string, ids []string) (int, int, error) {
	requested := len(ids)
	if userID == "" {
		return 0, 0, ErrMissingParameters
	}

	if requested == 0 {
		return 0, 0, ErrEmptyIDList
	}

	if requested > 100 {
		return 0, 0, ErrTooManyIDs
	}

	// Deduplicate IDs while preserving order
	seen := make(map[string]struct{}, len(ids))
	validIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return 0, 0, ErrInvalidInput
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			validIDs = append(validIDs, id)
		}
	}

	// Soft delete active daily logs only
	softDeleted, err := s.repo.bulkDeactivateDailyLogs(ctx, userID, validIDs)
	if err != nil {
		return requested, 0, err
	}

	return requested, softDeleted, nil
}

// bulkPermanentDeleteDailyLogs performs bulk hard-delete on inactive daily logs only.
// Active IDs in the list are ignored.
// Returns (requested, permanentlyDeleted, error) where requested is the pre-dedup input length.
func (s *defaultDailyLogService) bulkPermanentDeleteDailyLogs(ctx context.Context, userID string, ids []string) (int, int, error) {
	requested := len(ids)
	if userID == "" {
		return 0, 0, ErrMissingParameters
	}

	if requested == 0 {
		return 0, 0, ErrEmptyIDList
	}

	if requested > 100 {
		return 0, 0, ErrTooManyIDs
	}

	// Deduplicate IDs while preserving order
	seen := make(map[string]struct{}, len(ids))
	validIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return 0, 0, ErrInvalidInput
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			validIDs = append(validIDs, id)
		}
	}

	// Hard delete inactive daily logs only
	permanentlyDeleted, err := s.repo.bulkHardDeleteDailyLogs(ctx, userID, validIDs)
	if err != nil {
		return requested, 0, err
	}

	return requested, permanentlyDeleted, nil
}

func (s *defaultDailyLogService) reactivateDailyLog(ctx context.Context, id, userID string) (DailyLog, error) {
	if id == "" || userID == "" {
		return DailyLog{}, ErrMissingParameters
	}

	// Check existence and get current status
	isActive, err := s.repo.getDailyLogIsActive(ctx, id, userID)
	if err != nil {
		return DailyLog{}, err // returns ErrDailyLogNotFound if not found
	}

	if isActive {
		return DailyLog{}, ErrDailyLogAlreadyActive
	}

	return s.repo.reactivateDailyLog(ctx, id, userID)
}
