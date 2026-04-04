package dailylog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Repository size limits (defence-in-depth validation).
const (
	repoMaxUserIDLength = 64
	repoMaxEntryLength  = 10000
)

// Prepared statement queries.
const (
	queryGetDailyLog = `SELECT id, user_id, log_date, entry, created_at, updated_at
		FROM daily_logs WHERE id = $1 AND user_id = $2`
	queryGetDailyLogByDate = `SELECT id, user_id, log_date, entry, created_at, updated_at
		FROM daily_logs WHERE user_id = $1 AND log_date = $2`
	queryGetDailyLogsByDateRange = `SELECT id, user_id, log_date, entry, created_at, updated_at
		FROM daily_logs WHERE user_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC`
	queryCreateDailyLog = `INSERT INTO daily_logs (user_id, log_date, entry) VALUES ($1, $2, $3)
		RETURNING id, user_id, log_date, entry, created_at, updated_at`
	queryUpdateDailyLog = `UPDATE daily_logs SET entry = $1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3
		RETURNING id, user_id, log_date, entry, created_at, updated_at`
)

// dailylogRepository defines the interface for daily log data access.
type dailylogRepository interface {
	getDailyLog(ctx context.Context, id, userID string) (DailyLog, error)
	getDailyLogByDate(ctx context.Context, userID string, date time.Time) (DailyLog, error)
	getDailyLogsByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]DailyLog, error)
	createDailyLog(ctx context.Context, userID string, date time.Time, entry string) (DailyLog, error)
	updateDailyLog(ctx context.Context, id, userID, entry string) (DailyLog, error)
	Close() error
}

// sqlDailyLogRepository implements dailylogRepository using a SQL database.
type sqlDailyLogRepository struct {
	stmtGetDailyLog         *sql.Stmt
	stmtGetDailyLogByDate   *sql.Stmt
	stmtGetDailyLogsByRange *sql.Stmt
	stmtCreateDailyLog      *sql.Stmt
	stmtUpdateDailyLog      *sql.Stmt
}

// NewDailyLogRepository creates a new dailylogRepository with prepared statements.
func NewDailyLogRepository(db *sql.DB, _ dailylogLogger) dailylogRepository {
	repo := &sqlDailyLogRepository{}

	var err error

	repo.stmtGetDailyLog, err = db.Prepare(queryGetDailyLog)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare getDailyLog: %v", err))
	}

	repo.stmtGetDailyLogByDate, err = db.Prepare(queryGetDailyLogByDate)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare getDailyLogByDate: %v", err))
	}

	repo.stmtGetDailyLogsByRange, err = db.Prepare(queryGetDailyLogsByDateRange)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare getDailyLogsByRange: %v", err))
	}

	repo.stmtCreateDailyLog, err = db.Prepare(queryCreateDailyLog)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare createDailyLog: %v", err))
	}

	repo.stmtUpdateDailyLog, err = db.Prepare(queryUpdateDailyLog)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare updateDailyLog: %v", err))
	}

	return repo
}

// Close closes all prepared statements.
func (r *sqlDailyLogRepository) Close() error {
	var errs []error
	for _, stmt := range []*sql.Stmt{
		r.stmtGetDailyLog,
		r.stmtGetDailyLogByDate,
		r.stmtGetDailyLogsByRange,
		r.stmtCreateDailyLog,
		r.stmtUpdateDailyLog,
	} {
		if err := stmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *sqlDailyLogRepository) getDailyLog(ctx context.Context, id, userID string) (DailyLog, error) {
	if len(userID) > repoMaxUserIDLength {
		return DailyLog{}, ErrInvalidInput
	}

	var d DailyLog
	err := r.stmtGetDailyLog.QueryRowContext(ctx, id, userID).Scan(
		&d.ID, &d.UserID, &d.LogDate, &d.Entry,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DailyLog{}, ErrDailyLogNotFound
		}
		return DailyLog{}, fmt.Errorf("getDailyLog %s: %w", id, ErrDatabase)
	}

	if err := d.Validate(); err != nil {
		return DailyLog{}, fmt.Errorf("getDailyLog validate %s: %w", id, ErrDatabase)
	}
	return d, nil
}

func (r *sqlDailyLogRepository) getDailyLogByDate(ctx context.Context, userID string, date time.Time) (DailyLog, error) {
	if len(userID) > repoMaxUserIDLength {
		return DailyLog{}, ErrDailyLogNotFound
	}

	var d DailyLog
	err := r.stmtGetDailyLogByDate.QueryRowContext(ctx, userID, date).Scan(
		&d.ID, &d.UserID, &d.LogDate, &d.Entry,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DailyLog{}, ErrDailyLogNotFound
		}
		return DailyLog{}, fmt.Errorf("getDailyLogByDate: %w", ErrDatabase)
	}

	if err := d.Validate(); err != nil {
		return DailyLog{}, fmt.Errorf("getDailyLogByDate validate: %w", ErrDatabase)
	}
	return d, nil
}

func (r *sqlDailyLogRepository) getDailyLogsByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]DailyLog, error) {
	if len(userID) > repoMaxUserIDLength {
		return []DailyLog{}, nil
	}

	rows, err := r.stmtGetDailyLogsByRange.QueryContext(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("getDailyLogsByDateRange: %w", ErrDatabase)
	}
	defer func() { _ = rows.Close() }()

	var logs []DailyLog
	for rows.Next() {
		var d DailyLog
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.LogDate, &d.Entry,
			&d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("getDailyLogsByDateRange scan: %w", ErrDatabase)
		}
		if err := d.Validate(); err != nil {
			return nil, fmt.Errorf("getDailyLogsByDateRange validate: %w", ErrDatabase)
		}
		logs = append(logs, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getDailyLogsByDateRange rows: %w", ErrDatabase)
	}

	if len(logs) == 0 {
		return []DailyLog{}, nil
	}
	return logs, nil
}

func (r *sqlDailyLogRepository) createDailyLog(ctx context.Context, userID string, date time.Time, entry string) (DailyLog, error) {
	if len(userID) > repoMaxUserIDLength {
		return DailyLog{}, fmt.Errorf("createDailyLog: %w", ErrDatabase)
	}
	if len(entry) > repoMaxEntryLength {
		return DailyLog{}, ErrEntryTooLong
	}

	var d DailyLog
	err := r.stmtCreateDailyLog.QueryRowContext(ctx, userID, date, entry).Scan(
		&d.ID, &d.UserID, &d.LogDate, &d.Entry,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		// Check for unique constraint violation
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			return DailyLog{}, ErrDailyLogExists
		}
		return DailyLog{}, fmt.Errorf("createDailyLog: %w", ErrDatabase)
	}

	if err := d.Validate(); err != nil {
		return DailyLog{}, fmt.Errorf("createDailyLog validate: %w", ErrDatabase)
	}
	return d, nil
}

func (r *sqlDailyLogRepository) updateDailyLog(ctx context.Context, id, userID, entry string) (DailyLog, error) {
	if len(userID) > repoMaxUserIDLength {
		return DailyLog{}, ErrDailyLogNotFound
	}
	if len(entry) > repoMaxEntryLength {
		return DailyLog{}, ErrEntryTooLong
	}

	var d DailyLog
	err := r.stmtUpdateDailyLog.QueryRowContext(ctx, entry, id, userID).Scan(
		&d.ID, &d.UserID, &d.LogDate, &d.Entry,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DailyLog{}, ErrDailyLogNotFound
		}
		return DailyLog{}, fmt.Errorf("updateDailyLog %s: %w", id, ErrDatabase)
	}

	if err := d.Validate(); err != nil {
		return DailyLog{}, fmt.Errorf("updateDailyLog validate %s: %w", id, ErrDatabase)
	}
	return d, nil
}
