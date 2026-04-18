# Changelog

All notable changes to this project will be documented in this file.

## [1.0.0] - 2026-04-18

### Added

#### Authentication
- User registration with username/password
- RS256-signed JWT access tokens (15 min TTL)
- Refresh tokens with rotation (1 hour TTL, SHA-256 hashed storage)
- Token blocklist via Valkey for immediate revocation
- Argon2id password hashing (64MB, 3 iterations, 2 threads)
- Logout endpoint with token revocation

#### Categories
- Full CRUD operations for task categories
- Soft delete and hard delete lifecycle
- Bulk soft delete (up to 100 IDs)
- Bulk permanent delete for inactive categories
- Reactivation of soft-deleted categories
- Unique name constraint per user (case-insensitive)
- Optional hex colour field with validation
- Category delete protection (prevents deletion with active tasks)

#### Tasks
- Full CRUD operations for tasks
- Eight recurrence types: once, daily, every_n_days, weekly, every_n_weeks, monthly_date, monthly_weekday, yearly
- Four answer types: boolean, integer, string, select
- Select options for select-type tasks
- Multiple scheduled times per day
- Three end conditions: never, on_date, after_n occurrences
- Soft delete and hard delete lifecycle
- Bulk soft delete (up to 100 IDs)
- Bulk permanent delete for inactive tasks
- Reactivation of soft-deleted tasks
- Deep-hide filtering (tasks hidden when category is inactive)
- Cascading delete of schedules, options, occurrences, and answers

#### Occurrences
- On-demand occurrence generation from schedules
- Single date and date range queries
- Answer submission with type validation
- Answer update (upsert behavior)
- Occurrence suppression (mark as skipped)
- Occurrence unsuppression
- Bulk delete answers (up to 100 occurrence IDs)
- Deep-hide filtering (occurrences hidden for inactive tasks/categories)
- Select options included in occurrence responses

#### Daily Logs
- One journal entry per user per day
- Date and date range queries
- Soft delete and hard delete lifecycle
- Bulk soft delete (up to 100 IDs)
- Bulk permanent delete for inactive logs
- Reactivation of soft-deleted logs

#### Infrastructure
- Chi v5 router with middleware stack
- PostgreSQL 16 with prepared statements
- Valkey 8 for token blocklist
- Goose database migrations
- Prometheus metrics endpoint
- Health check endpoint with database ping
- Graceful shutdown with cleanup
- Request ID propagation
- Structured logging with levels (development, production, quiet, silent)

#### Security
- Input sanitisation with bluemonday strict policy
- Request body size limit (1MB)
- Request timeout (30 seconds)
- Security headers (X-Content-Type-Options, X-Frame-Options, etc.)
- CORS configuration with origin allowlist
- SQL injection prevention via prepared statements
- XSS prevention via HTML sanitisation
- Null byte stripping from inputs
- UUID validation on all ID parameters
- User isolation (all queries scoped to authenticated user)

#### Developer Experience
- Makefile with common commands
- Podman and podman-compose support
- Air hot reload for development
- Pre-commit hooks (golangci-lint, govulncheck, semgrep)
- Comprehensive test suite (unit and integration)
- OpenAPI 3.0 specification
- Version command with build info (--version flag)
