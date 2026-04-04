package log

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Repository size limits (defence-in-depth validation).
const (
	repoMaxUserIDLength   = 64
	repoMaxLogContentSize = 1048576 // 1MB
)

// Prepared statement queries.
const (
	queryGetLog    = `SELECT id, user_id, date_and_time, log, created_at, updated_at FROM logs WHERE id = $1 AND user_id = $2`
	queryGetLogs   = `SELECT id, user_id, date_and_time, log, created_at, updated_at FROM logs WHERE user_id = $1 AND date_and_time >= $2 AND date_and_time <= $3 ORDER BY date_and_time DESC LIMIT $4 OFFSET $5`
	queryCreateLog = `INSERT INTO logs (user_id, date_and_time, log) VALUES ($1, $2, $3) RETURNING id, user_id, date_and_time, log, created_at, updated_at`
	queryUpdateLog = `UPDATE logs SET log = $1, updated_at = NOW() WHERE id = $2 AND user_id = $3 RETURNING id, user_id, date_and_time, log, created_at, updated_at`
	queryDeleteLog = `DELETE FROM logs WHERE id = $1 AND user_id = $2`
)

// logRepository defines the interface for log data access.
type logRepository interface {
	getLog(ctx context.Context, id, userID string) (Log, error)
	getLogs(ctx context.Context, userID, startDate, endDate string, limit, offset int) ([]Log, error)
	createLog(ctx context.Context, userID, dateAndTime, logContent string) (Log, error)
	updateLog(ctx context.Context, id, userID, logContent string) (Log, error)
	deleteLog(ctx context.Context, id, userID string) error
	Close() error
}

// sqlLogRepository implements logRepository using a SQL database.
type sqlLogRepository struct {
	stmtGetLog    *sql.Stmt
	stmtGetLogs   *sql.Stmt
	stmtCreateLog *sql.Stmt
	stmtUpdateLog *sql.Stmt
	stmtDeleteLog *sql.Stmt
}

// NewLogRepository creates a new logRepository with prepared statements.
// Panics if any statement cannot be prepared — this is a fatal startup error.
func NewLogRepository(db *sql.DB, _ logLogger) logRepository {
	repo := &sqlLogRepository{}

	var err error

	repo.stmtGetLog, err = db.Prepare(queryGetLog)
	if err != nil {
		panic(fmt.Sprintf("log_repository: failed to prepare getLog: %v", err))
	}

	repo.stmtGetLogs, err = db.Prepare(queryGetLogs)
	if err != nil {
		panic(fmt.Sprintf("log_repository: failed to prepare getLogs: %v", err))
	}

	repo.stmtCreateLog, err = db.Prepare(queryCreateLog)
	if err != nil {
		panic(fmt.Sprintf("log_repository: failed to prepare createLog: %v", err))
	}

	repo.stmtUpdateLog, err = db.Prepare(queryUpdateLog)
	if err != nil {
		panic(fmt.Sprintf("log_repository: failed to prepare updateLog: %v", err))
	}

	repo.stmtDeleteLog, err = db.Prepare(queryDeleteLog)
	if err != nil {
		panic(fmt.Sprintf("log_repository: failed to prepare deleteLog: %v", err))
	}

	return repo
}

// Close closes all prepared statements.
func (r *sqlLogRepository) Close() error {
	var errs []error
	for _, stmt := range []*sql.Stmt{
		r.stmtGetLog,
		r.stmtGetLogs,
		r.stmtCreateLog,
		r.stmtUpdateLog,
		r.stmtDeleteLog,
	} {
		if err := stmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *sqlLogRepository) getLog(ctx context.Context, id, userID string) (Log, error) {
	if len(userID) > repoMaxUserIDLength {
		return Log{}, ErrInvalidInput
	}

	var entry Log
	err := r.stmtGetLog.QueryRowContext(ctx, id, userID).Scan(
		&entry.ID, &entry.UserID, &entry.DateAndTime, &entry.Log,
		&entry.CreatedAt, &entry.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Log{}, ErrLogNotFound
		}
		// Wrap with context for traceability; sentinel preserved for errors.Is matching.
		return Log{}, fmt.Errorf("getLog %s: %w", id, ErrDatabase)
	}

	// Validate database output before use.
	if err := entry.Validate(); err != nil {
		return Log{}, fmt.Errorf("getLog validate %s: %w", id, ErrDatabase)
	}
	return entry, nil
}

func (r *sqlLogRepository) getLogs(ctx context.Context, userID, startDate, endDate string, limit, offset int) ([]Log, error) {
	if len(userID) > repoMaxUserIDLength {
		return []Log{}, nil
	}

	rows, err := r.stmtGetLogs.QueryContext(ctx, userID, startDate, endDate, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getLogs: %w", ErrDatabase)
	}
	defer func() { _ = rows.Close() }()

	var logs []Log
	for rows.Next() {
		var entry Log
		if err := rows.Scan(
			&entry.ID, &entry.UserID, &entry.DateAndTime, &entry.Log,
			&entry.CreatedAt, &entry.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("getLogs scan: %w", ErrDatabase)
		}
		// Validate database output before use.
		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("getLogs validate: %w", ErrDatabase)
		}
		logs = append(logs, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getLogs rows: %w", ErrDatabase)
	}

	if len(logs) == 0 {
		return []Log{}, nil
	}
	return logs, nil
}

func (r *sqlLogRepository) createLog(ctx context.Context, userID, dateAndTime, logContent string) (Log, error) {
	if len(userID) > repoMaxUserIDLength {
		return Log{}, fmt.Errorf("createLog: %w", ErrDatabase)
	}
	if len(logContent) > repoMaxLogContentSize {
		return Log{}, fmt.Errorf("createLog: %w", ErrDatabase)
	}

	var entry Log
	err := r.stmtCreateLog.QueryRowContext(ctx, userID, dateAndTime, logContent).Scan(
		&entry.ID, &entry.UserID, &entry.DateAndTime, &entry.Log,
		&entry.CreatedAt, &entry.UpdatedAt,
	)
	if err != nil {
		return Log{}, fmt.Errorf("createLog: %w", ErrDatabase)
	}

	// Validate database output before use.
	if err := entry.Validate(); err != nil {
		return Log{}, fmt.Errorf("createLog validate: %w", ErrDatabase)
	}
	return entry, nil
}

func (r *sqlLogRepository) updateLog(ctx context.Context, id, userID, logContent string) (Log, error) {
	if len(userID) > repoMaxUserIDLength {
		return Log{}, ErrLogNotFound
	}
	if len(logContent) > repoMaxLogContentSize {
		return Log{}, fmt.Errorf("updateLog: %w", ErrDatabase)
	}

	var entry Log
	err := r.stmtUpdateLog.QueryRowContext(ctx, logContent, id, userID).Scan(
		&entry.ID, &entry.UserID, &entry.DateAndTime, &entry.Log,
		&entry.CreatedAt, &entry.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Log{}, ErrLogNotFound
		}
		return Log{}, fmt.Errorf("updateLog %s: %w", id, ErrDatabase)
	}

	// Validate database output before use.
	if err := entry.Validate(); err != nil {
		return Log{}, fmt.Errorf("updateLog validate %s: %w", id, ErrDatabase)
	}
	return entry, nil
}

func (r *sqlLogRepository) deleteLog(ctx context.Context, id, userID string) error {
	if len(userID) > repoMaxUserIDLength {
		return ErrLogNotFound
	}

	result, err := r.stmtDeleteLog.ExecContext(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("deleteLog %s: %w", id, ErrDatabase)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("deleteLog rowsAffected %s: %w", id, ErrDatabase)
	}

	if rowsAffected == 0 {
		return ErrLogNotFound
	}
	return nil
}
