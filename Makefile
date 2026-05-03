# ============================================================================
# THIS IS FOR LOCAL DEVELOPMENT ONLY
# ============================================================================
# Requires .env file - podman-compose will automatically load it

COMPOSE_FILE=compose.dev.yaml
DB_CONTAINER=app_db
APP_CONTAINER=app_api

# --- Version metadata ------------------------------------------------
#
# Version is bumped manually. Commit SHA and build date are captured at
# build time. All three are injected into internal/version via -ldflags.
# The -s -w flags strip debug info and symbol table (standard for release).
# The -trimpath flag removes local path info for reproducibility.
#
# Production releases are driven by GoReleaser (see .goreleaser.yaml and
# .github/workflows/release.yml). VERSION here is only used for the snapshot
# label printed by `make prod-build`; the real version on a tagged release is
# the git tag.
VERSION     ?= 0.1.0
# ---------------------------------------------------------------------

.PHONY: setup build prod-build run stop logs destroy clean \
        db-migrate db-reset db-status db-wait \
        test test-pretty \
        lint fmt vet \
        pre-commit-install pre-commit-run vulncheck semgrep socket \
        help

# ============================================================================
# First-time setup
# ============================================================================

# Full first-time setup: copies .env, installs pre-commit hooks, builds containers
setup:
	@echo "Setting up development environment..."
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "Created .env from .env.example — fill in your values. Then run make setup again. Also, make sure that the repo is a git repo. If not setup will fail."; \
		exit 1; \
	fi
	@$(MAKE) pre-commit-install
	@$(MAKE) build
	@echo ""
	@echo "Setup complete. Run 'make help' to see available commands."

# Install and warm up pre-commit hooks
pre-commit-install:
	@echo "Installing pre-commit hooks..."
	@pre-commit install
	@echo "Pre-warming hook cache (downloads happen once per machine)..."
	@pre-commit install --install-hooks
	@echo "Pre-commit hooks installed."

# ============================================================================
# Development
# ============================================================================

# Build containers and run migrations (first time or after container changes)
build:
	@echo "Building development containers..."
	@podman-compose -f $(COMPOSE_FILE) build
	@echo "Starting containers..."
	@podman-compose -f $(COMPOSE_FILE) up -d
	@$(MAKE) db-wait
	@$(MAKE) db-migrate
	@echo "Restarting app to pick up migrated database..."
	@podman-compose -f $(COMPOSE_FILE) restart app
	@echo "Build complete. Application running at http://localhost:8080"
	@echo "Use 'make logs' to view application logs."

# Build a local snapshot of the production release using GoReleaser.
# Produces cross-compiled binaries and archives under dist/ — Linux, macOS,
# and Windows for amd64 and arm64. Skips the container build (so podman
# users don't need to alias `podman` as `docker`) and never publishes.
#
# A real tagged release is cut by pushing a tag — the GitHub Actions
# workflow at .github/workflows/release.yml runs `goreleaser release`
# end-to-end, including the multi-arch container image push to ghcr.io.
#
# Requires goreleaser installed locally: https://goreleaser.com/install/
#
# Runtime reminders for whoever runs the published artefacts:
#   - Generate RSA keys out-of-band and mount as a read-only volume/secret;
#     the binary auto-generates keys on first run, which invalidates tokens
#     across replicas/restarts if the filesystem isn't persistent.
#   - Run goose migrations against the prod DB before deploying a new image
#     (the binary refuses to start if required tables are missing).
#   - Pass DB_*, VALKEY_URL, JWT_*, CORS_ALLOWED_ORIGINS via --env-file or
#     orchestrator secrets — never bake .env into the image.
#   - Block /metrics at the ingress / reverse proxy in production.
prod-build:
	@command -v goreleaser >/dev/null 2>&1 || { \
		echo "goreleaser not found. Install: https://goreleaser.com/install/"; \
		exit 1; \
	}
	@echo "Building snapshot release..."
	@goreleaser release --snapshot --clean --skip=publish,docker
	@echo ""
	@echo "Snapshot artefacts written to dist/."
	@echo "To cut a real release: git tag vX.Y.Z && git push --tags"

# Start containers and run migrations
run:
	@echo "Starting development environment..."
	@podman-compose -f $(COMPOSE_FILE) up -d
	@$(MAKE) db-wait
	@$(MAKE) db-migrate
	@echo "Restarting app to pick up migrated database..."
	@podman-compose -f $(COMPOSE_FILE) restart app
	@echo "Development environment ready at http://localhost:8080"

# View application logs
logs:
	@echo "Viewing application logs (Ctrl+C to exit)..."
	@podman logs -f $(APP_CONTAINER)

# Destroy all containers, volumes, and images
destroy:
	@echo "Stopping and removing containers, volumes, and images..."
	@podman-compose -f $(COMPOSE_FILE) down -v --rmi all
	@echo "Pruning dangling images..."
	@podman image prune -f
	@echo "Cleanup complete."

# Delete all temp, build, and test folders
clean:
	@echo "Cleaning temp, build, and test artifacts..."
	@rm -rf _tmp_/ tmp/ bin/ _BUILD_/ _test_results_/ keys/ dist/
	@rm -rf .golangci-lint-cache/
	@rm -f *.out *.coverprofile *.test .env
	@echo "Clean complete."

# ============================================================================
# Database
# ============================================================================

# Wait for database to be ready
db-wait:
	@echo "Waiting for database to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		podman-compose -f $(COMPOSE_FILE) exec -T db pg_isready -U postgres > /dev/null 2>&1 && break || \
		(echo "Waiting for database... ($$i/10)" && sleep 2); \
	done
	@echo "Database is ready."

# Run pending migrations
db-migrate:
	@echo "Running database migrations..."
	@podman-compose -f $(COMPOSE_FILE) exec -T app sh -c \
		'goose -dir /app/migrations postgres \
		"host=$$DB_HOST port=$$DB_PORT user=$$DB_USER password=$$DB_PASSWORD dbname=$$DB_NAME sslmode=$$DB_SSLMODE" up' || \
		(echo "Migration failed. Make sure the app container is running." && exit 1)
	@echo "Migrations complete."

# Rollback all migrations
db-reset:
	@echo "Resetting database..."
	@podman-compose -f $(COMPOSE_FILE) exec -T app sh -c \
		'goose -dir /app/migrations postgres \
		"host=$$DB_HOST port=$$DB_PORT user=$$DB_USER password=$$DB_PASSWORD dbname=$$DB_NAME sslmode=$$DB_SSLMODE" reset'
	@echo "Database reset complete."

# Check migration status
db-status:
	@echo "Checking migration status..."
	@podman-compose -f $(COMPOSE_FILE) exec -T app sh -c \
		'goose -dir /app/migrations postgres \
		"host=$$DB_HOST port=$$DB_PORT user=$$DB_USER password=$$DB_PASSWORD dbname=$$DB_NAME sslmode=$$DB_SSLMODE" status'

# ============================================================================
# Code quality
# ============================================================================

# Run all tests
test:
	@go test ./...

# Run tests with formatted table output
test-pretty:
	@go run tests/test_runner.go

# Run golangci-lint
lint:
	@golangci-lint run

# Format all Go files
fmt:
	@gofmt -l -w .

# Run go vet
vet:
	@go vet ./...

# Run all pre-commit hooks manually against all files
pre-commit-run:
	@pre-commit run --all-files

# Run govulncheck against all packages
vulncheck:
	@govulncheck -show verbose ./...

# Run semgrep with auto config
semgrep:
	@semgrep --config=auto --error --skip-unknown-extensions .

# Run Socket.dev supply chain scan
# Requires: npm install -g socket && socket login
# Scans all dependencies for known malicious packages and supply chain risks.
socket:
	@socket scan create .

# ============================================================================
# Help
# ============================================================================

help:
	@echo ""
	@echo "Development Commands"
	@echo "--------------------"
	@echo "  setup            First-time setup: copies .env, installs hooks, builds containers"
	@echo "  build            Build dev containers and run migrations"
	@echo "  prod-build       Snapshot the production release locally (cross-compiled binaries in dist/)"
	@echo "  run              Start containers and run migrations"
	@echo "  logs             View application logs"
	@echo "  destroy          Destroy all containers, volumes, and images"
	@echo "  clean            Delete all temp, build, and test folders"
	@echo ""
	@echo "Database"
	@echo "--------"
	@echo "  db-migrate       Run pending migrations"
	@echo "  db-reset         Rollback all migrations"
	@echo "  db-status        Check migration status"
	@echo "  db-wait          Wait for database to be ready"
	@echo ""
	@echo "Code Quality"
	@echo "------------"
	@echo "  test             Run all tests"
	@echo "  test-pretty      Run tests with formatted table output"
	@echo "  lint             Run golangci-lint"
	@echo "  fmt              Format all Go files"
	@echo "  vet              Run go vet"
	@echo "  pre-commit-run   Run all pre-commit hooks against all files"
	@echo "  vulncheck        Run govulncheck against all packages"
	@echo "  semgrep          Run semgrep security scan"
	@echo "  socket           Run Socket.dev supply chain scan"
	@echo ""
	@echo "Typical workflow"
	@echo "----------------"
	@echo "  First time:  make setup"
	@echo "  Daily:       make run -> make logs"
	@echo "  Fresh start: make destroy -> make build"
	@echo "  Tidy up:     make clean"
	@echo ""
