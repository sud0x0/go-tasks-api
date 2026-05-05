package occurrence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go-tasks-api/internal/task"

	"github.com/lib/pq"
)

// Repository size limits (defence-in-depth validation).
// Answer string length validation is handled at the service layer using rune count
// to match PostgreSQL VARCHAR(n) character semantics.
const (
	repoMaxUserIDLength = 64
)

// parseInt64Array parses a PostgreSQL array literal string to []int64.
func parseInt64Array(s string) []int64 {
	s = strings.Trim(s, "{}")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err == nil {
			result = append(result, n)
		}
	}
	return result
}

// parseStringArray parses a PostgreSQL array literal string to []string.
func parseStringArray(s string) []string {
	s = strings.Trim(s, "{}")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// parseTimeStr parses a PostgreSQL TIME string and combines it with a date.
func parseTimeStr(timeStr string, baseDate time.Time) *time.Time {
	if timeStr == "" {
		return nil
	}
	t, err := time.Parse("15:04:05", timeStr)
	if err != nil {
		t, err = time.Parse("15:04", timeStr)
		if err != nil {
			return nil
		}
	}
	fullTime := time.Date(
		baseDate.Year(), baseDate.Month(), baseDate.Day(),
		t.Hour(), t.Minute(), t.Second(), 0, time.UTC,
	)
	return &fullTime
}

// Prepared statement queries.
const (
	queryGetOccurrence = `SELECT o.id, o.task_id, o.schedule_id, o.user_id, o.occurrence_date, o.scheduled_time, o.is_suppressed, o.created_at
		FROM task_occurrences o
		JOIN tasks t ON o.task_id = t.id
		JOIN categories c ON t.category_id = c.id
		WHERE o.id = $1 AND o.user_id = $2 AND t.is_active = true AND c.is_active = true`
	queryGetOccurrencesByDate = `SELECT o.id, o.task_id, o.schedule_id, o.user_id, o.occurrence_date, o.scheduled_time, o.is_suppressed, o.created_at
		FROM task_occurrences o
		JOIN tasks t ON o.task_id = t.id
		JOIN categories c ON t.category_id = c.id
		WHERE o.user_id = $1 AND o.occurrence_date = $2 AND t.is_active = true AND c.is_active = true ORDER BY o.scheduled_time NULLS LAST`
	queryGetOccurrencesByDateRange = `SELECT o.id, o.task_id, o.schedule_id, o.user_id, o.occurrence_date, o.scheduled_time, o.is_suppressed, o.created_at
		FROM task_occurrences o
		JOIN tasks t ON o.task_id = t.id
		JOIN categories c ON t.category_id = c.id
		WHERE o.user_id = $1 AND o.occurrence_date >= $2 AND o.occurrence_date <= $3 AND t.is_active = true AND c.is_active = true
		ORDER BY o.occurrence_date, o.scheduled_time NULLS LAST`
	queryUpsertTimedOccurrence = `INSERT INTO task_occurrences (task_id, schedule_id, user_id, occurrence_date, scheduled_time)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (task_id, occurrence_date, scheduled_time) WHERE scheduled_time IS NOT NULL
		DO UPDATE SET schedule_id = EXCLUDED.schedule_id
		RETURNING id, task_id, schedule_id, user_id, occurrence_date, scheduled_time, is_suppressed, created_at`
	queryUpsertUntimedOccurrence = `INSERT INTO task_occurrences (task_id, schedule_id, user_id, occurrence_date, scheduled_time)
		VALUES ($1, $2, $3, $4, NULL)
		ON CONFLICT (task_id, occurrence_date) WHERE scheduled_time IS NULL
		DO UPDATE SET schedule_id = EXCLUDED.schedule_id
		RETURNING id, task_id, schedule_id, user_id, occurrence_date, scheduled_time, is_suppressed, created_at`
	querySuppressOccurrence   = `UPDATE task_occurrences SET is_suppressed = true WHERE id = $1 AND user_id = $2`
	queryUnsuppressOccurrence = `UPDATE task_occurrences SET is_suppressed = false WHERE id = $1 AND user_id = $2`

	queryGetAnswer = `SELECT id, occurrence_id, user_id, answer_string, answer_integer, answer_boolean, answer_select, answered_at, created_at, updated_at
		FROM task_answers WHERE occurrence_id = $1 AND user_id = $2`
	queryUpsertAnswer = `INSERT INTO task_answers (occurrence_id, user_id, answer_string, answer_integer, answer_boolean, answer_select)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (occurrence_id) DO UPDATE SET
			answer_string = EXCLUDED.answer_string,
			answer_integer = EXCLUDED.answer_integer,
			answer_boolean = EXCLUDED.answer_boolean,
			answer_select = EXCLUDED.answer_select,
			answered_at = NOW(),
			updated_at = NOW()
		RETURNING id, occurrence_id, user_id, answer_string, answer_integer, answer_boolean, answer_select, answered_at, created_at, updated_at`

	queryGetTask = `SELECT t.id, t.user_id, t.category_id, t.name, t.description, t.answer_type, t.is_active, t.created_at, t.updated_at
		FROM tasks t
		JOIN categories c ON t.category_id = c.id
		WHERE t.id = $1 AND t.user_id = $2 AND t.is_active = true AND c.is_active = true`
	queryGetActiveSchedulesByDate = `SELECT ts.id, ts.task_id, ts.recurrence_type, ts.recurrence_interval, ts.days_of_week,
		ts.month_day, ts.month_week, ts.month_weekday, ts.month_of_year, ts.scheduled_times, ts.start_date,
		ts.end_type, ts.end_date, ts.end_after_n, ts.created_at
		FROM task_schedules ts
		JOIN tasks t ON ts.task_id = t.id
		JOIN categories c ON t.category_id = c.id
		WHERE t.user_id = $1 AND t.is_active = true AND c.is_active = true AND ts.start_date <= $2
		AND (ts.end_type = 'never' OR (ts.end_type = 'on_date' AND ts.end_date >= $2) OR ts.end_type = 'after_n')`
	queryGetSelectOptions        = `SELECT tso.id, tso.task_id, tso.value, tso.position, tso.created_at FROM task_select_options tso JOIN tasks t ON tso.task_id = t.id WHERE tso.task_id = $1 AND t.user_id = $2 ORDER BY tso.position`
	queryCheckSelectOptionExists = `SELECT EXISTS(SELECT 1 FROM task_select_options tso JOIN tasks t ON tso.task_id = t.id WHERE tso.id = $1 AND tso.task_id = $2 AND t.user_id = $3)`
	queryCountOccurrences        = `SELECT COUNT(*) FROM task_occurrences tocc JOIN task_schedules ts ON tocc.schedule_id = ts.id JOIN tasks t ON ts.task_id = t.id WHERE tocc.schedule_id = $1 AND t.user_id = $2 AND tocc.is_suppressed = false`
)

// occurrenceRepository defines the interface for occurrence data access.
type occurrenceRepository interface {
	getOccurrence(ctx context.Context, id, userID string) (TaskOccurrence, error)
	getOccurrencesByDate(ctx context.Context, userID string, date time.Time) ([]TaskOccurrence, error)
	getOccurrencesByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]TaskOccurrence, error)
	upsertOccurrence(ctx context.Context, taskID, scheduleID, userID string, date time.Time, scheduledTime *time.Time) (TaskOccurrence, error)
	suppressOccurrence(ctx context.Context, id, userID string) error
	unsuppressOccurrence(ctx context.Context, id, userID string) error
	countOccurrences(ctx context.Context, scheduleID, userID string) (int, error)

	getAnswer(ctx context.Context, occurrenceID, userID string) (*TaskAnswer, error)
	getAnswersByOccurrenceIDs(ctx context.Context, occurrenceIDs []string, userID string) (map[string]*TaskAnswer, error)
	upsertAnswer(ctx context.Context, occurrenceID, userID string, answer AnswerRequest) (TaskAnswer, error)
	bulkDeleteAnswers(ctx context.Context, userID string, occurrenceIDs []string) (int, error)

	getTask(ctx context.Context, taskID, userID string) (task.Task, error)
	getActiveSchedulesByDate(ctx context.Context, userID string, date time.Time) ([]task.Schedule, error)
	getActiveSchedulesForRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]task.Schedule, error)
	getSelectOptions(ctx context.Context, taskID, userID string) ([]task.SelectOption, error)
	selectOptionExists(ctx context.Context, optionID, taskID, userID string) (bool, error)

	Close() error
}

// sqlOccurrenceRepository implements occurrenceRepository using a SQL database.
type sqlOccurrenceRepository struct {
	db                          *sql.DB
	stmtGetOccurrence           *sql.Stmt
	stmtGetOccurrencesByDate    *sql.Stmt
	stmtGetOccurrencesByRange   *sql.Stmt
	stmtUpsertTimedOccurrence   *sql.Stmt
	stmtUpsertUntimedOccurrence *sql.Stmt
	stmtSuppressOccurrence      *sql.Stmt
	stmtUnsuppressOccurrence    *sql.Stmt
	stmtCountOccurrences        *sql.Stmt
	stmtGetAnswer               *sql.Stmt
	stmtUpsertAnswer            *sql.Stmt
	stmtGetTask                 *sql.Stmt
	stmtGetActiveSchedules      *sql.Stmt
	stmtGetSelectOptions        *sql.Stmt
	stmtCheckSelectOptionExists *sql.Stmt
}

// NewOccurrenceRepository creates a new occurrenceRepository with prepared statements.
func NewOccurrenceRepository(db *sql.DB, _ occurrenceLogger) occurrenceRepository {
	repo := &sqlOccurrenceRepository{db: db}

	var err error

	repo.stmtGetOccurrence, err = db.Prepare(queryGetOccurrence)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare getOccurrence: %v", err))
	}

	repo.stmtGetOccurrencesByDate, err = db.Prepare(queryGetOccurrencesByDate)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare getOccurrencesByDate: %v", err))
	}

	repo.stmtGetOccurrencesByRange, err = db.Prepare(queryGetOccurrencesByDateRange)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare getOccurrencesByRange: %v", err))
	}

	repo.stmtUpsertTimedOccurrence, err = db.Prepare(queryUpsertTimedOccurrence)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare upsertTimedOccurrence: %v", err))
	}

	repo.stmtUpsertUntimedOccurrence, err = db.Prepare(queryUpsertUntimedOccurrence)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare upsertUntimedOccurrence: %v", err))
	}

	repo.stmtSuppressOccurrence, err = db.Prepare(querySuppressOccurrence)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare suppressOccurrence: %v", err))
	}

	repo.stmtUnsuppressOccurrence, err = db.Prepare(queryUnsuppressOccurrence)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare unsuppressOccurrence: %v", err))
	}

	repo.stmtCountOccurrences, err = db.Prepare(queryCountOccurrences)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare countOccurrences: %v", err))
	}

	repo.stmtGetAnswer, err = db.Prepare(queryGetAnswer)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare getAnswer: %v", err))
	}

	repo.stmtUpsertAnswer, err = db.Prepare(queryUpsertAnswer)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare upsertAnswer: %v", err))
	}

	repo.stmtGetTask, err = db.Prepare(queryGetTask)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare getTask: %v", err))
	}

	repo.stmtGetActiveSchedules, err = db.Prepare(queryGetActiveSchedulesByDate)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare getActiveSchedules: %v", err))
	}

	repo.stmtGetSelectOptions, err = db.Prepare(queryGetSelectOptions)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare getSelectOptions: %v", err))
	}

	repo.stmtCheckSelectOptionExists, err = db.Prepare(queryCheckSelectOptionExists)
	if err != nil {
		panic(fmt.Sprintf("occurrence_repository: failed to prepare checkSelectOptionExists: %v", err))
	}

	return repo
}

// Close closes all prepared statements.
func (r *sqlOccurrenceRepository) Close() error {
	var errs []error
	for _, stmt := range []*sql.Stmt{
		r.stmtGetOccurrence,
		r.stmtGetOccurrencesByDate,
		r.stmtGetOccurrencesByRange,
		r.stmtUpsertTimedOccurrence,
		r.stmtUpsertUntimedOccurrence,
		r.stmtSuppressOccurrence,
		r.stmtUnsuppressOccurrence,
		r.stmtCountOccurrences,
		r.stmtGetAnswer,
		r.stmtUpsertAnswer,
		r.stmtGetTask,
		r.stmtGetActiveSchedules,
		r.stmtGetSelectOptions,
		r.stmtCheckSelectOptionExists,
	} {
		if err := stmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *sqlOccurrenceRepository) getOccurrence(ctx context.Context, id, userID string) (TaskOccurrence, error) {
	if len(userID) > repoMaxUserIDLength {
		return TaskOccurrence{}, ErrInvalidInput
	}

	var o TaskOccurrence
	var scheduledTimeStr sql.NullString

	err := r.stmtGetOccurrence.QueryRowContext(ctx, id, userID).Scan(
		&o.ID, &o.TaskID, &o.ScheduleID, &o.UserID, &o.OccurrenceDate,
		&scheduledTimeStr, &o.IsSuppressed, &o.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TaskOccurrence{}, ErrOccurrenceNotFound
		}
		return TaskOccurrence{}, fmt.Errorf("getOccurrence %s: %w: %w", id, ErrDatabase, err)
	}

	if scheduledTimeStr.Valid {
		o.ScheduledTime = parseTimeStr(scheduledTimeStr.String, o.OccurrenceDate)
	}

	if err := o.Validate(); err != nil {
		return TaskOccurrence{}, fmt.Errorf("getOccurrence validate %s: %w: %w", id, ErrDatabase, err)
	}
	return o, nil
}

func (r *sqlOccurrenceRepository) getOccurrencesByDate(ctx context.Context, userID string, date time.Time) ([]TaskOccurrence, error) {
	if len(userID) > repoMaxUserIDLength {
		return []TaskOccurrence{}, nil
	}

	rows, err := r.stmtGetOccurrencesByDate.QueryContext(ctx, userID, date)
	if err != nil {
		return nil, fmt.Errorf("getOccurrencesByDate: %w: %w", ErrDatabase, err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanOccurrences(rows)
}

func (r *sqlOccurrenceRepository) getOccurrencesByDateRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]TaskOccurrence, error) {
	if len(userID) > repoMaxUserIDLength {
		return []TaskOccurrence{}, nil
	}

	rows, err := r.stmtGetOccurrencesByRange.QueryContext(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("getOccurrencesByDateRange: %w: %w", ErrDatabase, err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanOccurrences(rows)
}

func (r *sqlOccurrenceRepository) scanOccurrences(rows *sql.Rows) ([]TaskOccurrence, error) {
	var occurrences []TaskOccurrence
	for rows.Next() {
		var o TaskOccurrence
		var scheduledTimeStr sql.NullString

		if err := rows.Scan(
			&o.ID, &o.TaskID, &o.ScheduleID, &o.UserID, &o.OccurrenceDate,
			&scheduledTimeStr, &o.IsSuppressed, &o.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanOccurrences: %w: %w", ErrDatabase, err)
		}

		if scheduledTimeStr.Valid {
			o.ScheduledTime = parseTimeStr(scheduledTimeStr.String, o.OccurrenceDate)
		}

		if err := o.Validate(); err != nil {
			return nil, fmt.Errorf("scanOccurrences validate: %w: %w", ErrDatabase, err)
		}
		occurrences = append(occurrences, o)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanOccurrences rows: %w: %w", ErrDatabase, err)
	}

	if len(occurrences) == 0 {
		return []TaskOccurrence{}, nil
	}
	return occurrences, nil
}

func (r *sqlOccurrenceRepository) upsertOccurrence(ctx context.Context, taskID, scheduleID, userID string, date time.Time, scheduledTime *time.Time) (TaskOccurrence, error) {
	var o TaskOccurrence
	var scheduledTimeStr sql.NullString
	var err error

	if scheduledTime != nil {
		// Timed occurrence - use the timed upsert query
		err = r.stmtUpsertTimedOccurrence.QueryRowContext(ctx, taskID, scheduleID, userID, date, scheduledTime).Scan(
			&o.ID, &o.TaskID, &o.ScheduleID, &o.UserID, &o.OccurrenceDate,
			&scheduledTimeStr, &o.IsSuppressed, &o.CreatedAt,
		)
	} else {
		// Untimed occurrence - use the untimed upsert query (only 4 params, NULL is hardcoded)
		err = r.stmtUpsertUntimedOccurrence.QueryRowContext(ctx, taskID, scheduleID, userID, date).Scan(
			&o.ID, &o.TaskID, &o.ScheduleID, &o.UserID, &o.OccurrenceDate,
			&scheduledTimeStr, &o.IsSuppressed, &o.CreatedAt,
		)
	}

	if err != nil {
		return TaskOccurrence{}, fmt.Errorf("upsertOccurrence: %w: %w", ErrDatabase, err)
	}

	if scheduledTimeStr.Valid {
		o.ScheduledTime = parseTimeStr(scheduledTimeStr.String, o.OccurrenceDate)
	}

	if err := o.Validate(); err != nil {
		return TaskOccurrence{}, fmt.Errorf("upsertOccurrence validate: %w: %w", ErrDatabase, err)
	}
	return o, nil
}

func (r *sqlOccurrenceRepository) suppressOccurrence(ctx context.Context, id, userID string) error {
	result, err := r.stmtSuppressOccurrence.ExecContext(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("suppressOccurrence %s: %w: %w", id, ErrDatabase, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("suppressOccurrence rowsAffected %s: %w: %w", id, ErrDatabase, err)
	}

	if rowsAffected == 0 {
		return ErrOccurrenceNotFound
	}
	return nil
}

func (r *sqlOccurrenceRepository) unsuppressOccurrence(ctx context.Context, id, userID string) error {
	result, err := r.stmtUnsuppressOccurrence.ExecContext(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("unsuppressOccurrence %s: %w: %w", id, ErrDatabase, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("unsuppressOccurrence rowsAffected %s: %w: %w", id, ErrDatabase, err)
	}

	if rowsAffected == 0 {
		return ErrOccurrenceNotFound
	}
	return nil
}

func (r *sqlOccurrenceRepository) countOccurrences(ctx context.Context, scheduleID, userID string) (int, error) {
	var count int
	err := r.stmtCountOccurrences.QueryRowContext(ctx, scheduleID, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("countOccurrences %s: %w: %w", scheduleID, ErrDatabase, err)
	}
	return count, nil
}

func (r *sqlOccurrenceRepository) getAnswer(ctx context.Context, occurrenceID, userID string) (*TaskAnswer, error) {
	var a TaskAnswer
	err := r.stmtGetAnswer.QueryRowContext(ctx, occurrenceID, userID).Scan(
		&a.ID, &a.OccurrenceID, &a.UserID, &a.AnswerString, &a.AnswerInteger,
		&a.AnswerBoolean, &a.AnswerSelect, &a.AnsweredAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getAnswer %s: %w: %w", occurrenceID, ErrDatabase, err)
	}

	if err := a.Validate(); err != nil {
		return nil, fmt.Errorf("getAnswer validate %s: %w: %w", occurrenceID, ErrDatabase, err)
	}
	return &a, nil
}

func (r *sqlOccurrenceRepository) upsertAnswer(ctx context.Context, occurrenceID, userID string, answer AnswerRequest) (TaskAnswer, error) {
	// Answer string length validation is handled at the service layer.
	var a TaskAnswer
	err := r.stmtUpsertAnswer.QueryRowContext(ctx,
		occurrenceID, userID,
		answer.AnswerString, answer.AnswerInteger, answer.AnswerBoolean, answer.AnswerSelect,
	).Scan(
		&a.ID, &a.OccurrenceID, &a.UserID, &a.AnswerString, &a.AnswerInteger,
		&a.AnswerBoolean, &a.AnswerSelect, &a.AnsweredAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return TaskAnswer{}, fmt.Errorf("upsertAnswer: %w: %w", ErrDatabase, err)
	}

	if err := a.Validate(); err != nil {
		return TaskAnswer{}, fmt.Errorf("upsertAnswer validate: %w: %w", ErrDatabase, err)
	}
	return a, nil
}

func (r *sqlOccurrenceRepository) getTask(ctx context.Context, taskID, userID string) (task.Task, error) {
	var t task.Task
	err := r.stmtGetTask.QueryRowContext(ctx, taskID, userID).Scan(
		&t.ID, &t.UserID, &t.CategoryID, &t.Name, &t.Description,
		&t.AnswerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return task.Task{}, ErrOccurrenceNotFound
		}
		return task.Task{}, fmt.Errorf("getTask %s: %w: %w", taskID, ErrDatabase, err)
	}

	if err := t.Validate(); err != nil {
		return task.Task{}, fmt.Errorf("getTask validate %s: %w: %w", taskID, ErrDatabase, err)
	}
	return t, nil
}

func (r *sqlOccurrenceRepository) getActiveSchedulesByDate(ctx context.Context, userID string, date time.Time) ([]task.Schedule, error) {
	rows, err := r.stmtGetActiveSchedules.QueryContext(ctx, userID, date)
	if err != nil {
		return nil, fmt.Errorf("getActiveSchedulesByDate: %w: %w", ErrDatabase, err)
	}
	defer func() { _ = rows.Close() }()

	var schedules []task.Schedule
	for rows.Next() {
		var s task.Schedule
		var daysOfWeekStr, scheduledTimesStr sql.NullString

		if err := rows.Scan(
			&s.ID, &s.TaskID, &s.RecurrenceType, &s.RecurrenceInterval,
			&daysOfWeekStr, &s.MonthDay, &s.MonthWeek, &s.MonthWeekday,
			&s.MonthOfYear, &scheduledTimesStr, &s.StartDate, &s.EndType,
			&s.EndDate, &s.EndAfterN, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("getActiveSchedulesByDate scan: %w: %w", ErrDatabase, err)
		}

		// Parse array strings back to slices
		if daysOfWeekStr.Valid {
			s.DaysOfWeek = parseInt64Array(daysOfWeekStr.String)
		}
		if scheduledTimesStr.Valid {
			s.ScheduledTimes = parseStringArray(scheduledTimesStr.String)
		}

		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("getActiveSchedulesByDate validate: %w: %w", ErrDatabase, err)
		}
		schedules = append(schedules, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getActiveSchedulesByDate rows: %w: %w", ErrDatabase, err)
	}

	if len(schedules) == 0 {
		return []task.Schedule{}, nil
	}
	return schedules, nil
}

func (r *sqlOccurrenceRepository) getSelectOptions(ctx context.Context, taskID, userID string) ([]task.SelectOption, error) {
	rows, err := r.stmtGetSelectOptions.QueryContext(ctx, taskID, userID)
	if err != nil {
		return nil, fmt.Errorf("getSelectOptions: %w: %w", ErrDatabase, err)
	}
	defer func() { _ = rows.Close() }()

	var options []task.SelectOption
	for rows.Next() {
		var opt task.SelectOption
		if err := rows.Scan(&opt.ID, &opt.TaskID, &opt.Value, &opt.Position, &opt.CreatedAt); err != nil {
			return nil, fmt.Errorf("getSelectOptions scan: %w: %w", ErrDatabase, err)
		}
		if err := opt.Validate(); err != nil {
			return nil, fmt.Errorf("getSelectOptions validate: %w: %w", ErrDatabase, err)
		}
		options = append(options, opt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getSelectOptions rows: %w: %w", ErrDatabase, err)
	}

	if len(options) == 0 {
		return []task.SelectOption{}, nil
	}
	return options, nil
}

func (r *sqlOccurrenceRepository) selectOptionExists(ctx context.Context, optionID, taskID, userID string) (bool, error) {
	var exists bool
	err := r.stmtCheckSelectOptionExists.QueryRowContext(ctx, optionID, taskID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("selectOptionExists: %w: %w", ErrDatabase, err)
	}
	return exists, nil
}

func (r *sqlOccurrenceRepository) getAnswersByOccurrenceIDs(ctx context.Context, occurrenceIDs []string, userID string) (map[string]*TaskAnswer, error) {
	if len(occurrenceIDs) == 0 {
		return make(map[string]*TaskAnswer), nil
	}

	// Build query with placeholders for the array
	query := `SELECT id, occurrence_id, user_id, answer_string, answer_integer, answer_boolean, answer_select, answered_at, created_at, updated_at
		FROM task_answers WHERE occurrence_id = ANY($1) AND user_id = $2`

	rows, err := r.db.QueryContext(ctx, query, occurrenceIDs, userID)
	if err != nil {
		return nil, fmt.Errorf("getAnswersByOccurrenceIDs: %w: %w", ErrDatabase, err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]*TaskAnswer)
	for rows.Next() {
		var a TaskAnswer
		if err := rows.Scan(
			&a.ID, &a.OccurrenceID, &a.UserID, &a.AnswerString, &a.AnswerInteger,
			&a.AnswerBoolean, &a.AnswerSelect, &a.AnsweredAt, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("getAnswersByOccurrenceIDs scan: %w: %w", ErrDatabase, err)
		}
		if err := a.Validate(); err != nil {
			return nil, fmt.Errorf("getAnswersByOccurrenceIDs validate: %w: %w", ErrDatabase, err)
		}
		result[a.OccurrenceID] = &a
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getAnswersByOccurrenceIDs rows: %w: %w", ErrDatabase, err)
	}

	return result, nil
}

func (r *sqlOccurrenceRepository) getActiveSchedulesForRange(ctx context.Context, userID string, startDate, endDate time.Time) ([]task.Schedule, error) {
	query := `SELECT ts.id, ts.task_id, ts.recurrence_type, ts.recurrence_interval, ts.days_of_week,
		ts.month_day, ts.month_week, ts.month_weekday, ts.month_of_year, ts.scheduled_times, ts.start_date,
		ts.end_type, ts.end_date, ts.end_after_n, ts.created_at
		FROM task_schedules ts
		JOIN tasks t ON ts.task_id = t.id
		JOIN categories c ON t.category_id = c.id
		WHERE t.user_id = $1 AND t.is_active = true AND c.is_active = true AND ts.start_date <= $3
		AND (ts.end_type = 'never' OR (ts.end_type = 'on_date' AND ts.end_date >= $2) OR ts.end_type = 'after_n')`

	rows, err := r.db.QueryContext(ctx, query, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("getActiveSchedulesForRange: %w: %w", ErrDatabase, err)
	}
	defer func() { _ = rows.Close() }()

	var schedules []task.Schedule
	for rows.Next() {
		var s task.Schedule
		var daysOfWeekStr, scheduledTimesStr sql.NullString

		if err := rows.Scan(
			&s.ID, &s.TaskID, &s.RecurrenceType, &s.RecurrenceInterval,
			&daysOfWeekStr, &s.MonthDay, &s.MonthWeek, &s.MonthWeekday,
			&s.MonthOfYear, &scheduledTimesStr, &s.StartDate, &s.EndType,
			&s.EndDate, &s.EndAfterN, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("getActiveSchedulesForRange scan: %w: %w", ErrDatabase, err)
		}

		// Parse array strings back to slices
		if daysOfWeekStr.Valid {
			s.DaysOfWeek = parseInt64Array(daysOfWeekStr.String)
		}
		if scheduledTimesStr.Valid {
			s.ScheduledTimes = parseStringArray(scheduledTimesStr.String)
		}

		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("getActiveSchedulesForRange validate: %w: %w", ErrDatabase, err)
		}
		schedules = append(schedules, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getActiveSchedulesForRange rows: %w: %w", ErrDatabase, err)
	}

	if len(schedules) == 0 {
		return []task.Schedule{}, nil
	}
	return schedules, nil
}

func (r *sqlOccurrenceRepository) bulkDeleteAnswers(ctx context.Context, userID string, occurrenceIDs []string) (int, error) {
	if len(userID) > repoMaxUserIDLength {
		return 0, nil
	}

	query := `DELETE FROM task_answers WHERE user_id = $1 AND occurrence_id = ANY($2::uuid[])`
	result, err := r.db.ExecContext(ctx, query, userID, pq.StringArray(occurrenceIDs))
	if err != nil {
		return 0, fmt.Errorf("bulkDeleteAnswers: %w: %w", ErrDatabase, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("bulkDeleteAnswers rowsAffected: %w: %w", ErrDatabase, err)
	}

	return int(rowsAffected), nil
}
