package log

import (
	"context"
	"fmt"
	"time"

	"go-tasks-api/internal/shared"
)

const (
	defaultLimit  = 100
	defaultOffset = 0
)

// logService defines the interface for log business logic.
type logService interface {
	getLog(ctx context.Context, id, userID string) (Log, error)
	getLogs(ctx context.Context, userID, startDate, endDate string, limit, offset int) ([]Log, error)
	createLog(ctx context.Context, userID string, req Request) (Log, error)
	updateLog(ctx context.Context, id, userID string, req Request) (Log, error)
	deleteLog(ctx context.Context, id, userID string) error
}

// defaultLogService implements logService.
type defaultLogService struct {
	repo logRepository
}

// NewLogService creates a new logService.
func NewLogService(repo logRepository, _ logLogger) *defaultLogService {
	return &defaultLogService{repo: repo}
}

func (s *defaultLogService) getLog(ctx context.Context, id, userID string) (Log, error) {
	if id == "" || userID == "" {
		return Log{}, ErrMissingParameters
	}
	return s.repo.getLog(ctx, id, userID)
}

func (s *defaultLogService) getLogs(ctx context.Context, userID, startDate, endDate string, limit, offset int) ([]Log, error) {
	if userID == "" {
		return nil, ErrMissingParameters
	}

	if limit <= 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = defaultOffset
	}

	// Default to a wide range when not provided.
	// The handler documents these defaults to callers via the OpenAPI spec.
	if startDate == "" {
		startDate = "1970-01-01T00:00:00Z"
	}
	if endDate == "" {
		endDate = "2099-12-31T23:59:59Z"
	}

	if _, err := time.Parse(time.RFC3339, startDate); err != nil {
		return nil, fmt.Errorf("getLogs start_date: %w", ErrInvalidDateTime)
	}
	if _, err := time.Parse(time.RFC3339, endDate); err != nil {
		return nil, fmt.Errorf("getLogs end_date: %w", ErrInvalidDateTime)
	}

	return s.repo.getLogs(ctx, userID, startDate, endDate, limit, offset)
}

func (s *defaultLogService) createLog(ctx context.Context, userID string, req Request) (Log, error) {
	if userID == "" {
		return Log{}, ErrMissingParameters
	}

	if len(req.Log) > shared.LogMaxChars {
		return Log{}, NewLogTooLongError(len(req.Log))
	}

	t, err := time.Parse(time.RFC3339, req.DateAndTime)
	if err != nil {
		return Log{}, fmt.Errorf("createLog date_and_time: %w", ErrInvalidDateTime)
	}
	if t.Year() > 9000 {
		return Log{}, fmt.Errorf("createLog date_and_time: %w", ErrDateTimeOutOfRange)
	}

	return s.repo.createLog(ctx, userID, req.DateAndTime, req.Log)
}

func (s *defaultLogService) updateLog(ctx context.Context, id, userID string, req Request) (Log, error) {
	if id == "" || userID == "" {
		return Log{}, ErrMissingParameters
	}

	if len(req.Log) > shared.LogMaxChars {
		return Log{}, NewLogTooLongError(len(req.Log))
	}

	return s.repo.updateLog(ctx, id, userID, req.Log)
}

func (s *defaultLogService) deleteLog(ctx context.Context, id, userID string) error {
	if id == "" || userID == "" {
		return ErrMissingParameters
	}
	return s.repo.deleteLog(ctx, id, userID)
}
