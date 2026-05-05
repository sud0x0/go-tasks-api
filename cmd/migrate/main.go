// Command go-tasks-database-migrator applies the schema migrations bundled
// into this binary against a Postgres database. The migrations are embedded
// at build time from the project's migrations/ directory, so this binary
// requires no external goose install or SQL file mount at deploy time.
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

	"go-tasks-api/internal/version"
	"go-tasks-api/migrations"
)

const usage = `go-tasks-database-migrator — apply schema migrations bundled into this binary.

Usage:
    DATABASE_URL="host=db port=5432 user=... password=... dbname=... sslmode=require" \
        go-tasks-database-migrator <command> [args...]

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

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required (e.g. \"host=db port=5432 user=... password=... dbname=... sslmode=require\")")
		os.Exit(2)
	}

	db, err := sql.Open("pgx", dsn)
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
