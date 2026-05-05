// Package migrations exposes this directory's *.sql files as an embed.FS so
// the cmd/migrate binary can ship a self-contained migration runner. The
// goose CLI (used by `make db-migrate` for local dev) only reads the .sql
// files and ignores this Go file.
package migrations

import "embed"

// FS embeds every *.sql file in this directory and is consumed by cmd/migrate
// to run migrations without an external goose install.
//
//go:embed *.sql
var FS embed.FS
