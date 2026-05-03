package task

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// PostgreSQL SQLSTATE codes (https://www.postgresql.org/docs/current/errcodes-appendix.html).
const (
	pgErrCodeUniqueViolation     = "23505"
	pgErrCodeForeignKeyViolation = "23503"
)

// Repository size limits (defence-in-depth validation).
// Length validation for name, description, and option values is handled at the service layer
// using rune count to match PostgreSQL VARCHAR(n) character semantics.
const (
	repoMaxUserIDLength = 64
)

// parseIntArrayLiteral parses a PostgreSQL array literal string to []int64.
// Example: "{0,1,5}" -> []int64{0, 1, 5}.
func parseIntArrayLiteral(s string) ([]int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" || s == "NULL" {
		return nil, nil
	}
	// Remove braces
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	result := make([]int64, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil {
			return nil, err
		}
		result[i] = v
	}
	return result, nil
}

// parseStringArrayLiteral parses a PostgreSQL array literal string to []string.
// Example: "{\"09:00\",\"14:30\"}" -> []string{"09:00", "14:30"}.
func parseStringArrayLiteral(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" || s == "NULL" {
		return nil
	}
	// Remove braces
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	if s == "" {
		return nil
	}
	// Parse quoted strings
	var result []string
	var current strings.Builder
	inQuotes := false
	escaped := false
	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				result = append(result, current.String())
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 || len(result) > 0 {
		result = append(result, current.String())
	}
	return result
}

// Prepared statement queries.
const (
	queryGetTask = `SELECT t.id, t.user_id, t.category_id, t.name, t.description, t.answer_type, t.is_active, t.created_at, t.updated_at
		FROM tasks t
		JOIN categories c ON t.category_id = c.id
		WHERE t.id = $1 AND t.user_id = $2 AND c.is_active = true`
	queryGetTasks = `SELECT t.id, t.user_id, t.category_id, t.name, t.description, t.answer_type, t.is_active, t.created_at, t.updated_at
		FROM tasks t
		JOIN categories c ON t.category_id = c.id
		WHERE t.user_id = $1 AND t.is_active = $2 AND c.is_active = true ORDER BY t.name ASC LIMIT $3 OFFSET $4`
	queryGetTasksByCategoryID = `SELECT t.id, t.user_id, t.category_id, t.name, t.description, t.answer_type, t.is_active, t.created_at, t.updated_at
		FROM tasks t
		JOIN categories c ON t.category_id = c.id
		WHERE t.user_id = $1 AND t.category_id = $2 AND t.is_active = $3 AND c.is_active = true ORDER BY t.name ASC LIMIT $4 OFFSET $5`
	queryCreateTask = `INSERT INTO tasks (user_id, category_id, name, description, answer_type)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, user_id, category_id, name, description, answer_type, is_active, created_at, updated_at`
	queryUpdateTask = `UPDATE tasks SET name = $1, description = $2, updated_at = NOW()
		WHERE id = $3 AND user_id = $4 RETURNING id, user_id, category_id, name, description, answer_type, is_active, created_at, updated_at`
	queryDeactivateTask      = `UPDATE tasks SET is_active = false, updated_at = NOW() WHERE id = $1 AND user_id = $2 AND is_active = true`
	queryHardDeleteTask      = `DELETE FROM tasks WHERE id = $1 AND user_id = $2 AND is_active = false`
	queryReactivateTask      = `UPDATE tasks SET is_active = true, updated_at = NOW() WHERE id = $1 AND user_id = $2 AND is_active = false RETURNING id, user_id, category_id, name, description, answer_type, is_active, created_at, updated_at`
	queryGetTaskIsActive     = `SELECT is_active FROM tasks WHERE id = $1 AND user_id = $2`
	queryCheckCategoryExists = `SELECT EXISTS(SELECT 1 FROM categories WHERE id = $1 AND user_id = $2)`
	queryCheckCategoryActive = `SELECT is_active FROM categories WHERE id = $1 AND user_id = $2`
	queryGetTaskCategoryID   = `SELECT category_id FROM tasks WHERE id = $1 AND user_id = $2`
	queryGetInactiveTasks    = `SELECT id, user_id, category_id, name, description, answer_type, is_active, created_at, updated_at FROM tasks WHERE user_id = $1 AND is_active = false ORDER BY name ASC LIMIT $2 OFFSET $3`

	queryCreateSchedule = `INSERT INTO task_schedules (task_id, recurrence_type, recurrence_interval, days_of_week,
		month_day, month_week, month_weekday, month_of_year, scheduled_times, start_date, end_type, end_date, end_after_n)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, task_id, recurrence_type, recurrence_interval, days_of_week, month_day, month_week, month_weekday,
		month_of_year, scheduled_times, start_date, end_type, end_date, end_after_n, created_at`
	queryGetScheduleByTaskID = `SELECT ts.id, ts.task_id, ts.recurrence_type, ts.recurrence_interval, ts.days_of_week, ts.month_day,
		ts.month_week, ts.month_weekday, ts.month_of_year, ts.scheduled_times, ts.start_date, ts.end_type, ts.end_date, ts.end_after_n, ts.created_at
		FROM task_schedules ts JOIN tasks t ON ts.task_id = t.id WHERE ts.task_id = $1 AND t.user_id = $2`

	queryCreateSelectOption = `INSERT INTO task_select_options (task_id, value, position) VALUES ($1, $2, $3)
		RETURNING id, task_id, value, position, created_at`
	queryGetSelectOptionsByTaskID = `SELECT tso.id, tso.task_id, tso.value, tso.position, tso.created_at
		FROM task_select_options tso JOIN tasks t ON tso.task_id = t.id WHERE tso.task_id = $1 AND t.user_id = $2 ORDER BY tso.position ASC`
)

// taskRepository defines the interface for task data access.
type taskRepository interface {
	getTask(ctx context.Context, id, userID string) (Task, error)
	getTasks(ctx context.Context, userID string, isActive bool, limit, offset int) ([]Task, error)
	getInactiveTasks(ctx context.Context, userID string, limit, offset int) ([]Task, error)
	getTasksByCategoryID(ctx context.Context, userID, categoryID string, isActive bool, limit, offset int) ([]Task, error)
	createTask(ctx context.Context, userID, categoryID, name string, description *string, answerType string) (Task, error)
	createTaskWithScheduleAndOptions(ctx context.Context, userID, categoryID, name string, description *string, answerType string, schedule *Schedule, selectOptions []SelectOptionRequest) (WithDetails, error)
	updateTask(ctx context.Context, id, userID, name string, description *string) (Task, error)
	deactivateTask(ctx context.Context, id, userID string) error
	hardDeleteTask(ctx context.Context, id, userID string) error
	bulkDeactivateTasks(ctx context.Context, userID string, ids []string) (int, error)
	bulkHardDeleteTasks(ctx context.Context, userID string, ids []string) (int, error)
	reactivateTask(ctx context.Context, id, userID string) (Task, error)
	getTaskIsActive(ctx context.Context, id, userID string) (bool, error)
	getTaskCategoryID(ctx context.Context, id, userID string) (string, error)
	categoryExists(ctx context.Context, categoryID, userID string) (bool, error)
	categoryIsActive(ctx context.Context, categoryID, userID string) (bool, error)

	createSchedule(ctx context.Context, schedule *Schedule) (*Schedule, error)
	getScheduleByTaskID(ctx context.Context, taskID, userID string) (*Schedule, error)

	createSelectOption(ctx context.Context, taskID, value string, position int) (SelectOption, error)
	getSelectOptionsByTaskID(ctx context.Context, taskID, userID string) ([]SelectOption, error)

	Close() error
}

// sqlTaskRepository implements taskRepository using a SQL database.
type sqlTaskRepository struct {
	db                           *sql.DB
	stmtGetTask                  *sql.Stmt
	stmtGetTasks                 *sql.Stmt
	stmtGetInactiveTasks         *sql.Stmt
	stmtGetTasksByCategoryID     *sql.Stmt
	stmtCreateTask               *sql.Stmt
	stmtUpdateTask               *sql.Stmt
	stmtDeactivateTask           *sql.Stmt
	stmtHardDeleteTask           *sql.Stmt
	stmtReactivateTask           *sql.Stmt
	stmtGetTaskIsActive          *sql.Stmt
	stmtGetTaskCategoryID        *sql.Stmt
	stmtCheckCategoryExists      *sql.Stmt
	stmtCheckCategoryActive      *sql.Stmt
	stmtCreateSchedule           *sql.Stmt
	stmtGetScheduleByTaskID      *sql.Stmt
	stmtCreateSelectOption       *sql.Stmt
	stmtGetSelectOptionsByTaskID *sql.Stmt
}

// NewTaskRepository creates a new taskRepository with prepared statements.
// Panics if any statement cannot be prepared — this is a fatal startup error.
func NewTaskRepository(db *sql.DB, _ taskLogger) taskRepository {
	repo := &sqlTaskRepository{db: db}

	var err error

	repo.stmtGetTask, err = db.Prepare(queryGetTask)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare getTask: %v", err))
	}

	repo.stmtGetTasks, err = db.Prepare(queryGetTasks)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare getTasks: %v", err))
	}

	repo.stmtGetInactiveTasks, err = db.Prepare(queryGetInactiveTasks)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare getInactiveTasks: %v", err))
	}

	repo.stmtGetTasksByCategoryID, err = db.Prepare(queryGetTasksByCategoryID)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare getTasksByCategoryID: %v", err))
	}

	repo.stmtCreateTask, err = db.Prepare(queryCreateTask)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare createTask: %v", err))
	}

	repo.stmtUpdateTask, err = db.Prepare(queryUpdateTask)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare updateTask: %v", err))
	}

	repo.stmtDeactivateTask, err = db.Prepare(queryDeactivateTask)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare deactivateTask: %v", err))
	}

	repo.stmtHardDeleteTask, err = db.Prepare(queryHardDeleteTask)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare hardDeleteTask: %v", err))
	}

	repo.stmtReactivateTask, err = db.Prepare(queryReactivateTask)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare reactivateTask: %v", err))
	}

	repo.stmtGetTaskIsActive, err = db.Prepare(queryGetTaskIsActive)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare getTaskIsActive: %v", err))
	}

	repo.stmtGetTaskCategoryID, err = db.Prepare(queryGetTaskCategoryID)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare getTaskCategoryID: %v", err))
	}

	repo.stmtCheckCategoryExists, err = db.Prepare(queryCheckCategoryExists)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare checkCategoryExists: %v", err))
	}

	repo.stmtCheckCategoryActive, err = db.Prepare(queryCheckCategoryActive)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare checkCategoryActive: %v", err))
	}

	repo.stmtCreateSchedule, err = db.Prepare(queryCreateSchedule)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare createSchedule: %v", err))
	}

	repo.stmtGetScheduleByTaskID, err = db.Prepare(queryGetScheduleByTaskID)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare getScheduleByTaskID: %v", err))
	}

	repo.stmtCreateSelectOption, err = db.Prepare(queryCreateSelectOption)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare createSelectOption: %v", err))
	}

	repo.stmtGetSelectOptionsByTaskID, err = db.Prepare(queryGetSelectOptionsByTaskID)
	if err != nil {
		panic(fmt.Sprintf("task_repository: failed to prepare getSelectOptionsByTaskID: %v", err))
	}

	return repo
}

// Close closes all prepared statements.
func (r *sqlTaskRepository) Close() error {
	var errs []error
	for _, stmt := range []*sql.Stmt{
		r.stmtGetTask,
		r.stmtGetTasks,
		r.stmtGetInactiveTasks,
		r.stmtGetTasksByCategoryID,
		r.stmtCreateTask,
		r.stmtUpdateTask,
		r.stmtDeactivateTask,
		r.stmtHardDeleteTask,
		r.stmtReactivateTask,
		r.stmtGetTaskIsActive,
		r.stmtGetTaskCategoryID,
		r.stmtCheckCategoryExists,
		r.stmtCheckCategoryActive,
		r.stmtCreateSchedule,
		r.stmtGetScheduleByTaskID,
		r.stmtCreateSelectOption,
		r.stmtGetSelectOptionsByTaskID,
	} {
		if err := stmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *sqlTaskRepository) getTask(ctx context.Context, id, userID string) (Task, error) {
	if len(userID) > repoMaxUserIDLength {
		return Task{}, ErrInvalidInput
	}

	var t Task
	err := r.stmtGetTask.QueryRowContext(ctx, id, userID).Scan(
		&t.ID, &t.UserID, &t.CategoryID, &t.Name, &t.Description,
		&t.AnswerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, fmt.Errorf("getTask %s: %w", id, ErrDatabase)
	}

	if err := t.Validate(); err != nil {
		return Task{}, fmt.Errorf("getTask validate %s: %w", id, ErrDatabase)
	}
	return t, nil
}

func (r *sqlTaskRepository) getTasks(ctx context.Context, userID string, isActive bool, limit, offset int) ([]Task, error) {
	if len(userID) > repoMaxUserIDLength {
		return []Task{}, nil
	}

	rows, err := r.stmtGetTasks.QueryContext(ctx, userID, isActive, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getTasks: %w", ErrDatabase)
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

func (r *sqlTaskRepository) getInactiveTasks(ctx context.Context, userID string, limit, offset int) ([]Task, error) {
	if len(userID) > repoMaxUserIDLength {
		return []Task{}, nil
	}

	rows, err := r.stmtGetInactiveTasks.QueryContext(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getInactiveTasks: %w", ErrDatabase)
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

func (r *sqlTaskRepository) getTasksByCategoryID(ctx context.Context, userID, categoryID string, isActive bool, limit, offset int) ([]Task, error) {
	if len(userID) > repoMaxUserIDLength {
		return []Task{}, nil
	}

	rows, err := r.stmtGetTasksByCategoryID.QueryContext(ctx, userID, categoryID, isActive, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("getTasksByCategoryID: %w", ErrDatabase)
	}
	defer func() { _ = rows.Close() }()

	return r.scanTasks(rows)
}

func (r *sqlTaskRepository) scanTasks(rows *sql.Rows) ([]Task, error) {
	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.CategoryID, &t.Name, &t.Description,
			&t.AnswerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanTasks: %w", ErrDatabase)
		}
		if err := t.Validate(); err != nil {
			return nil, fmt.Errorf("scanTasks validate: %w", ErrDatabase)
		}
		tasks = append(tasks, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanTasks rows: %w", ErrDatabase)
	}

	if len(tasks) == 0 {
		return []Task{}, nil
	}
	return tasks, nil
}

func (r *sqlTaskRepository) createTask(ctx context.Context, userID, categoryID, name string, description *string, answerType string) (Task, error) {
	// Defence-in-depth: validate userID length.
	// Name/description length validation is handled at the service layer.
	if len(userID) > repoMaxUserIDLength {
		return Task{}, fmt.Errorf("createTask: %w", ErrDatabase)
	}

	var t Task
	err := r.stmtCreateTask.QueryRowContext(ctx, userID, categoryID, name, description, answerType).Scan(
		&t.ID, &t.UserID, &t.CategoryID, &t.Name, &t.Description,
		&t.AnswerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgErrCodeForeignKeyViolation {
			return Task{}, ErrCategoryNotFound
		}
		return Task{}, fmt.Errorf("createTask: %w", ErrDatabase)
	}

	if err := t.Validate(); err != nil {
		return Task{}, fmt.Errorf("createTask validate: %w", ErrDatabase)
	}
	return t, nil
}

func (r *sqlTaskRepository) updateTask(ctx context.Context, id, userID, name string, description *string) (Task, error) {
	// Defence-in-depth: validate userID length.
	// Name/description length validation is handled at the service layer.
	if len(userID) > repoMaxUserIDLength {
		return Task{}, ErrTaskNotFound
	}

	var t Task
	err := r.stmtUpdateTask.QueryRowContext(ctx, name, description, id, userID).Scan(
		&t.ID, &t.UserID, &t.CategoryID, &t.Name, &t.Description,
		&t.AnswerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, fmt.Errorf("updateTask %s: %w", id, ErrDatabase)
	}

	if err := t.Validate(); err != nil {
		return Task{}, fmt.Errorf("updateTask validate %s: %w", id, ErrDatabase)
	}
	return t, nil
}

func (r *sqlTaskRepository) deactivateTask(ctx context.Context, id, userID string) error {
	if len(userID) > repoMaxUserIDLength {
		return ErrTaskNotFound
	}

	result, err := r.stmtDeactivateTask.ExecContext(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("deactivateTask %s: %w", id, ErrDatabase)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("deactivateTask rowsAffected %s: %w", id, ErrDatabase)
	}

	if rowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

func (r *sqlTaskRepository) categoryExists(ctx context.Context, categoryID, userID string) (bool, error) {
	var exists bool
	err := r.stmtCheckCategoryExists.QueryRowContext(ctx, categoryID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("categoryExists %s: %w", categoryID, ErrDatabase)
	}
	return exists, nil
}

func (r *sqlTaskRepository) categoryIsActive(ctx context.Context, categoryID, userID string) (bool, error) {
	var isActive bool
	err := r.stmtCheckCategoryActive.QueryRowContext(ctx, categoryID, userID).Scan(&isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrCategoryNotFound
		}
		return false, fmt.Errorf("categoryIsActive %s: %w", categoryID, ErrDatabase)
	}
	return isActive, nil
}

func (r *sqlTaskRepository) hardDeleteTask(ctx context.Context, id, userID string) error {
	if len(userID) > repoMaxUserIDLength {
		return ErrTaskNotFound
	}

	result, err := r.stmtHardDeleteTask.ExecContext(ctx, id, userID)
	if err != nil {
		return fmt.Errorf("hardDeleteTask %s: %w", id, ErrDatabase)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("hardDeleteTask rowsAffected %s: %w", id, ErrDatabase)
	}

	if rowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

func (r *sqlTaskRepository) bulkDeactivateTasks(ctx context.Context, userID string, ids []string) (int, error) {
	if len(userID) > repoMaxUserIDLength {
		return 0, nil
	}

	query := `UPDATE tasks SET is_active = false, updated_at = NOW() WHERE user_id = $1 AND id = ANY($2::uuid[]) AND is_active = true`
	result, err := r.db.ExecContext(ctx, query, userID, ids)
	if err != nil {
		return 0, fmt.Errorf("bulkDeactivateTasks: %w", ErrDatabase)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("bulkDeactivateTasks rowsAffected: %w", ErrDatabase)
	}

	return int(rowsAffected), nil
}

func (r *sqlTaskRepository) bulkHardDeleteTasks(ctx context.Context, userID string, ids []string) (int, error) {
	if len(userID) > repoMaxUserIDLength {
		return 0, nil
	}

	query := `DELETE FROM tasks WHERE user_id = $1 AND id = ANY($2::uuid[]) AND is_active = false`
	result, err := r.db.ExecContext(ctx, query, userID, ids)
	if err != nil {
		return 0, fmt.Errorf("bulkHardDeleteTasks: %w", ErrDatabase)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("bulkHardDeleteTasks rowsAffected: %w", ErrDatabase)
	}

	return int(rowsAffected), nil
}

func (r *sqlTaskRepository) reactivateTask(ctx context.Context, id, userID string) (Task, error) {
	if len(userID) > repoMaxUserIDLength {
		return Task{}, ErrTaskNotFound
	}

	var t Task
	err := r.stmtReactivateTask.QueryRowContext(ctx, id, userID).Scan(
		&t.ID, &t.UserID, &t.CategoryID, &t.Name, &t.Description,
		&t.AnswerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, fmt.Errorf("reactivateTask %s: %w", id, ErrDatabase)
	}

	if err := t.Validate(); err != nil {
		return Task{}, fmt.Errorf("reactivateTask validate %s: %w", id, ErrDatabase)
	}
	return t, nil
}

func (r *sqlTaskRepository) getTaskIsActive(ctx context.Context, id, userID string) (bool, error) {
	if len(userID) > repoMaxUserIDLength {
		return false, ErrTaskNotFound
	}

	var isActive bool
	err := r.stmtGetTaskIsActive.QueryRowContext(ctx, id, userID).Scan(&isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrTaskNotFound
		}
		return false, fmt.Errorf("getTaskIsActive %s: %w", id, ErrDatabase)
	}
	return isActive, nil
}

func (r *sqlTaskRepository) getTaskCategoryID(ctx context.Context, id, userID string) (string, error) {
	if len(userID) > repoMaxUserIDLength {
		return "", ErrTaskNotFound
	}

	var categoryID string
	err := r.stmtGetTaskCategoryID.QueryRowContext(ctx, id, userID).Scan(&categoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrTaskNotFound
		}
		return "", fmt.Errorf("getTaskCategoryID %s: %w", id, ErrDatabase)
	}
	return categoryID, nil
}

func (r *sqlTaskRepository) createSchedule(ctx context.Context, schedule *Schedule) (*Schedule, error) {
	var s Schedule

	// Scan into intermediate string variables for arrays
	var daysOfWeekStr, scheduledTimesStr sql.NullString

	err := r.stmtCreateSchedule.QueryRowContext(ctx,
		schedule.TaskID,
		schedule.RecurrenceType,
		schedule.RecurrenceInterval,
		pq.Int64Array(schedule.DaysOfWeek),
		schedule.MonthDay,
		schedule.MonthWeek,
		schedule.MonthWeekday,
		schedule.MonthOfYear,
		pq.StringArray(schedule.ScheduledTimes),
		schedule.StartDate,
		schedule.EndType,
		schedule.EndDate,
		schedule.EndAfterN,
	).Scan(
		&s.ID, &s.TaskID, &s.RecurrenceType, &s.RecurrenceInterval,
		&daysOfWeekStr, &s.MonthDay, &s.MonthWeek, &s.MonthWeekday,
		&s.MonthOfYear, &scheduledTimesStr, &s.StartDate, &s.EndType,
		&s.EndDate, &s.EndAfterN, &s.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("createSchedule: %w", ErrDatabase)
	}

	// Parse array strings back to slices
	if daysOfWeekStr.Valid {
		s.DaysOfWeek, _ = parseIntArrayLiteral(daysOfWeekStr.String)
	}
	if scheduledTimesStr.Valid {
		s.ScheduledTimes = parseStringArrayLiteral(scheduledTimesStr.String)
	}

	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("createSchedule validate: %w", ErrDatabase)
	}
	return &s, nil
}

func (r *sqlTaskRepository) getScheduleByTaskID(ctx context.Context, taskID, userID string) (*Schedule, error) {
	var s Schedule
	var daysOfWeekStr, scheduledTimesStr sql.NullString

	err := r.stmtGetScheduleByTaskID.QueryRowContext(ctx, taskID, userID).Scan(
		&s.ID, &s.TaskID, &s.RecurrenceType, &s.RecurrenceInterval,
		&daysOfWeekStr, &s.MonthDay, &s.MonthWeek, &s.MonthWeekday,
		&s.MonthOfYear, &scheduledTimesStr, &s.StartDate, &s.EndType,
		&s.EndDate, &s.EndAfterN, &s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("getScheduleByTaskID %s: %w", taskID, ErrDatabase)
	}

	// Parse array strings back to slices
	if daysOfWeekStr.Valid {
		s.DaysOfWeek, _ = parseIntArrayLiteral(daysOfWeekStr.String)
	}
	if scheduledTimesStr.Valid {
		s.ScheduledTimes = parseStringArrayLiteral(scheduledTimesStr.String)
	}

	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("getScheduleByTaskID validate %s: %w", taskID, ErrDatabase)
	}
	return &s, nil
}

func (r *sqlTaskRepository) createSelectOption(ctx context.Context, taskID, value string, position int) (SelectOption, error) {
	// Option value length validation is handled at the service layer.
	var opt SelectOption
	err := r.stmtCreateSelectOption.QueryRowContext(ctx, taskID, value, position).Scan(
		&opt.ID, &opt.TaskID, &opt.Value, &opt.Position, &opt.CreatedAt,
	)
	if err != nil {
		return SelectOption{}, fmt.Errorf("createSelectOption: %w", ErrDatabase)
	}

	if err := opt.Validate(); err != nil {
		return SelectOption{}, fmt.Errorf("createSelectOption validate: %w", ErrDatabase)
	}
	return opt, nil
}

func (r *sqlTaskRepository) getSelectOptionsByTaskID(ctx context.Context, taskID, userID string) ([]SelectOption, error) {
	rows, err := r.stmtGetSelectOptionsByTaskID.QueryContext(ctx, taskID, userID)
	if err != nil {
		return nil, fmt.Errorf("getSelectOptionsByTaskID: %w", ErrDatabase)
	}
	defer func() { _ = rows.Close() }()

	var options []SelectOption
	for rows.Next() {
		var opt SelectOption
		if err := rows.Scan(&opt.ID, &opt.TaskID, &opt.Value, &opt.Position, &opt.CreatedAt); err != nil {
			return nil, fmt.Errorf("getSelectOptionsByTaskID scan: %w", ErrDatabase)
		}
		if err := opt.Validate(); err != nil {
			return nil, fmt.Errorf("getSelectOptionsByTaskID validate: %w", ErrDatabase)
		}
		options = append(options, opt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("getSelectOptionsByTaskID rows: %w", ErrDatabase)
	}

	if len(options) == 0 {
		return []SelectOption{}, nil
	}
	return options, nil
}

func (r *sqlTaskRepository) createTaskWithScheduleAndOptions(
	ctx context.Context,
	userID, categoryID, name string,
	description *string,
	answerType string,
	schedule *Schedule,
	selectOptions []SelectOptionRequest,
) (WithDetails, error) {
	// Defence-in-depth: validate userID length.
	// Name/description length validation is handled at the service layer.
	if len(userID) > repoMaxUserIDLength {
		return WithDetails{}, fmt.Errorf("createTaskWithScheduleAndOptions: %w", ErrDatabase)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return WithDetails{}, fmt.Errorf("createTaskWithScheduleAndOptions begin: %w", ErrDatabase)
	}
	defer func() { _ = tx.Rollback() }()

	// Insert task
	var t Task
	err = tx.QueryRowContext(ctx, queryCreateTask, userID, categoryID, name, description, answerType).Scan(
		&t.ID, &t.UserID, &t.CategoryID, &t.Name, &t.Description,
		&t.AnswerType, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgErrCodeForeignKeyViolation {
			return WithDetails{}, ErrCategoryNotFound
		}
		return WithDetails{}, fmt.Errorf("createTaskWithScheduleAndOptions task: %w", ErrDatabase)
	}
	if err := t.Validate(); err != nil {
		return WithDetails{}, fmt.Errorf("createTaskWithScheduleAndOptions task validate: %w", ErrDatabase)
	}

	result := WithDetails{Task: t}

	// Insert schedule
	if schedule != nil {
		schedule.TaskID = t.ID

		var s Schedule
		var daysOfWeekStr, scheduledTimesStr sql.NullString

		err = tx.QueryRowContext(ctx, queryCreateSchedule,
			schedule.TaskID,
			schedule.RecurrenceType,
			schedule.RecurrenceInterval,
			pq.Int64Array(schedule.DaysOfWeek),
			schedule.MonthDay,
			schedule.MonthWeek,
			schedule.MonthWeekday,
			schedule.MonthOfYear,
			pq.StringArray(schedule.ScheduledTimes),
			schedule.StartDate,
			schedule.EndType,
			schedule.EndDate,
			schedule.EndAfterN,
		).Scan(
			&s.ID, &s.TaskID, &s.RecurrenceType, &s.RecurrenceInterval,
			&daysOfWeekStr, &s.MonthDay, &s.MonthWeek, &s.MonthWeekday,
			&s.MonthOfYear, &scheduledTimesStr, &s.StartDate, &s.EndType,
			&s.EndDate, &s.EndAfterN, &s.CreatedAt,
		)
		if err != nil {
			return WithDetails{}, fmt.Errorf("createTaskWithScheduleAndOptions schedule: %w", ErrDatabase)
		}

		if daysOfWeekStr.Valid {
			s.DaysOfWeek, _ = parseIntArrayLiteral(daysOfWeekStr.String)
		}
		if scheduledTimesStr.Valid {
			s.ScheduledTimes = parseStringArrayLiteral(scheduledTimesStr.String)
		}

		if err := s.Validate(); err != nil {
			return WithDetails{}, fmt.Errorf("createTaskWithScheduleAndOptions schedule validate: %w", ErrDatabase)
		}
		result.Schedule = &s
	}

	// Insert select options
	// Option value length validation is handled at the service layer.
	if len(selectOptions) > 0 {
		options := make([]SelectOption, 0, len(selectOptions))
		for i, opt := range selectOptions {
			var o SelectOption
			err = tx.QueryRowContext(ctx, queryCreateSelectOption, t.ID, opt.Value, i).Scan(
				&o.ID, &o.TaskID, &o.Value, &o.Position, &o.CreatedAt,
			)
			if err != nil {
				return WithDetails{}, fmt.Errorf("createTaskWithScheduleAndOptions option: %w", ErrDatabase)
			}
			if err := o.Validate(); err != nil {
				return WithDetails{}, fmt.Errorf("createTaskWithScheduleAndOptions option validate: %w", ErrDatabase)
			}
			options = append(options, o)
		}
		result.SelectOptions = options
	}

	if err := tx.Commit(); err != nil {
		return WithDetails{}, fmt.Errorf("createTaskWithScheduleAndOptions commit: %w", ErrDatabase)
	}

	return result, nil
}
