// Command go-tasks-database-migrator applies the schema migrations bundled
// into this binary against a Postgres database. The migrations are embedded
// at build time from the project's migrations/ directory, so this binary
// requires no external goose install or SQL file mount at deploy time.
//
// Database connection variables are the same DB_* set used by the API
// server — operators only configure one set of variables for both binaries.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"go-tasks-api/internal/config"
	"go-tasks-api/internal/version"
	"go-tasks-api/migrations"
)

const usage = `go-tasks-database-migrator — apply schema migrations bundled into this binary.

Usage:
    DB_HOST=db DB_PORT=5432 DB_USER=appuser DB_PASSWORD=... DB_NAME=appdb \
        DB_SSLMODE=require go-tasks-database-migrator <command> [args...]

Required environment variables (same as the API server):
    DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME

Optional:
    DB_SSLMODE   defaults to "require"

Commands (passed through to goose):
    up                    apply all pending migrations
    up-by-one             apply the next migration
    up-to <version>       apply migrations up to <version>
    down                  roll back the latest migration
    down-to <version>     roll back to <version>
    redo                  roll back the latest migration, then re-apply
    reset                 roll back all migrations
    status                print the migration status
    version               print this binary's version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cmd := os.Args[1]
	switch cmd {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(os.Stdout, usage)
		return
	case "-v", "--version", "version":
		_, _ = fmt.Fprint(os.Stdout, version.Current().Banner("go-tasks-database-migrator"))
		return
	}

	dbCfg, err := config.LoadDatabase()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	db, err := sql.Open("pgx", dbCfg.ConnectionString())
	if err != nil {
		fmt.Fprintf(os.Stderr, "open database: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		fmt.Fprintf(os.Stderr, "set dialect: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := goose.RunContext(ctx, cmd, db, ".", os.Args[2:]...); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintf(os.Stderr, "%s: %v\n", cmd, err)
		os.Exit(1)
	}
}
