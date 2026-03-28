# API Service — Backend Design Spec

This document is the authoritative low-level design reference for the `api`
service backend. All implementation work in `api/` should conform to the
interfaces, types, and conventions defined here.

---

## 1. Package Layout

**Implemented:**

```
api/
  main.go                          # config → DB → router → server (testable run() pattern)
  main_test.go                     # tests for run() lifecycle
  Makefile                         # build, test, lint, coverage, run targets
  specs/backend.md                 # this document
  internal/
    config/config.go               # parse config.yaml + env var overlay
    postgres/
      db.go                        # pgxpool init, embedded migrations, ping
      db_test.go                   # unit + integration tests
      migrations/
        001_create_ingestion_tables.up.sql
        001_create_ingestion_tables.down.sql
    store/
      store.go                     # VulnStore, ScoreStore, RunStore interfaces + domain types
    middleware/middleware.go        # requestID, recovery, logger, metrics, auth
    handler/
      health.go                    # GET /health
      vuln.go                      # GET/POST/PUT /api/v1/vulns[/{id}] (501 stubs)
      score.go                     # GET/POST /api/v1/vulns/{id}/scores (501 stubs)
      run.go                       # GET/POST /api/v1/runs/{ingestion,newsletter} (501 stubs)
    response/response.go           # Write(w, status, errType, detail, result)
    router/router.go               # chi mux, route registration, sub-router mounting
```

**Planned:**

```
  internal/
    store/
      postgres/
        vuln.go                    # pgx implementation of VulnStore
        score.go                   # pgx implementation of ScoreStore
        run.go                     # pgx implementation of RunStore
    metrics/metrics.go             # Prometheus metric registrations
```

**Rationale:**

- `internal/store/store.go` is a leaf package — defines interfaces and domain
  types; no internal imports from within the `api/` module. This prevents
  circular dependencies.
- `internal/postgres/` owns the connection pool, embedded migrations, and
  database lifecycle. The package is named `postgres` (not `db`) to make the
  pgx/PostgreSQL coupling explicit.
- `internal/store/postgres/` (planned) will import `store` and `pgx/v5`;
  handlers never import `postgres` directly. The concrete pgx implementations
  are swappable and easily mocked in tests.
- Handlers import `store` (interface) only — the concrete implementation is
  injected by `router.New`.
- No `init()` self-registration anywhere. All wiring is explicit in `main.go`.

---

## 2. Configuration (`internal/config`)

```go
type Config struct {
    API      APIConfig      `yaml:"api"`
    Database DatabaseConfig `yaml:"database"`
}

type APIConfig struct {
    Listen         string `yaml:"listen"`          // default ":8080"
    InternalSecret string `yaml:"internal_secret"` // env: API_INTERNAL_SECRET
}

type DatabaseConfig struct {
    DSN string `yaml:"dsn"` // env: DATABASE_URL
}

// Load reads path, overlays environment variables, and validates required
// fields. Returns an error if the file is missing, malformed, or a required
// field is empty after the overlay.
func Load(path string) (Config, error)
```

**Environment overlay** (applied after YAML parse, overrides non-empty env
values):

| Env var | Config field |
|---------|-------------|
| `API_INTERNAL_SECRET` | `API.InternalSecret` |
| `DATABASE_URL` | `Database.DSN` |

**Required after overlay:** `API.InternalSecret`, `Database.DSN`. `Listen`
defaults to `":8080"` when absent or empty. Missing or empty required fields
cause `main.go` to call `log.Fatalf` and exit non-zero.

---

## 3. Database (`internal/postgres`)

```go
// New opens a pgxpool connection pool, runs any pending embedded migrations,
// and pings the database within a 5-second deadline. Returns an error if
// migrations fail, the pool cannot be created, or the ping fails.
// Caller is responsible for calling pool.Close() at shutdown.
func New(ctx context.Context, dsn string, log *slog.Logger) (*pgxpool.Pool, error)
```

The function performs three steps in order:

1. **Run migrations** — SQL files are embedded via `//go:embed migrations/*.sql`.
   The golang-migrate `iofs` source driver reads from the embedded FS, and the
   `pgx5` database driver applies them. `postgres://` DSNs are converted to
   `pgx5://` for driver compatibility. `migrate.ErrNoChange` is treated as
   success (idempotent on restart).
2. **Create pool** — `pgxpool.New(ctx, dsn)` opens the connection pool.
3. **Ping** — `pool.Ping` with a 5-second `context.WithTimeout` verifies
   connectivity. On failure the pool is closed before returning the error.

Structured logging is emitted at each step: migration state (version, dirty
flag), pool creation, ping attempt, and final connectivity confirmation.

A failed `New` at startup causes `run()` to return an error, which `main()`
logs and exits with code 1.

---

## 4. Store Layer (`internal/store`)

### 4a. Domain Types

Declared in `store.go`. Used by repository interfaces, handler packages, and
`main.go`. No other package in `api/internal/` is imported here.

```go
type Vuln struct {
    ID          string
    CVEID       string
    GHSAID      string
    EDBID       string
    Source      string
    Title       string
    Description string
    CVSSScore   float64
    CVSSVector  string
    PublishedAt *time.Time
    UpdatedAt   *time.Time
    IngestedAt  time.Time
    RawJSON     json.RawMessage
}

type Score struct {
    ID              string
    VulnID          string
    Score           int
    Rationale       string
    ScoredAt        time.Time
    NewsletterRunID *string
}

type IngestionRun struct {
    ID           string
    Source       string
    StartedAt    time.Time
    FinishedAt   *time.Time
    ItemsFetched int
    ItemsNew     int
}

type NewsletterRun struct {
    ID              string
    RunAt           time.Time
    ItemCount       int
    DeliveryTargets json.RawMessage
}

// ListFilters is passed to VulnStore.List. Zero values are ignored (no filter
// applied for that field). Scored filters on whether the vuln has at least one
// score record.
type ListFilters struct {
    Source  string
    MinCVSS *float64
    MaxCVSS *float64
    Since   *time.Time
    Until   *time.Time
    Scored  *bool
}
```

### 4b. Repository Interfaces

```go
type VulnStore interface {
    // Create inserts a new vulnerability record and returns the assigned UUID.
    Create(ctx context.Context, v Vuln) (id string, err error)

    // GetByID returns a single vulnerability by its database UUID.
    // Returns pgx.ErrNoRows if not found.
    GetByID(ctx context.Context, id string) (Vuln, error)

    // GetByCanonicalID looks up by cve_id, ghsa_id, or edb_id (UNIQUE columns).
    // Returns the record and found=true if exactly one match exists.
    // Returns found=false (no error) when no record matches any of the IDs.
    GetByCanonicalID(ctx context.Context, cveID, ghsaID, edbID string) (Vuln, bool, error)

    // List returns vulnerabilities matching f. All filter fields are optional;
    // zero values are ignored.
    List(ctx context.Context, f ListFilters) ([]Vuln, error)

    // Update replaces the mutable fields of an existing record by UUID.
    // Returns pgx.ErrNoRows if id does not exist.
    Update(ctx context.Context, id string, v Vuln) error
}

type ScoreStore interface {
    // ListByVulnID returns all scores for the given vulnerability UUID.
    ListByVulnID(ctx context.Context, vulnID string) ([]Score, error)

    // Create inserts a new score record.
    Create(ctx context.Context, s Score) error
}

type RunStore interface {
    ListIngestion(ctx context.Context) ([]IngestionRun, error)
    CreateIngestion(ctx context.Context, r IngestionRun) error
    ListNewsletter(ctx context.Context) ([]NewsletterRun, error)
    CreateNewsletter(ctx context.Context, r NewsletterRun) error

    // LastSuccessfulIngestion returns finished_at for the most recent completed
    // ingestion run for source. Returns a zero time.Time if no run exists.
    LastSuccessfulIngestion(ctx context.Context, source string) (time.Time, error)
}
```

### 4c. Store Error Conventions

Handlers map store errors to HTTP statuses as follows:

| Store error | HTTP status |
|-------------|-------------|
| `pgx.ErrNoRows` | 404 `not_found` |
| `*pgconn.PgError` code `"23505"` (unique violation) | 409 `conflict` |
| All other errors | 500 `internal_error` |

Handlers must not expose raw error text to callers — only the error codes
defined in §8 are written to the response.

---

## 5. Metrics (`internal/metrics`)

Package-level Prometheus variables. `Register` is called once at startup with
the default Prometheus registerer (or a custom one in tests).

```go
// Register registers all API metrics with reg. Returns an error if any metric
// fails to register (e.g. duplicate registration in tests).
func Register(reg prometheus.Registerer) error
```

| Metric | Type | Labels |
|--------|------|--------|
| `api_http_requests_total` | Counter | `method`, `path`, `status` |
| `api_http_request_duration_seconds` | Histogram | `method`, `path` |
| `api_http_requests_in_flight` | Gauge | — |

**`path` label** — uses the chi route pattern (e.g. `/api/v1/vulns/{id}`), not
the raw URL. This prevents high cardinality from UUIDs in the path.

Histogram buckets: `prometheus.DefBuckets`.

---

## 6. Middleware (`internal/middleware`)

Five middleware constructors, each returning `func(http.Handler) http.Handler`.
An unexported `wrappedResponseWriter` captures the status code for `Logger`
and `Metrics`; it satisfies `http.ResponseWriter` and optionally
`http.Flusher`.

```go
// RequestID generates a UUID request ID, sets it on the context (accessible
// via RequestIDFromContext), and copies it to the X-Request-ID response header.
// If the incoming request already carries X-Request-ID, that value is
// propagated as-is.
func RequestID(log *slog.Logger) func(http.Handler) http.Handler

// Recovery catches panics, logs the stack trace at Error level with the
// request ID, and writes a 500 internal_error response.
func Recovery(log *slog.Logger) func(http.Handler) http.Handler

// Logger logs each request at Info level: method, path, status, duration
// (ms), and request ID.
func Logger(log *slog.Logger) func(http.Handler) http.Handler

// Metrics records request count (with status label), duration histogram, and
// in-flight gauge using the vars registered by metrics.Register.
func Metrics() func(http.Handler) http.Handler

// Auth validates the X-Internal-Secret header. Responds 401 with error code
// "unauthorized" if the header is absent or does not match secret.
// Applied to the /api/v1/ sub-router only — /health and /metrics are public.
func Auth(secret string) func(http.Handler) http.Handler
```

Named constants (prevents magic strings throughout the middleware package):

```go
const (
    headerInternalSecret   = "X-Internal-Secret"
    headerRequestID        = "X-Request-ID"
    errUnauthorized        = "unauthorized"
    errDetailMissingSecret = "X-Internal-Secret header is missing or invalid"
)
```

Exported helper for handlers that need the request ID for structured logging:

```go
// RequestIDFromContext returns the request ID stored by RequestID middleware,
// or an empty string if none is present.
func RequestIDFromContext(ctx context.Context) string
```

An unexported `contextKey` type is used to avoid collisions in context value
storage:

```go
type contextKey int

const requestIDKey contextKey = iota
```

---

## 7. Router (`internal/router`)

```go
// New wires a chi mux with all middleware and routes. The returned handler
// is passed directly to http.Server.
func New(
    vulns  store.VulnStore,
    scores store.ScoreStore,
    runs   store.RunStore,
    secret string,
    log    *slog.Logger,
) http.Handler
```

**Middleware stack:**

```
global: RequestID → Recovery → Logger → Metrics
  /health          (no auth)
  /metrics         (no auth — Prometheus scrape endpoint)
  /api/v1/         Auth → ...routes
```

**Handler constructor pattern** — each handler is a function that closes over
its store dependency and returns an `http.HandlerFunc`. This avoids a
handler struct and keeps injection explicit:

```go
// Good
func GetVuln(vulns store.VulnStore) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) { ... }
}
```

**Route table:**

| Method | Pattern | Handler |
|--------|---------|---------|
| `GET` | `/health` | `handler.Health` |
| `GET` | `/metrics` | `promhttp.Handler()` |
| `GET` | `/api/v1/vulns` | `handler.ListVulns(vulns)` |
| `GET` | `/api/v1/vulns/{id}` | `handler.GetVuln(vulns)` |
| `POST` | `/api/v1/vulns` | `handler.CreateVuln(vulns)` |
| `PUT` | `/api/v1/vulns/{id}` | `handler.UpdateVuln(vulns)` |
| `GET` | `/api/v1/vulns/{id}/scores` | `handler.ListScores(scores)` |
| `POST` | `/api/v1/vulns/{id}/scores` | `handler.CreateScore(scores)` |
| `GET` | `/api/v1/runs/ingestion` | `handler.ListIngestionRuns(runs)` |
| `POST` | `/api/v1/runs/ingestion` | `handler.CreateIngestionRun(runs)` |
| `GET` | `/api/v1/runs/newsletter` | `handler.ListNewsletterRuns(runs)` |
| `POST` | `/api/v1/runs/newsletter` | `handler.CreateNewsletterRun(runs)` |

The `/api/v1/` sub-router is mounted with `r.Route("/api/v1", ...)` and has
`middleware.Auth(secret)` applied via `r.Use(...)`.

**`GET /api/v1/vulns` dual behaviour:** when any of `cve_id`, `ghsa_id`, or
`edb_id` is present as a query parameter, the handler calls
`VulnStore.GetByCanonicalID` and returns a single object or 404. When none of
those params are present, it calls `VulnStore.List(filters)` and returns an
array.

---

## 8. Handler Conventions (`internal/handler/`)

All handlers call `response.Write` — never `json.NewEncoder` or
`http.Error` directly. Named request and response types are declared at the
package level (never anonymous structs inside function bodies, per CLAUDE.md).

**Error constants per file:**

```go
const (
    errBadRequest = "bad_request"
    errNotFound   = "not_found"
    errConflict   = "conflict"
    errInternal   = "internal_error"
)
```

**Error → status mapping:**

| Scenario | Status | Error code |
|----------|--------|------------|
| Malformed JSON body | 400 | `bad_request` |
| Invalid query parameter | 400 | `bad_request` |
| `pgx.ErrNoRows` from store | 404 | `not_found` |
| `PgError` code `23505` from store | 409 | `conflict` |
| Any other store error | 500 | `internal_error` |

**List query params for `GET /api/v1/vulns`:**

| Param | Type | Maps to |
|-------|------|---------|
| `source` | string | `ListFilters.Source` |
| `min_cvss` | float | `ListFilters.MinCVSS` |
| `max_cvss` | float | `ListFilters.MaxCVSS` |
| `since` | RFC3339 | `ListFilters.Since` |
| `until` | RFC3339 | `ListFilters.Until` |
| `scored` | bool | `ListFilters.Scored` |
| `cve_id` | string | `GetByCanonicalID` (point lookup) |
| `ghsa_id` | string | `GetByCanonicalID` (point lookup) |
| `edb_id` | string | `GetByCanonicalID` (point lookup) |

The presence of any canonical ID param (`cve_id`, `ghsa_id`, `edb_id`)
switches the handler to point-lookup mode; the other list filters are ignored.

**Logging inside handlers:** use `slog.ErrorContext(r.Context(), ...)` for
unexpected store errors. Include the request ID via
`middleware.RequestIDFromContext(r.Context())` as a structured field.

---

## 9. `main.go` Wiring

`main()` is a thin signal-handling wrapper. All logic lives in the testable
`run()` function:

```go
// main — registers SIGINT/SIGTERM handlers and delegates to run().
func main() {
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    if err := run(ctx, os.Args, os.Stdout, os.Stderr, nil); err != nil {
        slog.Error("Failed to start HTTP server", "error", err.Error())
        os.Exit(exitCodeError)
    }
}
```

```go
// run — testable lifecycle: parse flags → config → logger → DB → router →
// listen → serve → shutdown.
func run(ctx context.Context, args []string, stdout, stderr io.Writer,
    opts *runOpts) error
```

**`runOpts`** is a test hook struct:

```go
type runOpts struct {
    ready  chan<- net.Addr // receives listen address once serving (test sync)
    skipDB bool           // when true, skip postgres.New() (unit tests)
}
```

**Startup sequence inside `run()`:**

```go
configPath, err := parseFlags(args)          // argparse CLI (-c/--config)
cfg, err := config.Load(configPath)          // YAML + env overlay
log := slog.New(slog.NewJSONHandler(stdout, nil))

// Database (skipped when opts.skipDB is true)
pool, err := postgres.New(ctx, cfg.Database.DSN, log)
defer pool.Close()

// Store implementations (nil until postgres store package exists)
h := router.New(nil, nil, nil, cfg.API.InternalSecret, log)

srv, ln, err := startListener(cfg.API.Listen, h, log) // net.Listen("tcp", ...)
// notify tests via opts.ready
srv.Serve(ln) // in goroutine
awaitShutdown(ctx, srv, serveErr, log)                 // blocks until signal
```

**Graceful shutdown:** `awaitShutdown` blocks on `<-ctx.Done()`, then calls
`srv.Shutdown` with a 10-second timeout. The pool closes after the server
drains via `defer pool.Close()`.

No global state. All packages receive their dependencies as constructor
arguments.

---

## 10. Error Handling Summary

| Scenario | Behavior |
|----------|----------|
| Config file missing or malformed | `log.Fatalf` → non-zero exit |
| Required config field empty after env overlay | `log.Fatalf` → non-zero exit |
| DB ping fails at startup | `log.Error` + `os.Exit(1)` |
| Metrics registration fails | `log.Error` + `os.Exit(1)` |
| `pgx.ErrNoRows` in handler | 404 `not_found` |
| `PgError 23505` in handler | 409 `conflict` |
| Other store error in handler | Log at Error with request ID + 500 `internal_error` |
| Panic in handler | Recovery middleware → 500 `internal_error` |
| Context cancelled (client disconnect) | pgx propagates cancellation; handler returns without writing |

---

## 11. Import Paths

All internal packages use the module prefix `daily-patch/api/internal/`:

**Implemented:**

```
daily-patch/api/internal/config
daily-patch/api/internal/postgres
daily-patch/api/internal/store
daily-patch/api/internal/middleware
daily-patch/api/internal/handler
daily-patch/api/internal/response
daily-patch/api/internal/router
```

**Planned:**

```
daily-patch/api/internal/store/postgres
daily-patch/api/internal/metrics
```

**External dependencies in `go.mod`:**

| Module | Purpose | Status |
|--------|---------|--------|
| `github.com/akamensky/argparse` | CLI argument parsing | In use |
| `github.com/go-chi/chi/v5` | HTTP router and middleware composition | In use |
| `github.com/jackc/pgx/v5` | PostgreSQL driver and connection pool | In use |
| `github.com/golang-migrate/migrate/v4` | Embedded database migrations | In use |
| `gopkg.in/yaml.v3` | YAML config parsing | In use |
| `github.com/prometheus/client_golang` | Prometheus metrics | Planned |

---

## 12. Deferred Decisions

The following are scoped out of the current design and will be addressed in
subsequent issues:

- **Pagination** — list endpoints return all matching records in v1; cursor or
  offset pagination is deferred until query volume warrants it.
- **`POST /api/v1/runs/ingestion` request body** — blocked on issue #13; the
  body schema will mirror `IngestionRun` once the run-tracking API is
  finalized. `RecordRun` and `LastSuccessfulRun` in the ingestion service's
  API client are currently stubbed with `panic("not implemented")`.
- **Prometheus metrics** — `internal/metrics/` package and `/metrics` endpoint
  are not yet implemented. The `Metrics()` middleware is a no-op placeholder.
- **Store implementations** — `internal/store/postgres/` with concrete
  `VulnStore`, `ScoreStore`, and `RunStore` backed by pgx queries. Handlers
  currently receive `nil` stores and return 501.
- **Scoring and newsletter tables** — `scored_vulns` and `newsletter_runs`
  tables will be added in a future migration (`002_...`) when those features
  are implemented.
