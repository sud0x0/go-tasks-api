# ============================================================================
# THIS IS FOR LOCAL DEVELOPMENT ONLY
# ============================================================================
# Requires .env file - podman-compose will automatically load it

COMPOSE_FILE=compose.dev.yaml
DB_CONTAINER=app_db
APP_CONTAINER=app_api

# --- Release artefact metadata --------------------------------------------
#
# Production releases are cut by pushing a `v*` tag — .github/workflows/release.yml
# runs goreleaser end-to-end: cross-compiles the api + migrator binaries,
# generates SPDX-JSON SBOMs, computes SHA-256 checksums, and publishes a
# GitHub Release with SLSA Level 3 build provenance.
#
# `make prod-build` reproduces the snapshot bundle locally (without publishing
# or signing); useful for smoke-testing the build before tagging.
# `make goreleaser-check` runs the full pipeline the CI workflow performs,
# so you can validate everything before pushing a release tag.
#
# Version is bumped manually. Commit SHA and build date are captured at
# build time. All three are injected into internal/version via -ldflags.
# VERSION here is only used for the snapshot label printed by `make prod-build`;
# the real version on a tagged release is the git tag.
VERSION ?= 0.1.0
# --------------------------------------------------------------------------

.PHONY: setup pre-commit-install \
        build run logs destroy clean \
        db-wait db-migrate db-reset db-status \
        prod-build goreleaser-check \
        test test-pretty lint fmt vet \
        pre-commit-run vulncheck semgrep socket \
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

# Install and warm up pre-commit hooks. The commit-msg hook is installed in
# addition to the default pre-commit hook so Conventional Commits enforcement
# kicks in on commit messages, not just staged files.
pre-commit-install:
	@echo "Installing pre-commit hooks..."
	@pre-commit install
	@pre-commit install --hook-type commit-msg
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

# Delete all temp, build, test, and release artifacts. `dist/` is the
# GoReleaser snapshot/release output — also covered by .gitignore.
clean:
	@echo "Cleaning temp, build, test, and release artifacts..."
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
# Release
# ============================================================================

# Build a local snapshot of the production release using GoReleaser.
# Produces cross-compiled binaries and archives under dist/ — Linux, macOS,
# and Windows for amd64 and arm64. Skips the SBOM step (so syft isn't required)
# and never publishes.
#
# A real tagged release is cut by pushing a tag — the GitHub Actions
# workflow at .github/workflows/release.yml runs `goreleaser release`
# end-to-end, including SBOM generation and SLSA Level 3 provenance.
#
# Requires goreleaser installed locally: https://goreleaser.com/install/
#
# Runtime reminders for whoever runs the published binaries:
#   - Generate RSA keys out-of-band and mount as a read-only volume/secret;
#     the binary auto-generates keys on first run, which invalidates tokens
#     across replicas/restarts if the filesystem isn't persistent.
#   - Run the database migrator against the prod DB before deploying a new
#     API binary (the API refuses to start if required tables are missing).
#   - Pass DB_*, VALKEY_URL, JWT_*, CORS_ALLOWED_ORIGINS via --env-file or
#     orchestrator secrets.
#   - Block /metrics at the ingress / reverse proxy in production.
prod-build:
	@command -v goreleaser >/dev/null 2>&1 || { \
		echo "goreleaser not found. Install: https://goreleaser.com/install/"; \
		exit 1; \
	}
	@echo "Building snapshot release..."
	@goreleaser release --snapshot --clean --skip=sbom
	@echo ""
	@echo "Snapshot artefacts written to dist/. (SBOM generation skipped — install syft to include it.)"
	@echo "To cut a real release: git tag vX.Y.Z && git push --tags"

# Validate the release pipeline end-to-end against a local snapshot. Mirrors
# the steps in .github/workflows/release.yml (config check, full snapshot
# build, SLSA subjects extraction via the workflow's jq filter, version-banner
# smoke test on native-arch binaries) so you can catch issues before pushing
# a tag. Catches issues that `goreleaser check` alone misses — for example,
# a malformed jq filter that only manifests when applied to a real
# dist/artifacts.json.
#
# Run this before pushing a release tag.
goreleaser-check:
	@command -v goreleaser >/dev/null 2>&1 || { \
		echo "goreleaser not found. Install: https://goreleaser.com/install/"; \
		exit 1; \
	}
	@command -v jq >/dev/null 2>&1 || { \
		echo "jq not found. Install jq for the subjects-extraction step."; \
		exit 1; \
	}
	@echo "==> Validating goreleaser config..."
	@goreleaser check
	@echo ""
	@echo "==> Building full snapshot release..."
	@if command -v syft >/dev/null 2>&1; then \
		goreleaser release --snapshot --clean; \
	else \
		echo "(syft not installed — skipping SBOM step)"; \
		goreleaser release --snapshot --clean --skip=sbom; \
	fi
	@echo ""
	@echo "==> Verifying SLSA subject extraction matches the workflow filter..."
	@HASHES=$$(jq -r '.[] | select(.type=="Binary" and .extra.Checksum != null) | "\(.extra.Checksum | sub("^sha256:"; ""))  \(.name)"' dist/artifacts.json); \
	COUNT=$$(printf '%s\n' "$$HASHES" | grep -c .); \
	if [ "$$COUNT" -eq 0 ]; then \
		echo "FAIL: jq filter produced no Binary subjects — check .github/workflows/release.yml"; \
		exit 1; \
	fi; \
	echo "OK: $$COUNT subjects extracted:"; \
	printf '%s\n' "$$HASHES" | sed 's/^/    /'
	@echo ""
	@echo "==> Smoke-testing native-arch binaries..."
	@HOST_OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	HOST_ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/'); \
	API_DIR=$$(find dist -maxdepth 1 -type d -name "api_$${HOST_OS}_$${HOST_ARCH}*" | head -1); \
	MIG_DIR=$$(find dist -maxdepth 1 -type d -name "migrator_$${HOST_OS}_$${HOST_ARCH}*" | head -1); \
	if [ -z "$$API_DIR" ] || [ -z "$$MIG_DIR" ]; then \
		echo "WARN: no native-arch build found for $$HOST_OS/$$HOST_ARCH — skipping binary smoke test"; \
	else \
		"$$API_DIR/go-tasks-api" --version >/dev/null && echo "OK: go-tasks-api --version"; \
		"$$MIG_DIR/go-tasks-database-migrator" --version >/dev/null && echo "OK: go-tasks-database-migrator --version"; \
	fi
	@echo ""
	@echo "All goreleaser checks passed. Safe to push a release tag."

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
	@echo "Development"
	@echo "-----------"
	@echo "  setup            First-time setup: copies .env, installs hooks, builds containers"
	@echo "  build            Build dev containers and run migrations"
	@echo "  run              Start containers and run migrations"
	@echo "  logs             View application logs"
	@echo "  destroy          Destroy all containers, volumes, and images"
	@echo "  clean            Delete all temp, build, test, and release artifacts"
	@echo ""
	@echo "Database"
	@echo "--------"
	@echo "  db-migrate       Run pending migrations"
	@echo "  db-reset         Rollback all migrations"
	@echo "  db-status        Check migration status"
	@echo "  db-wait          Wait for database to be ready"
	@echo ""
	@echo "Release"
	@echo "-------"
	@echo "  prod-build       Snapshot the production release locally (cross-compiled binaries in dist/)"
	@echo "  goreleaser-check Validate the release pipeline end-to-end (run before pushing a tag)"
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
