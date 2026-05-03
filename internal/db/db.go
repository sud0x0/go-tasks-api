package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go-tasks-api/internal/config"
	"go-tasks-api/internal/shared/logger"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB wraps *sql.DB and provides application-level helpers.
// Obtain one via New() and pass it explicitly to repositories — no globals.
type DB struct {
	db *sql.DB
}

// New opens and validates a Postgres connection using the provided configuration.
// Returns an error for any misconfiguration rather than calling os.Exit —
// the caller (main) decides whether to exit and how to log the failure.
func New(cfg *config.DatabaseConfig, log logger.Logger) (*DB, error) {
	sqlDB, err := sql.Open("pgx", cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := verifySchema(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("schema verification failed — run make db-migrate: %w", err)
	}

	log.LogInfo("database initialised", "host", cfg.Host, "dbname", cfg.Name)
	return &DB{db: sqlDB}, nil
}

// SQL returns the underlying *sql.DB for use by repositories.
func (d *DB) SQL() *sql.DB {
	return d.db
}

// Close closes the database connection.
func (d *DB) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// HealthCheck performs a liveness check on the database connection.
func (d *DB) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := d.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}
	return nil
}

// WithTransaction executes fn inside a database transaction.
// Rolls back automatically if fn returns an error; commits otherwise.
func (d *DB) WithTransaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx error: %w, rollback error: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// TX is an interface satisfied by both *sql.DB and *sql.Tx.
// Use it in repositories to support both transactional and non-transactional calls.
type TX interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// requiredTables lists all tables that must exist for the application to run.
// Update this list whenever you add a migration.
var requiredTables = []string{
	"users",
	"refresh_tokens",
	"categories",
	"tasks",
	"task_select_options",
	"task_schedules",
	"task_occurrences",
	"task_answers",
	"daily_logs",
}

func verifySchema(ctx context.Context, sqlDB *sql.DB) error {
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = $1
	`
	var missing []string
	for _, table := range requiredTables {
		var tableName string
		err := sqlDB.QueryRowContext(ctx, query, table).Scan(&tableName)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			missing = append(missing, table)
		case err != nil:
			return fmt.Errorf("check table %s: %w", table, err)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required tables: %v", missing)
	}
	return nil
}
