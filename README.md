# Go API Template

A production-ready Go API template using Domain-Driven Design (DDD). The `log` domain is included as a working reference — copy its structure for every new domain you add.

---

## Structure

```
.
├── api/v1/openapi.yaml          # OpenAPI spec
├── cmd/
│   └── api/main.go              # Server entry point (routing, graceful shutdown)
├── internal/
│   ├── config/config.go         # Centralised configuration from environment variables
│   ├── db/db.go                 # Database connection, pool, health check, transactions
│   ├── log/                     # Reference domain (copy this for new domains)
│   │   ├── log_model.go         # Domain types and request structs
│   │   ├── log_errors.go        # Sentinel errors and logger interface alias
│   │   ├── log_repository.go    # SQL queries, prepared statements
│   │   ├── log_service.go       # Business logic
│   │   ├── log_handler.go       # HTTP handlers, routing helpers
│   │   └── log_test.go          # Integration tests using sqlmock
│   ├── metrics/metrics.go       # Prometheus instrumentation and /metrics endpoint
│   ├── middleware/
│   │   ├── cors_middleware.go      # CORS for cross-origin requests
│   │   └── security_middleware.go  # Security headers (CSP, X-Frame-Options, etc.)
│   └── shared/
│       ├── limits.go            # App-wide content limits and LimitExceededError
│       ├── validation.go        # Pagination, null byte sanitisation, WriteUnauthorised
│       └── logger/logger.go     # Canonical Logger interface (slog-based)
├── migrations/                  # Goose SQL migrations
├── tests/test_runner.go         # Pretty test output with JSON results
├── compose.dev.yaml             # Local dev: Postgres + app with hot reload
├── container.dev                # Dev container (golang:1.26-alpine + air + goose)
├── container.prod               # Production multi-stage build (distroless, non-root, statically linked)
├── .air.toml                    # Air live reload config
├── .golangci.yml                # golangci-lint v2 config
├── .pre-commit-config.yaml      # gitleaks, gofmt, golangci-lint, govulncheck, semgrep
└── Makefile                     # All development commands
```

---

## Getting started

### Rename the project

Before using this template, rename the module to match your project:

1. Update `go.mod` — change the module name to your module name (e.g., `module github.com/yourname/yourproject`)
2. Find and replace all imports — replace `go-tasks-api/` with your new module path in all `.go` files

### First-time setup: copies .env, installs pre-commit hooks, builds containers
`make setup`

### Start the stack (subsequent runs)
`make run`

### View logs
`make logs`

On first run, `make setup` will copy `.env.example` to `.env` and exit, asking you to fill in your values. Edit `.env` then run `make setup` again.

The directory must be a git repository before running `make setup` — pre-commit requires it:
```bash
git init
git add .
git commit -m "initial commit"
```

Podman must also be running before `make setup` or `make run`:
```bash
podman machine start
```

`make run` is your daily start command — run it each time you open the project after your machine has been off. Air handles hot reload inside the container as you save `.go` files, so you do not need to restart anything during development.

---

## Configuration

All environment variables are read once at startup by the `internal/config` package and stored in a typed `Config` struct. This makes the full configuration surface visible in one place and avoids scattered `os.Getenv` calls across packages.

### How it works

```go
// cmd/api/main.go
cfg, err := config.Load()
if err != nil {
    fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
    os.Exit(1)
}

// Pass config to components that need it
appLogger := logger.NewLogger(cfg.Log.Level)
database, err := db.New(&cfg.Database, appLogger)
```

### Adding a new environment variable

1. Add the field to the appropriate struct in `internal/config/config.go`:

```go
type ServerConfig struct {
    Port         string
    ReadTimeout  time.Duration  // new field
}
```

2. Read the value in `Load()`:

```go
Server: ServerConfig{
    Port:        getEnv("PORT", "8080"),
    ReadTimeout: time.Duration(getEnvInt("SERVER_READ_TIMEOUT_SECS", 10)) * time.Second,
},
```

3. Use it via the config struct — never call `os.Getenv` directly in other packages:

```go
server := &http.Server{
    ReadTimeout: cfg.Server.ReadTimeout,
}
```

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP server port |
| `LOG_LEVEL` | `development` | Log level: `development`, `production`, `quiet`, `silent` |
| `DB_HOST` | — | PostgreSQL host (required) |
| `DB_PORT` | — | PostgreSQL port (required) |
| `DB_USER` | — | PostgreSQL user (required) |
| `DB_PASSWORD` | — | PostgreSQL password (required) |
| `DB_NAME` | — | PostgreSQL database name (required) |
| `DB_SSLMODE` | `require` | PostgreSQL SSL mode |
| `DB_MAX_OPEN_CONNS` | `100` | Maximum open connections |
| `DB_MAX_IDLE_CONNS` | `50` | Maximum idle connections |
| `DB_CONN_MAX_LIFETIME_MINS` | `5` | Maximum connection lifetime in minutes |
| `DB_CONN_MAX_IDLE_TIME_MINS` | `10` | Maximum idle connection time in minutes |
| `CORS_ALLOWED_ORIGINS` | — | Comma-separated allowed origins (e.g. `http://localhost:3000`) |

---

## Authentication

Authentication is **intentionally not implemented** in this template. Auth requirements vary significantly between projects (OAuth, OIDC, API keys, mTLS, etc.), so this template defines the **auth contract** instead — any middleware that validates a token and sets `user_id` in the request context will work automatically.

### The auth contract

1. **Context key** — use `applog.UserContextKey` to set the authenticated user ID in the request context
2. **Repository scoping** — all repository queries are already scoped by `user_id`, so ownership enforcement is in place
3. **TODO marker** — the `TODO` comment in `cmd/api/main.go` marks exactly where auth middleware plugs in
4. **401 handling** — `shared.WriteUnauthorised` and handler error mapping are ready to use

### Implementing auth

Wire your auth middleware in `cmd/api/main.go` where the TODO comment is:

```go
r.Route("/api/v1", func(r chi.Router) {
    // TODO: Wire auth middleware here. Any middleware that validates a token
    // and sets user_id in the request context using applog.UserContextKey
    // will work automatically. All repository queries are scoped by user_id.
    r.Use(authMiddleware.Handler)

    r.Get("/logs", logHandler.ListLogs)
    // ...
})
```

Your middleware must set the user ID in context:

```go
ctx := context.WithValue(r.Context(), applog.UserContextKey, userID)
next.ServeHTTP(w, r.WithContext(ctx))
```

### Recommended libraries

- [`github.com/coreos/go-oidc/v3`](https://github.com/coreos/go-oidc) — OIDC client and token verification
- [`golang.org/x/oauth2`](https://pkg.go.dev/golang.org/x/oauth2) — OAuth 2.0 client

### Claims to validate

When implementing JWT validation, verify these claims:

| Claim | Validation |
|-------|------------|
| `iss` | Must match your expected issuer |
| `aud` | Must match your API identifier |
| `exp` | Must not be expired |
| `nbf` | Must not be used before this time |
| `alg` | **Server-side whitelist only** — never read from token header. Enforce RS256. |
| `kid` | Must match a key in your JWKS |

See the [OWASP JWT Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/JSON_Web_Token_for_Java_Cheat_Sheet.html) for comprehensive JWT security guidance.

---

## Adding a new domain

The `log` domain is the pattern to follow. Every domain is fully self-contained — no cross-domain imports. Copy the folder and rename all files and symbols.

### 1. Create the domain folder

```bash
cp -r internal/log internal/order
```

Rename every file:

```
internal/order/
├── order_model.go
├── order_errors.go
├── order_repository.go
├── order_service.go
├── order_handler.go
└── order_test.go
```

### 2. Update each file

**`order_model.go`** — define your domain struct and request type:

```go
type Order struct {
    ID        string    `json:"id"`
    UserID    string    `json:"user_id"`
    Total     int       `json:"total"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type OrderRequest struct {
    Total int `json:"total" validate:"required,gt=0"`
}
```

**`order_errors.go`** — define sentinel errors for your domain. The logger alias is already correct:

```go
type logLogger = logger.Logger

var (
    ErrOrderNotFound = errors.New("order not found")
    ErrDatabase      = errors.New("database error")
    // ...
)
```

**`order_repository.go`** — write your SQL queries as constants, prepare them all in `NewOrderRepository`, and remove the `if stmt != nil` pattern — always panic on prepare failure:

```go
const queryGetOrder = `SELECT id, user_id, total, created_at, updated_at FROM orders WHERE id = $1 AND user_id = $2`

func NewOrderRepository(db *sql.DB, log logLogger) orderRepository {
    repo := &sqlOrderRepository{logger: log}
    var err error
    repo.stmtGetOrder, err = db.Prepare(queryGetOrder)
    if err != nil {
        panic(fmt.Sprintf("order_repository: failed to prepare getOrder: %v", err))
    }
    // ...
    return repo
}
```

**`order_service.go`** — business logic only. No HTTP, no SQL. Validate inputs and call the repository:

```go
func (s *defaultOrderService) getOrder(ctx context.Context, id, userID string) (Order, error) {
    if id == "" || userID == "" {
        return Order{}, ErrMissingParameters
    }
    return s.repo.getOrder(ctx, id, userID)
}
```

**`order_handler.go`** — HTTP only. Parse request, call service, write response. The `responseJSON` helper marshals to a buffer before writing the header, preventing partial responses on encode errors. Keep this pattern:

```go
func NewOrderHandler(service orderService, log logLogger) *OrderHandler {
    return &OrderHandler{
        service:   service,
        logger:    log,
        validate:  validator.New(),
        sanitiser: bluemonday.StrictPolicy(),
    }
}
```

Authentication is wired via context. Use `getUserID()` (defined in your handler file) to retrieve the user ID:

```go
userID := getUserID(r.Context())
if userID == "" {
    h.handleError(w, ErrUnauthorised)
    return
}
```

The auth middleware (which you implement per-project) sets `UserContextKey` in the request context after validating the access token.

### 3. Write a migration

```sql
-- migrations/00002_create_orders.sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE orders (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL,
    total      INTEGER     NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_orders_user_id ON orders(user_id);

CREATE TRIGGER update_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS orders CASCADE;
-- +goose StatementEnd
```

Add `"orders"` to `requiredTables` in `internal/db/db.go`:

```go
var requiredTables = []string{
    "logs",
    "orders",
}
```

### 4. Wire it up in `cmd/api/main.go`

```go
orderRepo    := order.NewOrderRepository(database.SQL(), appLogger)
orderService := order.NewOrderService(orderRepo, appLogger)
orderHandler := order.NewOrderHandler(orderService, appLogger)

r.Route("/api/v1", func(r chi.Router) {
    r.Get("/orders/{id}", orderHandler.GetOrder)
    r.Post("/orders",     orderHandler.CreateOrder)
    // ...
})
```

Append to the graceful shutdown block:

```go
if err := orderRepo.Close(); err != nil {
    fmt.Println("Order repository close error:", err)
}
```

### 5. Add to the OpenAPI spec

Document the new endpoints in `api/v1/openapi.yaml` following the existing `Log` schema pattern.

---

## Database

The `DB` struct in `internal/db/db.go` wraps `*sql.DB` and is passed explicitly to repositories. There are no package-level globals. This makes testing straightforward — pass a `*sql.DB` backed by `sqlmock` in tests.

Database configuration is handled by the `internal/config` package — see the [Configuration](#configuration) section for all available environment variables.

At startup, `db.New()` verifies that all tables in `requiredTables` exist. If any are missing it prints a clear error and exits. Run `make db-migrate` before starting the app.

`WithTransaction` is available on the `DB` struct for multi-step operations:

```go
err := database.WithTransaction(ctx, func(tx *sql.Tx) error {
    // all operations here are atomic
    return nil
})
```

---

## Observability

### Prometheus metrics

The application exposes a `/metrics` endpoint in Prometheus text format. All HTTP requests are automatically instrumented.

**WARNING:** The `/metrics` endpoint has no authentication. In production, restrict access via network policy, reverse proxy, or move to a separate internal port. Exposed metrics can reveal system internals.

**Available metrics:**

| Metric | Type | Description |
|---|---|---|
| `http_requests_total` | Counter | Total HTTP requests by method, path, and status code |
| `http_request_duration_seconds` | Histogram | Request duration with buckets from 5ms to 10s |
| `http_requests_in_flight` | Gauge | Current number of requests being processed |
| `http_response_size_bytes` | Histogram | Response size in bytes |

**Path normalisation:** UUIDs and numeric IDs in paths are replaced with `:id` to prevent label cardinality explosion. For example, `/api/v1/logs/550e8400-e29b-41d4-a716-446655440000` becomes `/api/v1/logs/:id`.

### Adding custom metrics

Add new metrics in `internal/metrics/metrics.go`:

```go
// Add to the Metrics struct
myCounter *prometheus.CounterVec

// Register in New()
m.myCounter = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "my_custom_metric_total",
        Help: "Description of what this metric measures.",
    },
    []string{"label1", "label2"},
)
prometheus.MustRegister(m.myCounter)

// Use it in your code
appMetrics.myCounter.WithLabelValues("value1", "value2").Inc()
```

### What is not included — and why

The Prometheus **server** and **Grafana** are not in this repository, and that is intentional.

Your application is a metrics *producer*. Prometheus and Grafana are the *consumers*. These are different concerns and should live in different repositories:

- Grafana dashboard JSON changes when you change what you want to graph — not when you change business logic. Coupling them forces unrelated commits.
- Prometheus config changes when your deployment topology changes — when you add a new service, scale horizontally, or add a staging environment. That is an infrastructure concern, not an application concern.
- In any real deployment you want **one** observability stack watching **multiple** services. If Prometheus lives inside each app's repo, that becomes impossible.

The pattern used by mature projects (including PocketBase, Caddy, and Gitea) is: the application exposes `/metrics`, and a separate infrastructure repository owns the Prometheus and Grafana configuration that scrapes it.

Create a separate `<your-project>-infra` repository with a `compose.yaml` that brings up Prometheus and Grafana pointed at your running application.

---

## HTTP security headers

The `SecurityHeaders` middleware in `internal/middleware/security_middleware.go` sets these headers on all responses. See [MDN's Security Practical Implementation Guides](https://developer.mozilla.org/en-US/docs/Web/Security/Practical_implementation_guides) for detailed explanations.

| Header | Value | Purpose |
|---|---|---|
| `Content-Security-Policy` | `default-src 'none'; frame-ancestors 'none'` | Strict CSP for JSON APIs; `frame-ancestors` prevents iframe embedding |
| `X-Content-Type-Options` | `nosniff` | Prevents MIME-sniffing |
| `Cache-Control` | `no-store` | Prevents caching of authenticated responses |
| `Referrer-Policy` | `no-referrer` | Prevents URL/token leakage |

### CORS

The `CORS` middleware in `internal/middleware/cors_middleware.go` handles cross-origin requests. Configure allowed origins via `CORS_ALLOWED_ORIGINS` (comma-separated).

- Never use `*` in production — always specify explicit origins
- If your API has no browser clients (server-to-server only), remove the CORS middleware entirely

### Handled at infrastructure layer

**Strict-Transport-Security (HSTS)** — belongs at your reverse proxy or load balancer, not the app. Your app runs HTTP inside the container; TLS termination happens outside it.

**X-Request-ID** — already added by chi's `middleware.RequestID`.

**Rate limiting** — implement at your reverse proxy, API gateway, or load balancer (e.g., nginx `limit_req`, Cloudflare, AWS WAF). Infrastructure-level rate limiting is more effective because it can reject requests before they reach your application, protecting against resource exhaustion. Application-level rate limiting requires additional state management (Redis/Valkey) and still consumes application resources for every request.

---

## Coding security rules

All code in this template must follow these rules:

1. **Sanitise before validate** — every user input must be sanitised before validation
2. **Type and validate inputs** — every user input must be assigned to a type and validated against that type before use
3. **Validate database inputs** — every value going to the database must be validated against its type before the query is executed
4. **Validate database outputs** — every value coming from the database must be validated against its type before use
5. **Authenticate by default** — every API endpoint must require authentication unless explicitly excluded
6. **Authorise in repository** — in the repository layer, always check that the authenticated user is authorised to access the requested data before returning it
7. **Validate file uploads** — every file upload must be validated against its MIME type, file extension, and size limit

---

## Security tooling

Pre-commit hooks run on every commit:

| Hook | What it catches |
|---|---|
| `gitleaks` | Secrets accidentally committed |
| `gofmt` | Formatting violations |
| `golangci-lint` | Static analysis, security checks (gosec), style |
| `govulncheck` | Known CVEs in your dependencies |
| `semgrep` | Security anti-patterns |

Run all hooks manually at any time:

```bash
make pre-commit-run
```

Run security scans individually:

```bash
make vulncheck   # govulncheck ./...
make semgrep     # semgrep --config=auto
```

---

## Makefile reference

| Command | Description |
|---|---|
| `make setup` | First-time setup: copy `.env`, install hooks, build containers |
| `make build` | Build containers and run migrations |
| `make run` | Start containers and run migrations |
| `make logs` | Tail application logs |
| `make destroy` | Remove all containers, volumes, and images |
| `make db-migrate` | Run pending migrations |
| `make db-reset` | Roll back all migrations |
| `make db-status` | Check migration status |
| `make test` | Run all tests |
| `make test-pretty` | Run tests with formatted table output |
| `make lint` | Run golangci-lint |
| `make fmt` | Format all Go files |
| `make vet` | Run go vet |
| `make vulncheck` | Run govulncheck |
| `make semgrep` | Run semgrep |
| `make pre-commit-run` | Run all pre-commit hooks against all files |
