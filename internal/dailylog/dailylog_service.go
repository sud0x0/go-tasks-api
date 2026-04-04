package dailylog

import (
	"context"
	"time"
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
	createDailyLog(ctx context.Context, userID string, req CreateRequest) (DailyLog, error)
	updateDailyLog(ctx context.Context, id, userID string, req UpdateRequest) (DailyLog, error)
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

func (s *defaultDailyLogService) createDailyLog(ctx context.Context, userID string, req CreateRequest) (DailyLog, error) {
	if userID == "" {
		return DailyLog{}, ErrMissingParameters
	}

	if len(req.Entry) > maxEntryLength {
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

	if len(req.Entry) > maxEntryLength {
		return DailyLog{}, ErrEntryTooLong
	}

	return s.repo.updateDailyLog(ctx, id, userID, req.Entry)
}
