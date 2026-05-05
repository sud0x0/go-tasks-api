# go-tasks-api

A task and habit management REST API built with Go. Users register, create recurring tasks across eight schedule types, log daily answers, and keep a personal journal. All data is scoped to the authenticated user.

## Contents

- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Getting Started](#getting-started)
- [Version](#version)
- [Configuration](#configuration)
- [API Overview](#api-overview)
- [Authentication](#authentication)
- [Recurrence Types](#recurrence-types)
- [Answer Types](#answer-types)
- [Database](#database)
- [Security](#security)
- [Code Quality](#code-quality)
- [Make Commands](#make-commands)
- [Releases](#releases)
- [TODO](#todo)

---

## Architecture

The project follows Domain-Driven Design with a layered structure. Each domain (auth, category, task, occurrence, dailylog) owns its handler, service, repository, model, and errors. No domain imports another domain's repository directly.

```
cmd/
  api/main.go                 — API server: wiring, middleware, graceful shutdown
  migrate/main.go             — database migrator binary (embeds migrations/*.sql)
internal/
  auth/                       — registration, login, JWT, refresh tokens, blocklist
  category/                   — task categories (CRUD with soft/hard delete)
  task/                       — tasks with schedules and select options (CRUD with soft/hard delete)
  occurrence/                 — on-demand occurrence generation and answers
  dailylog/                   — one journal entry per user per day (with soft/hard delete)
  config/                     — environment variable loading
  db/                         — database connection and health check
  metrics/                    — Prometheus instrumentation
  middleware/                 — CORS, security headers, request logger
  shared/                     — validation helpers, pagination, sanitisation, logger
  version/                    — version info for --version flag
migrations/                   — Goose SQL migrations (also embedded into cmd/migrate at build time)
```

**Request flow:** HTTP request → global middleware (request ID, logger, security headers, CORS, metrics, timeout, body limit) → auth middleware (JWT validation, blocklist check) → handler (sanitise → validate → service) → service (business logic) → repository (prepared statements) → PostgreSQL.

---

## Tech Stack

| Component | Choice |
|---|---|
| Language | Go 1.26 |
| Router | chi v5 |
| Database | PostgreSQL 16 |
| Cache / blocklist | Valkey 8 (Redis-compatible) |
| Auth | RS256 JWT (15 min access, 1 hr refresh with rotation) |
| Password hashing | Argon2id (64MB, 3 iterations, 2 threads) |
| Migrations | Goose |
| Metrics | Prometheus |
| Input sanitisation | bluemonday strict policy |
| Validation | go-playground/validator |
| Containers | Podman + podman-compose |
| Hot reload | Air |

---

## Getting Started

**Prerequisites:** Podman, podman-compose, Go 1.26, pre-commit, golangci-lint, govulncheck, semgrep, goose.

**First time:**

```bash
cp .env.example .env
# Edit .env and fill in your values
make setup
```

`make setup` installs pre-commit hooks, builds containers, starts PostgreSQL and Valkey, and runs all migrations. The API is available at `http://localhost:8080` when complete.

**Daily use:**

```bash
make run      # start containers and apply any pending migrations
make logs     # tail application logs
```

**Clean slate:**

```bash
make destroy  # remove all containers, volumes, and images
make build    # rebuild from scratch
```

---

## Version

Invoke `./go-tasks-api --version` (or `-v`) to print the binary's version information. The output includes the release version, git commit, build date, Go toolchain version, and target OS/architecture. The migrator binary supports the same flag.

```
go-tasks-api version 0.1.0
  Git commit: 49cddbf
  Build date: 2026-04-11T04:06:54Z
  Go version: go1.26.2
  OS/Arch:    darwin/arm64
```

See [Releases](#releases) for the release pipeline, artefact layout, and download verification.

---

## Configuration

All configuration is read from environment variables. Copy `.env.example` to `.env` and fill in values before running.

| Variable | Required | Default | Description |
|---|---|---|---|
| `PORT` | No | `8080` | HTTP server port |
| `DB_HOST` | Yes | — | PostgreSQL host |
| `DB_PORT` | Yes | — | PostgreSQL port |
| `DB_USER` | Yes | — | PostgreSQL user |
| `DB_PASSWORD` | Yes | — | PostgreSQL password |
| `DB_NAME` | Yes | — | PostgreSQL database name |
| `DB_SSLMODE` | No | `require` | SSL mode (`disable` for local dev) |
| `DB_MAX_OPEN_CONNS` | No | `100` | Connection pool max open |
| `DB_MAX_IDLE_CONNS` | No | `50` | Connection pool max idle |
| `DB_CONN_MAX_LIFETIME_MINS` | No | `5` | Connection max lifetime (minutes) |
| `DB_CONN_MAX_IDLE_TIME_MINS` | No | `10` | Connection max idle time (minutes) |
| `LOG_LEVEL` | No | `development` | `development`, `production`, `quiet`, `silent` |
| `CORS_ALLOWED_ORIGINS` | No | — | Comma-separated origins (no wildcards in production) |
| `VALKEY_URL` | No | `localhost:6379` | Valkey address |
| `VALKEY_PASSWORD` | No | — | Password for `requirepass`-protected Valkey (sent inline with HELLO) |
| `JWT_ISSUER` | No | `go-tasks-api` | JWT `iss` claim |
| `JWT_AUDIENCE` | No | `go-tasks-api` | JWT `aud` claim |
| `JWT_PRIVATE_KEY_PATH` | No | `./keys/private.pem` | RSA private key for signing |
| `JWT_PUBLIC_KEY_PATH` | No | `./keys/public.pem` | RSA public key for verification |

RSA keys are loaded from the configured paths on startup. The loader accepts both PKCS#1 (`-----BEGIN RSA PRIVATE KEY-----`) and PKCS#8 (`-----BEGIN PRIVATE KEY-----`) PEM formats — Vault and modern `openssl` emit the latter by default. If the key files are missing the binary generates a fresh PKCS#1 keypair on first run; a parse or permission error on an *existing* key file is a hard error rather than a silent regeneration. In production, provision keys out-of-band and mount the directory read-only.

---

## API Overview

Base URL: `http://localhost:8080`

All `/api/v1/*` endpoints (except auth) require a valid JWT access token in the `Authorization: Bearer <token>` header.

### Auth

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | No | Register a new user |
| POST | `/api/v1/auth/login` | No | Login (returns user info and tokens) |
| POST | `/api/v1/auth/refresh` | X-Refresh-Token header | Rotate tokens |
| POST | `/api/v1/auth/logout` | Bearer + X-Refresh-Token | Revoke tokens |

### Categories

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/categories` | List active categories (`limit`, `offset`) |
| POST | `/api/v1/categories` | Create category |
| GET | `/api/v1/categories/inactive` | List inactive (soft-deleted) categories (`limit`, `offset`) |
| POST | `/api/v1/categories/bulk-delete` | Bulk soft delete categories |
| POST | `/api/v1/categories/bulk-permanent-delete` | Bulk hard delete inactive categories |
| GET | `/api/v1/categories/{id}` | Get category |
| PUT | `/api/v1/categories/{id}` | Update category |
| DELETE | `/api/v1/categories/{id}` | Soft delete category (sets `is_active = false`) |
| DELETE | `/api/v1/categories/{id}/permanent` | Hard delete inactive category |
| POST | `/api/v1/categories/{id}/reactivate` | Reactivate a soft-deleted category |

**Category fields:**
- `name` (required): Max 100 characters. Unique per user among active categories (case-insensitive). Whitespace is trimmed.
- `description` (optional): Max 500 characters.
- `colour` (optional): Hex colour code in `#RRGGBB` format (e.g., `#ff0000`). Accepts upper/lower case; stored as lower-case. Defaults to `#808080` on create. If omitted on update, keeps existing value.

**Lifecycle (soft-then-hard delete):**
- `DELETE /categories/{id}`: sets `is_active = false` (soft delete). Returns 409 if already inactive or if the category has active tasks (deactivate tasks first).
- `DELETE /categories/{id}/permanent`: hard deletes the category and cascades to all associated tasks. Returns 409 if still active.
- Reactivation fails if another active category has the same name (case-insensitive).

**Bulk operations:**
- Accepts 1–100 IDs per request: `{"ids": ["uuid1", "uuid2", ...]}`
- `bulk-delete`: soft deletes active categories (inactive IDs are ignored)
- `bulk-permanent-delete`: hard deletes inactive categories (active IDs are ignored)
- Returns `{"requested": N, "soft_deleted": M}` or `{"requested": N, "permanently_deleted": M}`

**Error codes:**
- `400`: Invalid input (empty name, invalid colour format, name too long)
- `409`: Category with this name already exists, category is already inactive/active, category has active tasks, or cannot permanently delete an active category

### Tasks

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/tasks` | List tasks (`category_id`, `active`, `limit`, `offset`) |
| POST | `/api/v1/tasks` | Create task with schedule and optional select options |
| GET | `/api/v1/tasks/inactive` | List inactive (soft-deleted) tasks (`limit`, `offset`) |
| POST | `/api/v1/tasks/bulk-delete` | Bulk soft delete tasks |
| POST | `/api/v1/tasks/bulk-permanent-delete` | Bulk hard delete inactive tasks |
| GET | `/api/v1/tasks/{id}` | Get task with schedule and select options |
| PUT | `/api/v1/tasks/{id}` | Update task name and description |
| DELETE | `/api/v1/tasks/{id}` | Soft delete task (sets `is_active = false`) |
| DELETE | `/api/v1/tasks/{id}/permanent` | Hard delete inactive task |
| POST | `/api/v1/tasks/{id}/reactivate` | Reactivate a soft-deleted task |

**Lifecycle (soft-then-hard delete):**
- `DELETE /tasks/{id}`: sets `is_active = false` (soft delete). Returns 409 if already inactive.
- `DELETE /tasks/{id}/permanent`: hard deletes the task and cascades to all associated schedules, select options, occurrences, and answers. Returns 409 if still active.
- Reactivation fails if the task's category is inactive (must reactivate category first).

**Bulk operations:**
- Accepts 1–100 IDs per request: `{"ids": ["uuid1", "uuid2", ...]}`
- `bulk-delete`: soft deletes active tasks (inactive IDs are ignored)
- `bulk-permanent-delete`: hard deletes inactive tasks (active IDs are ignored)
- Returns `{"requested": N, "soft_deleted": M}` or `{"requested": N, "permanently_deleted": M}`

**Deep-hide filtering:**
- Tasks whose category is inactive are automatically hidden from all listing and get endpoints.
- This applies even if the task itself is active — a task is only visible when both the task and its category are active.

### Occurrences

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/occurrences` | Generate and list occurrences (`date` or `start_date` + `end_date` required) |
| POST | `/api/v1/occurrences/{id}/answer` | Submit or update an answer |
| POST | `/api/v1/occurrences/{id}/suppress` | Mark occurrence as skipped for this day |
| POST | `/api/v1/occurrences/{id}/unsuppress` | Remove the skipped/suppressed flag |
| POST | `/api/v1/occurrences/bulk-delete-answers` | Bulk delete answers by occurrence IDs (max 100) |

**Suppression behavior:**
- Suppressed occurrences reject answers with `409 Conflict` — unsuppress first to submit an answer.
- Use suppress to mark a task as deliberately skipped without recording a false/zero value.

**Deep-hide filtering:**
- Occurrences for inactive tasks or inactive categories are automatically hidden from all listing endpoints.
- This ensures archived data is not surfaced to users.

### Daily Logs

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/daily-logs` | List logs (`date` or `start_date` + `end_date`; defaults to today) |
| POST | `/api/v1/daily-logs` | Create journal entry (one per day) |
| GET | `/api/v1/daily-logs/inactive` | List inactive (soft-deleted) daily logs (`limit`, `offset`) |
| POST | `/api/v1/daily-logs/bulk-delete` | Bulk soft delete daily logs |
| POST | `/api/v1/daily-logs/bulk-permanent-delete` | Bulk hard delete inactive daily logs |
| PUT | `/api/v1/daily-logs/{id}` | Update journal entry |
| DELETE | `/api/v1/daily-logs/{id}` | Soft delete daily log (sets `is_active = false`) |
| DELETE | `/api/v1/daily-logs/{id}/permanent` | Hard delete inactive daily log |
| POST | `/api/v1/daily-logs/{id}/reactivate` | Reactivate a soft-deleted daily log |

**Lifecycle (soft-then-hard delete):**
- `DELETE /daily-logs/{id}`: sets `is_active = false` (soft delete). Returns 409 if already inactive.
- `DELETE /daily-logs/{id}/permanent`: hard deletes the daily log. Returns 409 if still active.

**Bulk operations:**
- Accepts 1–100 IDs per request: `{"ids": ["uuid1", "uuid2", ...]}`
- `bulk-delete`: soft deletes active daily logs (inactive IDs are ignored)
- `bulk-permanent-delete`: hard deletes inactive daily logs (active IDs are ignored)
- Duplicate IDs are deduplicated server-side
- Returns `{"requested": N, "soft_deleted": M}` or `{"requested": N, "permanently_deleted": M}`

### Health and Metrics

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/health` | No | Health check (includes database ping) |
| GET | `/metrics` | No | Prometheus metrics — restrict in production |

---

## Authentication

The API uses RS256-signed JWTs passed in HTTP headers. Access tokens are valid for 15 minutes and refresh tokens for 1 hour with rotation.

### Headers

| Header | Endpoint | Purpose |
|---|---|---|
| `Authorization: Bearer <token>` | All `/api/v1/*` | Access token for authentication |
| `X-Refresh-Token: <token>` | `/auth/refresh`, `/auth/logout` | Refresh token for rotation/revocation |

### Access Token

RS256-signed JWT with claims: `sub` (user ID), `iss`, `aud`, `exp`, `nbf`, `iat`, and `jti`. Validated on every authenticated request. Revoked JTIs are stored in Valkey blocklist.

### Refresh Token

32 random bytes, base64url-encoded. Stored as SHA-256 hash in PostgreSQL. On use, the old token is deleted and a new pair is issued (rotation). If the same refresh token is used twice, the second attempt is rejected.

### Token Lifecycle

```
POST /auth/register { username, password }
  → Returns: { id, username, created_at, updated_at }

POST /auth/login { username, password }
  → Returns: { user, access_token, refresh_token, expires_at, token_type }

POST /auth/refresh
  Headers: X-Refresh-Token: <refresh_token>
  → Returns: { access_token, refresh_token, expires_at, token_type }

POST /auth/logout
  Headers: Authorization: Bearer <access_token>
           X-Refresh-Token: <refresh_token>
  → Verifies refresh token ownership
  → Deletes refresh token from database
  → Adds access token JTI to blocklist
  → Returns: 204 No Content
```

### JavaScript Example

```javascript
// Store tokens after login
const response = await fetch('/api/v1/auth/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username, password })
});
const { access_token, refresh_token, expires_at } = await response.json();

// Make authenticated requests
fetch('/api/v1/categories', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${access_token}`
  },
  body: JSON.stringify({ name: 'New Category' })
});

// Refresh tokens before expiry
fetch('/api/v1/auth/refresh', {
  method: 'POST',
  headers: { 'X-Refresh-Token': refresh_token }
});
```

---

## Recurrence Types

Tasks have one of eight recurrence schedules, configured at creation. The schedule cannot be changed after creation — deactivate the task and create a new one if a schedule change is needed.

| Type | Required fields | Description |
|---|---|---|
| `once` | `start_date` | Appears once on `start_date` only |
| `daily` | `start_date` | Every day from `start_date` |
| `every_n_days` | `start_date`, `recurrence_interval` | Every N days |
| `weekly` | `start_date`, `days_of_week` | Specific days of the week (0=Sun, 6=Sat) |
| `every_n_weeks` | `start_date`, `recurrence_interval`, `days_of_week` | Specific days every N weeks |
| `monthly_date` | `start_date`, `month_day` | Same date each month (1–31) |
| `monthly_weekday` | `start_date`, `month_week`, `month_weekday` | Nth weekday of the month (e.g. 2nd Tuesday) |
| `yearly` | `start_date`, `month_day`, `month_of_year` | Same date each year |

All schedules support three end conditions via `end_type`: `never`, `on_date` (requires `end_date`), or `after_n` (requires `end_after_n`).

Scheduled times (`scheduled_times: ["09:00", "21:00"]`) are optional. If provided, one occurrence is generated per time slot. If omitted, one untimed occurrence is generated per matching day.

---

## Answer Types

Each task has a fixed `answer_type` set at creation. Submitting an answer of a different type returns 400.

| Type | Answer field | Constraints |
|---|---|---|
| `boolean` | `answer_boolean` | `true` or `false` |
| `integer` | `answer_integer` | Any integer including negative |
| `string` | `answer_string` | Max 500 characters |
| `select` | `answer_select` | UUID of a valid option for this task |

Select tasks require 2–10 options at creation. Options are fixed — to change them, deactivate the task and create a new one.

Answers are upserted — submitting a second answer for the same occurrence replaces the first. `created_at` is preserved, `updated_at` and `answered_at` are updated.

Suppressed occurrences reject answers with `409 Conflict`. Unsuppress the occurrence first if you need to submit an answer. This ensures suppression cleanly represents "intentionally skipped" without mixed state.

---

## Database

Migrations in `migrations/`:

- `00001_initial_schema.sql` — complete database schema including users, refresh tokens, categories, tasks, schedules, select options, occurrences, answers, and daily logs with all indexes and constraints

**Development:** run migrations with `make db-migrate`. Roll back with `make db-reset`. Check status with `make db-status`. These targets shell into the dev container and invoke the `goose` CLI against the mounted `migrations/` directory.

**Production / release:** each tagged release publishes a `go-tasks-database-migrator-<version>_<os>_<arch>` binary alongside the API binary. The migrator is a self-contained executable — the SQL migrations are embedded into the binary at build time, so it has no external dependency on `goose` or on the migrations directory. It reads the same `DB_*` environment variables as the API server so operators only configure one set of variables for both binaries. Run it before rolling out a new API binary:

```bash
chmod +x go-tasks-database-migrator-<version>_linux_amd64
DB_HOST=db DB_PORT=5432 DB_USER="$DB_USER" DB_PASSWORD="$DB_PASSWORD" \
DB_NAME="$DB_NAME" DB_SSLMODE=require \
    ./go-tasks-database-migrator-<version>_linux_amd64 up
```

Subcommands: `up`, `up-by-one`, `up-to <v>`, `down`, `down-to <v>`, `redo`, `reset`, `status`, `version`.

**Occurrence generation** follows the iCalendar materialised occurrence pattern — occurrences are generated on demand when `GET /occurrences` is called and upserted into `task_occurrences`. Calling the same date twice is idempotent. Occurrence uniqueness is enforced per-task, not per-schedule — this ensures a task generates exactly one occurrence per date/time slot regardless of how schedules are configured.

**Soft delete pattern** — Categories, tasks, and daily logs use soft delete via `is_active` column. First delete sets `is_active = false`, second delete (via `/permanent` endpoint) performs hard delete with cascade.

---

## Security

- **Passwords** — Argon2id with 64MB memory, 3 iterations, 2 threads, random 16-byte salt per password. Constant-time comparison on verification. Length: 8–128 code points (runes) after NFKC normalization. No composition rules (uppercase/lowercase/digit requirements) — passphrases with spaces are allowed. ASCII control characters (U+0000–U+0008, U+000B, U+000C, U+000E–U+001F, U+007F) are rejected; tab, LF, CR, and space are allowed. NFKC normalization ensures that visually similar inputs (e.g., full-width `ｐａｓｓｗｏｒｄ` vs ASCII `password`) hash identically.
- **JWT signing** — RS256 only. Algorithm is whitelisted in the verifier — the `alg` header from the token is never trusted.
- **Refresh tokens** — stored as SHA-256 hashes only. The plaintext token is never stored. Ownership verified before deletion (prevents cross-user logout attacks).
- **Blocklist** — revoked access token JTIs are stored in Valkey with TTL equal to the token's remaining lifetime. Checked on every authenticated request.
- **Input sanitisation** — all string inputs are processed through bluemonday strict policy, null byte stripping, and HTML unescape before validation. Passwords are NOT sanitised (bluemonday would mangle legitimate special characters); instead, control character validation is applied separately.
- **Unicode-aware length validation** — free-text fields (names, descriptions, entries) use rune count (`utf8.RuneCountInString`) instead of byte length for length limits. This matches PostgreSQL `VARCHAR(n)` character semantics and prevents multi-byte characters from failing validation or being truncated. Length validation is owned by the service layer; repositories only perform type and shape guards.
- **Request limits** — 1MB body limit, 60-second global timeout, read header timeout 5s, write timeout 30s.
- **Security headers** — `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`, `Referrer-Policy: no-referrer`.
- **CORS** — explicit origin allowlist from `CORS_ALLOWED_ORIGINS`. Wildcards are not supported.
- **Pagination** — limit capped at 100, offset capped at 10,000. Non-integer values return 400.

**Known limitations:**
- No application-level brute-force protection on login — implement this at the infrastructure layer (reverse proxy, WAF, or rate-limiting middleware).
- The `/metrics` endpoint has no authentication — restrict it via network policy or reverse proxy in production.

---

## Code Quality

Pre-commit hooks run automatically on every commit:

- `gofmt` — formatting
- `golangci-lint` — linting (govet, staticcheck, errcheck, gosec, and others — see `.golangci.yml`)
- `govulncheck` — known vulnerability scan on `go.mod` / `go.sum` changes
- `semgrep` — static security analysis
- `gitleaks` — secrets detection
- `conventional-pre-commit` — validates that commit subjects follow the [Conventional Commits](https://www.conventionalcommits.org/) spec (`feat:`, `fix:`, `chore:`, `feat(api)!:`, etc.). GoReleaser groups the release changelog by these prefixes.

`make setup` installs both the `pre-commit` and `commit-msg` stage hooks. If you bootstrap a clone manually, run `pre-commit install --hook-type commit-msg` in addition to `pre-commit install` so the conventional-commits check actually fires.

Run all hooks manually:

```bash
make pre-commit-run
```

Run individual tools:

```bash
make lint        # golangci-lint
make vet         # go vet
make vulncheck   # govulncheck
make semgrep     # semgrep
make socket      # Socket.dev supply chain scan (requires npm install -g socket)
```

---

## Make Commands

```
Development
  setup            First-time setup: copies .env, installs hooks, builds containers
  build            Build dev containers and run migrations
  run              Start containers and run migrations
  logs             View application logs
  destroy          Destroy all containers, volumes, and images
  clean            Delete all temp, build, test, and release artifacts

Database
  db-migrate       Run pending migrations
  db-reset         Rollback all migrations
  db-status        Check migration status
  db-wait          Wait for database to be ready

Release
  prod-build       Snapshot the production release locally (cross-compiled binaries in dist/)
  goreleaser-check Validate the release pipeline end-to-end (run before pushing a tag)

Code Quality
  test             Run all tests
  test-pretty      Run tests with formatted table output
  lint             Run golangci-lint
  fmt              Format all Go files
  vet              Run go vet
  pre-commit-run   Run all pre-commit hooks against all files
  vulncheck        Run govulncheck
  semgrep          Run semgrep security scan
  socket           Run Socket.dev supply chain scan

Typical workflow
  First time:  make setup
  Daily:       make run → make logs
  Fresh start: make destroy → make build
  Tidy up:     make clean
```

---

## Releases

Tagged releases are automated via GitHub Actions on every `v*` tag push. The workflow (`.github/workflows/release.yml`) performs these steps:

1. Cross-compiles the API server (`go-tasks-api`) and the database migrator (`go-tasks-database-migrator`) for Linux, macOS, and Windows on amd64 + arm64 (Windows arm64 excluded).
2. Injects the release version, git commit, and build date into the binaries via `-ldflags` so `--version` reports them.
3. Generates an SPDX-JSON SBOM per binary using [syft](https://github.com/anchore/syft).
4. Computes SHA-256 checksums for every binary and SBOM into `checksums.txt`.
5. Publishes a GitHub Release with all binaries, SBOMs, and `checksums.txt`, plus auto-generated changelog grouped by Conventional Commit type.
6. Generates SLSA Level 3 build provenance via [`slsa-github-generator`](https://github.com/slsa-framework/slsa-github-generator) and attaches `multiple.intoto.jsonl` to the release.

The release does not include a container image. Operators download the API and migrator binaries for their platform, run the migrator against the production database, then start the API.

### Binary Artefacts

Each release publishes a flat set of files (one binary per platform, plus shared metadata):

```
go-tasks-api-<version>_linux_amd64
go-tasks-api-<version>_linux_arm64
go-tasks-api-<version>_darwin_amd64
go-tasks-api-<version>_darwin_arm64
go-tasks-api-<version>_windows_amd64.exe
go-tasks-database-migrator-<version>_linux_amd64
go-tasks-database-migrator-<version>_linux_arm64
go-tasks-database-migrator-<version>_darwin_amd64
go-tasks-database-migrator-<version>_darwin_arm64
go-tasks-database-migrator-<version>_windows_amd64.exe
<binary>.sbom.json                 ← one SBOM per binary
checksums.txt
multiple.intoto.jsonl              ← SLSA provenance covering every binary
```

Both binaries are static and have no runtime dependencies. The SQL migrations are embedded into the migrator binary at build time — there is no external `goose` install or migrations directory to mount. The migrator reads the same `DB_*` environment variables as the API server (see [Configuration](#configuration)) so operators only configure one set of variables for both binaries.

Apply migrations, then run the API:

```bash
chmod +x go-tasks-api-<version>_linux_amd64 \
         go-tasks-database-migrator-<version>_linux_amd64

export DB_HOST=db DB_PORT=5432 DB_USER=appuser DB_PASSWORD=... \
       DB_NAME=appdb DB_SSLMODE=require

# 1. Apply migrations.
./go-tasks-database-migrator-<version>_linux_amd64 up

# 2. Run the API.
./go-tasks-api-<version>_linux_amd64
```

### Verifying Downloads

Validate SHA-256 and SLSA provenance before running:

```bash
# 1. Verify checksums for everything you downloaded.
sha256sum -c checksums.txt --ignore-missing

# 2. Verify the SLSA build provenance.
#    Install: go install github.com/slsa-framework/slsa-verifier/v2/cli/slsa-verifier@latest
slsa-verifier verify-artifact go-tasks-api-<version>_linux_amd64 \
    --provenance-path multiple.intoto.jsonl \
    --source-uri github.com/sud0x0/go-tasks-api \
    --source-tag v<version>
```

A successful provenance check confirms the binary was built from this repository at the specified tag by the release workflow — not tampered with after upload, not built from a fork, not produced by a different workflow. Run the same `slsa-verifier` command against the migrator binary too.

### Production Operator Notes

- **RSA keys.** Generate the JWT signing keypair out-of-band and mount it as a read-only volume / secret. The API auto-generates keys on first run if the files are missing, which invalidates issued tokens across replicas/restarts if the filesystem is not persistent.
- **Migrate first.** Run the database migrator against the production database before deploying a new API binary — the API refuses to start if required tables are missing.
- **Restrict `/metrics`.** The Prometheus endpoint has no authentication. Block it at the ingress / reverse proxy in production.

### Local Validation

Run `make goreleaser-check` to validate the release pipeline locally before pushing a tag. It runs `goreleaser check`, executes a full snapshot build, replays the workflow's jq filter against the resulting `dist/artifacts.json` to confirm SLSA subjects extract cleanly, and version-banner-tests the native-arch binaries. This catches issues pure config validation misses (e.g. a malformed jq filter that only manifests when applied to a real `artifacts.json`).

Requires [goreleaser](https://goreleaser.com/install/), `jq`, and (optionally) [syft](https://github.com/anchore/syft#installation) for the SBOM step.

For a quicker smoke test that just produces snapshot binaries under `dist/` without the SLSA / version-banner steps, run `make prod-build`.

---

## TODO

- Target based answer: for example 1/4.
- Password deny list usage.
- Refresh token receive only via header (to prevent JS access), and Bearer Token only 15 mins
- User account update (name, theme, accent colour)
- font size in profile
- theme colour in profile
- Add email based MFA
- Test script to work with MFA
- User delete
- Front end based e2e encryption toggle for user data
