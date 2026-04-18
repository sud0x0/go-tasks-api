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
// Entry length validation is handled at the service layer using rune count
// to match PostgreSQL TEXT character semantics.
const (
	repoMaxUserIDLength = 64
)

// Prepared statement queries.
const (
	queryGetDailyLog = `SELECT id, user_id, log_date, entry, is_active, created_at, updated_at
		FROM daily_logs WHERE id = $1 AND user_id = $2 AND is_active = true`
	queryGetDailyLogByDate = `SELECT id, user_id, log_date, entry, is_active, created_at, updated_at
		FROM daily_logs WHERE user_id = $1 AND log_date = $2 AND is_active = true`
	queryGetDailyLogsByDateRange = `SELECT id, user_id, log_date, entry, is_active, created_at, updated_at
		FROM daily_logs WHERE user_id = $1 AND log_date >= $2 AND log_date <= $3 AND is_active = true
		ORDER BY log_date DESC`
	queryGetInactiveDailyLogs = `SELECT id, user_id, log_date, entry, is_active, created_at, updated_at
		FROM daily_logs WHERE user_id = $1 AND is_active = false
		ORDER BY log_date DESC LIMIT $2 OFFSET $3`
	queryCreateDailyLog = `INSERT INTO daily_logs (user_id, log_date, entry) VALUES ($1, $2, $3)
		RETURNING id, user_id, log_date, entry, is_active, created_at, updated_at`
	queryUpdateDailyLog = `UPDATE daily_logs SET entry = $1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3 AND is_active = true
		RETURNING id, user_id, log_date, entry, is_active, created_at, updated_at`
	queryDeactivateDailyLog = `UPDATE daily_logs SET is_active = false, updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND is_active = true`
	queryHardDeleteDailyLog  = `DELETE FROM daily_logs WHERE id = $1 AND user_id = $2 AND is_active = false`
	queryReactivateDailyLog  = `UPDATE daily_logs SET is_active = true, updated_at = NOW() WHERE id = $1 AND user_id = $2 AND is_active = false RETURNING id, user_id, log_date, entry, is_active, created_at, updated_at`
	queryGetDailyLogIsActive = `SELECT is_active FROM daily_logs WHERE id = $1 AND user_id = $2`
)

// dailylogRepository defines the interface for daily log data access.
type dailylogRepository interface {
	getDailyLog(ctx context.Context, id, userID string) (DailyLog, error)
	getDailyLogByDate(ctx context.Context, userID string, date time.Time) (DailyLog, error)
	getDailyLogsByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]DailyLog, error)
	getInactiveDailyLogs(ctx context.Context, userID string, limit, offset int) ([]DailyLog, error)
	createDailyLog(ctx context.Context, userID string, date time.Time, entry string) (DailyLog, error)
	updateDailyLog(ctx context.Context, id, userID, entry string) (DailyLog, error)
	deactivateDailyLog(ctx context.Context, id, userID string) error
	hardDeleteDailyLog(ctx context.Context, id, userID string) error
	bulkDeactivateDailyLogs(ctx context.Context, userID string, ids []string) (int, error)
	bulkHardDeleteDailyLogs(ctx context.Context, userID string, ids []string) (int, error)
	reactivateDailyLog(ctx context.Context, id, userID string) (DailyLog, error)
	getDailyLogIsActive(ctx context.Context, id, userID string) (bool, error)
	Close() error
}

// sqlDailyLogRepository implements dailylogRepository using a SQL database.
type sqlDailyLogRepository struct {
	db                       *sql.DB
	stmtGetDailyLog          *sql.Stmt
	stmtGetDailyLogByDate    *sql.Stmt
	stmtGetDailyLogsByRange  *sql.Stmt
	stmtGetInactiveDailyLogs *sql.Stmt
	stmtCreateDailyLog       *sql.Stmt
	stmtUpdateDailyLog       *sql.Stmt
	stmtDeactivateDailyLog   *sql.Stmt
	stmtHardDeleteDailyLog   *sql.Stmt
	stmtReactivateDailyLog   *sql.Stmt
	stmtGetDailyLogIsActive  *sql.Stmt
}

// NewDailyLogRepository creates a new dailylogRepository with prepared statements.
func NewDailyLogRepository(db *sql.DB, _ dailylogLogger) dailylogRepository {
	repo := &sqlDailyLogRepository{db: db}

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

	repo.stmtGetInactiveDailyLogs, err = db.Prepare(queryGetInactiveDailyLogs)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare getInactiveDailyLogs: %v", err))
	}

	repo.stmtCreateDailyLog, err = db.Prepare(queryCreateDailyLog)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare createDailyLog: %v", err))
	}

	repo.stmtUpdateDailyLog, err = db.Prepare(queryUpdateDailyLog)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare updateDailyLog: %v", err))
	}

	repo.stmtDeactivateDailyLog, err = db.Prepare(queryDeactivateDailyLog)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare deactivateDailyLog: %v", err))
	}

	repo.stmtHardDeleteDailyLog, err = db.Prepare(queryHardDeleteDailyLog)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare hardDeleteDailyLog: %v", err))
	}

	repo.stmtReactivateDailyLog, err = db.Prepare(queryReactivateDailyLog)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare reactivateDailyLog: %v", err))
	}

	repo.stmtGetDailyLogIsActive, err = db.Prepare(queryGetDailyLogIsActive)
	if err != nil {
		panic(fmt.Sprintf("dailylog_repository: failed to prepare getDailyLogIsActive: %v", err))
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
		r.stmtGetInactiveDailyLogs,
		r.stmtCreateDailyLog,
		r.stmtUpdateDailyLog,
		r.stmtDeactivateDailyLog,
		r.stmtHardDeleteDailyLog,
		r.stmtReactivateDailyLog,
		r.stmtGetDailyLogIsActive,
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
		&d.ID, &d.UserID, &d.LogDate, &d.Entry, &d.IsActive,
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
		&d.ID, &d.UserID, &d.LogDate, &d.Entry, &d.IsActive,
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

	return r.scanDailyLogs(rows, "getDailyLogsByDateRange")
}

func (r *sqlDailyLogRepository) getInactiveDailyLogs(ctx context.Context, userID string, limit, offset int) ([]DailyLog, error) {
	if len(userID) > repoMaxUserIDLength {
		return []DailyLog{}, nil
	}

	rows, err := r.stmtGetInactiveDailyLogs.QueryContext(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getInactiveDailyLogs: %w", ErrDatabase)
	}
	defer func() { _ = rows.Close() }()

	return r.scanDailyLogs(rows, "getInactiveDailyLogs")
}

func (r *sqlDailyLogRepository) scanDailyLogs(rows *sql.Rows, methodName string) ([]DailyLog, error) {
	var logs []DailyLog
	for rows.Next() {
		var d DailyLog
		if err := rows.Scan(
			&d.ID, &d.UserID, &d.LogDate, &d.Entry, &d.IsActive,
			&d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("%s scan: %w", methodName, ErrDatabase)
		}
		if err := d.Validate(); err != nil {
			return nil, fmt.Errorf("%s validate: %w", methodName, ErrDatabase)
		}
		logs = append(logs, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", methodName, ErrDatabase)
	}

	if len(logs) == 0 {
		return []DailyLog{}, nil
	}
	return logs, nil
}

func (r *sqlDailyLogRepository) createDailyLog(ctx context.Context, userID string, date time.Time, entry string) (DailyLog, error) {
	// Defence-in-depth: validate userID length.
	// Entry length validation is handled at the service layer.
	if len(userID) > repoMaxUserIDLength {
		return DailyLog{}, fmt.Errorf("createDailyLog: %w", ErrDatabase)
	}

	var d DailyLog
	err := r.stmtCreateDailyLog.QueryRowContext(ctx, userID, date, entry).Scan(
		&d.ID, &d.UserID, &d.LogDate, &d.Entry, &d.IsActive,
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
	// Defence-in-depth: validate userID length.
	// Entry length validation is handled at the service layer.
	if len(userID) > repoMaxUserIDLength {
		return DailyLog{}, ErrDailyLogNotFound
	}

	var d DailyLog
	err := r.stmtUpdateDailyLog.QueryRowContext(ctx, entry, id, userID).Scan(
		&d.ID, &d.UserID, &d.LogDate, &d.Entry, &d.IsActive,
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

func (r *sqlDailyLogRepository) deactivateDailyLog(ctx context.Context, id, userID string) error {
	if len(userID) > repoMaxUserIDLength {
		return ErrDailyLogNotFound
	}

	result, err := r.stmtDeactivateDailyLog.ExecContext(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("deactivateDailyLog %s: %w", id, ErrDatabase)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("deactivateDailyLog rowsAffected %s: %w", id, ErrDatabase)
	}

	if rowsAffected == 0 {
		return ErrDailyLogNotFound
	}
	return nil
}

func (r *sqlDailyLogRepository) hardDeleteDailyLog(ctx context.Context, id, userID string) error {
	if len(userID) > repoMaxUserIDLength {
		return ErrDailyLogNotFound
	}

	result, err := r.stmtHardDeleteDailyLog.ExecContext(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("hardDeleteDailyLog %s: %w", id, ErrDatabase)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("hardDeleteDailyLog rowsAffected %s: %w", id, ErrDatabase)
	}

	if rowsAffected == 0 {
		return ErrDailyLogNotFound
	}
	return nil
}

func (r *sqlDailyLogRepository) bulkDeactivateDailyLogs(ctx context.Context, userID string, ids []string) (int, error) {
	if len(userID) > repoMaxUserIDLength {
		return 0, nil
	}

	query := `UPDATE daily_logs SET is_active = false, updated_at = NOW() WHERE user_id = $1 AND id = ANY($2::uuid[]) AND is_active = true`
	result, err := r.db.ExecContext(ctx, query, userID, ids)
	if err != nil {
		return 0, fmt.Errorf("bulkDeactivateDailyLogs: %w", ErrDatabase)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("bulkDeactivateDailyLogs rowsAffected: %w", ErrDatabase)
	}

	return int(rowsAffected), nil
}

func (r *sqlDailyLogRepository) bulkHardDeleteDailyLogs(ctx context.Context, userID string, ids []string) (int, error) {
	if len(userID) > repoMaxUserIDLength {
		return 0, nil
	}

	query := `DELETE FROM daily_logs WHERE user_id = $1 AND id = ANY($2::uuid[]) AND is_active = false`
	result, err := r.db.ExecContext(ctx, query, userID, ids)
	if err != nil {
		return 0, fmt.Errorf("bulkHardDeleteDailyLogs: %w", ErrDatabase)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("bulkHardDeleteDailyLogs rowsAffected: %w", ErrDatabase)
	}

	return int(rowsAffected), nil
}

func (r *sqlDailyLogRepository) reactivateDailyLog(ctx context.Context, id, userID string) (DailyLog, error) {
	if len(userID) > repoMaxUserIDLength {
		return DailyLog{}, ErrDailyLogNotFound
	}

	var d DailyLog
	err := r.stmtReactivateDailyLog.QueryRowContext(ctx, id, userID).Scan(
		&d.ID, &d.UserID, &d.LogDate, &d.Entry, &d.IsActive,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DailyLog{}, ErrDailyLogNotFound
		}
		return DailyLog{}, fmt.Errorf("reactivateDailyLog %s: %w", id, ErrDatabase)
	}

	if err := d.Validate(); err != nil {
		return DailyLog{}, fmt.Errorf("reactivateDailyLog validate %s: %w", id, ErrDatabase)
	}
	return d, nil
}

// getDailyLogIsActive returns whether a daily log is active.
// Returns ErrDailyLogNotFound if the log doesn't exist or doesn't belong to the user.
func (r *sqlDailyLogRepository) getDailyLogIsActive(ctx context.Context, id, userID string) (bool, error) {
	if len(userID) > repoMaxUserIDLength {
		return false, ErrDailyLogNotFound
	}

	var isActive bool
	err := r.stmtGetDailyLogIsActive.QueryRowContext(ctx, id, userID).Scan(&isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrDailyLogNotFound
		}
		return false, fmt.Errorf("getDailyLogIsActive %s: %w", id, ErrDatabase)
	}

	return isActive, nil
}
