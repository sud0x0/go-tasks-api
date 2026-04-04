# go-tasks-api

A task and habit management REST API built with Go. Users register, create recurring tasks across eight schedule types, log daily answers, and keep a personal journal. All data is scoped to the authenticated user.

## Contents

- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Getting Started](#getting-started)
- [Configuration](#configuration)
- [API Overview](#api-overview)
- [Authentication](#authentication)
- [Recurrence Types](#recurrence-types)
- [Answer Types](#answer-types)
- [Database](#database)
- [Security](#security)
- [Code Quality](#code-quality)
- [Make Commands](#make-commands)

---

## Architecture

The project follows Domain-Driven Design with a layered structure. Each domain (auth, category, task, occurrence, dailylog, log) owns its handler, service, repository, model, and errors. No domain imports another domain's repository directly.

```
cmd/api/main.go               — wiring, middleware, graceful shutdown
internal/
  auth/                       — registration, login, JWT, refresh tokens, blocklist
  category/                   — task categories (CRUD)
  task/                       — tasks with schedules and select options (CRUD)
  occurrence/                 — on-demand occurrence generation and answers
  dailylog/                   — one journal entry per user per day
  config/                     — environment variable loading
  db/                         — database connection and health check
  metrics/                    — Prometheus instrumentation
  middleware/                 — CORS, security headers, request logger
  shared/                     — validation helpers, pagination, sanitisation
migrations/                   — Goose SQL migrations
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
| `JWT_ISSUER` | No | `go-tasks-api` | JWT `iss` claim |
| `JWT_AUDIENCE` | No | `go-tasks-api` | JWT `aud` claim |
| `JWT_PRIVATE_KEY_PATH` | No | `./keys/private.pem` | RSA private key for signing |
| `JWT_PUBLIC_KEY_PATH` | No | `./keys/public.pem` | RSA public key for verification |

RSA keys are generated automatically on first startup if not present. In production, generate and manage keys separately and never include them in version control or build artefacts.

---

## API Overview

Base URL: `http://localhost:8080`

All `/api/v1/*` endpoints require a valid `Authorization: Bearer <token>` header except the auth endpoints listed below.

### Auth

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/auth/register` | No | Register a new user |
| POST | `/api/v1/auth/login` | No | Login and receive token pair |
| POST | `/api/v1/auth/refresh` | No | Rotate refresh token |
| POST | `/api/v1/auth/logout` | No | Revoke tokens |

### Categories

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/categories` | List categories (`limit`, `offset`) |
| POST | `/api/v1/categories` | Create category |
| GET | `/api/v1/categories/{id}` | Get category |
| PUT | `/api/v1/categories/{id}` | Update category |
| DELETE | `/api/v1/categories/{id}` | Delete category (blocked if active tasks exist) |

### Tasks

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/tasks` | List tasks (`category_id`, `active`, `limit`, `offset`) |
| POST | `/api/v1/tasks` | Create task with schedule and optional select options |
| GET | `/api/v1/tasks/{id}` | Get task with schedule and select options |
| PUT | `/api/v1/tasks/{id}` | Update task name and description |
| DELETE | `/api/v1/tasks/{id}` | Soft delete (sets `is_active = false`) |

### Occurrences

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/occurrences` | Generate and list occurrences (`date` or `start_date` + `end_date` required) |
| POST | `/api/v1/occurrences/{id}/answer` | Submit or update an answer |
| POST | `/api/v1/occurrences/{id}/suppress` | Mark occurrence as skipped for this day |

### Daily Logs

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/daily-logs` | List logs (`date` or `start_date` + `end_date`; defaults to today) |
| POST | `/api/v1/daily-logs` | Create journal entry (one per day) |
| PUT | `/api/v1/daily-logs/{id}` | Update journal entry |

### Health and Metrics

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/health` | No | Health check (includes database ping) |
| GET | `/metrics` | No | Prometheus metrics — restrict in production |

---

## Authentication

The API uses RS256-signed JWTs. On login, two tokens are issued: a short-lived access token (15 minutes) and a longer-lived refresh token (1 hour).

**Access token** — send in the `Authorization: Bearer <token>` header on every protected request. Claims include `sub` (user ID), `iss`, `aud`, `exp`, `nbf`, `iat`, and `jti`.

**Refresh token** — opaque 32-byte random value stored as a SHA-256 hash in PostgreSQL. On use, the old token is deleted and a new pair is issued (rotation). If the same refresh token is used twice, the second attempt is rejected.

**Logout** — deletes the refresh token from the database and adds the access token's `jti` to a Valkey blocklist with a TTL matching the token's remaining lifetime. The blocklist is checked on every authenticated request.

**Token lifecycle:**

```
POST /auth/login
  → access_token (15 min) + refresh_token (1 hr)

POST /auth/refresh  { refresh_token }
  → new access_token + new refresh_token
  → old refresh_token is deleted

POST /auth/logout  { refresh_token }  + Authorization: Bearer <access_token>
  → refresh_token deleted from database
  → access_token jti added to Valkey blocklist
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

Suppressed occurrences still accept answers. Suppression marks a task as deliberately skipped but does not prevent recording data.

---

## Database

Three migrations in `migrations/`:

- `00001_initial_schema.sql` — legacy logs table
- `00002_task_manager.sql` — users, refresh tokens, categories, tasks, schedules, select options, occurrences, answers, daily logs
- `00003_fix_occurrence_unique.sql` — partial unique indexes for NULL-safe occurrence deduplication

Run migrations with `make db-migrate`. Roll back with `make db-reset`. Check status with `make db-status`.

All migrations are managed by Goose and run inside the app container.

**Occurrence generation** follows the iCalendar materialised occurrence pattern — occurrences are generated on demand when `GET /occurrences` is called and upserted into `task_occurrences`. Calling the same date twice is idempotent.

---

## Security

- **Passwords** — Argon2id with 64MB memory, 3 iterations, 2 threads, random 16-byte salt per password. Constant-time comparison on verification.
- **JWT signing** — RS256 only. Algorithm is whitelisted in the verifier — the `alg` header from the token is never trusted.
- **Refresh tokens** — stored as SHA-256 hashes only. The plaintext token is never stored.
- **Blocklist** — revoked access token JTIs are stored in Valkey with TTL equal to the token's remaining lifetime. Checked on every authenticated request.
- **Input sanitisation** — all string inputs are processed through bluemonday strict policy, null byte stripping, and HTML unescape before validation. Passwords are not sanitised.
- **Request limits** — 1MB body limit, 60-second global timeout, read header timeout 5s, write timeout 30s.
- **Security headers** — `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`, `Referrer-Policy: no-referrer`.
- **CORS** — explicit origin allowlist from `CORS_ALLOWED_ORIGINS`. Wildcards are not supported.
- **Pagination** — limit capped at 100, offset capped at 10,000. Non-integer values return 400.

**Known limitations:** No application-level brute-force protection on login — implement this at the infrastructure layer (reverse proxy, WAF, or rate-limiting middleware). The `/metrics` endpoint has no authentication — restrict it via network policy or reverse proxy in production.

---

## Code Quality

Pre-commit hooks run automatically on every commit:

- `gofmt` — formatting
- `golangci-lint` — linting (govet, staticcheck, errcheck, gosec, and others — see `.golangci.yml`)
- `govulncheck` — known vulnerability scan on `go.mod` / `go.sum` changes
- `semgrep` — static security analysis
- `gitleaks` — secrets detection

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
  build            Build containers and run migrations
  run              Start containers and run migrations
  logs             View application logs
  destroy          Destroy all containers, volumes, and images

Database
  db-migrate       Run pending migrations
  db-reset         Rollback all migrations
  db-status        Check migration status
  db-wait          Wait for database to be ready

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
```

---

## TODO
- Add email based MFA
- User account update (name, theme, accent colour)
- User delete
- Test script to work with MFA
