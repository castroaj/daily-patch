# Ingestion Service — Backend Design Spec

This document is the authoritative low-level design reference for the
`ingestion` service backend. All implementation work in `ingestion/` should
conform to the interfaces, types, and conventions defined here.

---

## 1. Package Layout

**Implemented:**

```
ingestion/
  main.go                              # entry point; wires registry, runner (config wiring TBD)
  specs/
    backend.md                         # this document
  internal/
    types/
      vuln.go                          # Vulnerability, RunRecord, SourceType
    apiclient/
      client.go                        # APIClient interface + HTTP implementation
    source/
      source.go                        # Source interface
      registry.go                      # Registry struct + Register/All methods
    runner/
      runner.go                        # orchestrates sources → dedup → persist → record
```

**Planned (not yet implemented):**

```
  internal/
    sources/
      nvd/
        nvd.go                         # NVD API v2 source implementation
      ghsa/
        ghsa.go                        # GitHub Security Advisories implementation
      exploitdb/
        exploitdb.go                   # Exploit-DB CSV/RSS source implementation
```

Config loading has no dedicated package yet — see §8 for the incremental strategy.

**Rationale:**

- `internal/types` is a shared leaf package imported by all others — prevents
  circular dependencies.
- `internal/source` holds the `Source` interface and the `Registry`; source
  implementations in `internal/sources/` import it.
- `main.go` constructs the `Registry` and registers each enabled source
  explicitly — no `init()` self-registration, making wiring visible and
  testable.
- `runner` is isolated so it can be tested with mock `Source` and `APIClient`
  values.

---

## 2. Core Types (`internal/types/vuln.go`)

### SourceType

Typed string matching the `CHECK` constraint on the `vulnerabilities` and
`ingestion_runs` tables:

```go
type SourceType string

const (
    SourceNVD       SourceType = "nvd"
    SourceGHSA      SourceType = "ghsa"
    SourceExploitDB SourceType = "exploitdb"
)
```

### Vulnerability

Normalized struct passed from each source implementation to the runner.
Only fields with dedicated DB columns are promoted; everything else stays
in `RawJSON`.

```go
type Vulnerability struct {
    CVEID       string          // "CVE-2024-1234"; empty if not applicable
    GHSAID      string          // "GHSA-xxxx-xxxx-xxxx"; empty if not applicable
    EDBID       string          // "EDB-12345"; empty if not applicable
    Source      SourceType
    Title       string
    Description string
    CVSSScore   float64         // 0.0 means not available; check CVSSVector too
    CVSSVector  string          // "CVSS:3.1/AV:N/AC:L/..."
    PublishedAt time.Time       // zero if unknown
    UpdatedAt   time.Time       // zero if unknown
    RawJSON     json.RawMessage // original source payload, verbatim
}
```

`id` and `ingested_at` are absent — they are assigned by the database, not
by the ingestion service.

### RunRecord

Maps directly to a row in the `ingestion_runs` table:

```go
type RunRecord struct {
    Source       SourceType
    StartedAt    time.Time
    FinishedAt   time.Time
    ItemsFetched int  // total records returned by Fetch
    ItemsNew     int  // CreateVuln calls that succeeded (not updates)
}
```

---

## 3. Source Interface (`internal/source/source.go`)

Pagination is **internal** to each source implementation. The runner sees a
single blocking `Fetch` call that returns the full result set for the time
window. This keeps the runner simple and makes each source self-contained.

```go
// Source is implemented by each vulnerability data source.
type Source interface {
    Name() types.SourceType

    // Fetch returns all records published or modified after since.
    // A zero-value since means full backfill.
    // Pagination, retries, and rate limiting are handled internally.
    // Returns a non-nil error only if the fetch cannot proceed at all;
    // the runner logs the failure and skips this source.
    Fetch(ctx context.Context, since time.Time) ([]types.Vulnerability, error)
}
```

**Why `[]Vulnerability` not a channel:** total daily volume is bounded
(~200–500 records); a channel adds coordination complexity with no throughput
benefit at this scale.

---

## 4. Registry (`internal/source/registry.go`)

The registry is a struct with methods. `main.go` constructs a `Registry`,
registers each enabled source explicitly, and passes it to the runner. There
is no global state and no `init()` self-registration.

```go
type Registry struct {
    sources map[types.SourceType]Source
}

func NewRegistry() *Registry

// Register adds a source. Panics if a source with the same Name() is
// already registered — catches duplicate wiring mistakes at startup.
func (r *Registry) Register(s Source)

// All returns registered sources sorted by Name for deterministic ordering.
func (r *Registry) All() []Source
```

`main.go` wires sources after loading config, passing each source its own
config subtree:

```go
reg := source.NewRegistry()
if cfg.Sources.NVD.Enabled {
    reg.Register(nvd.New(cfg.Sources.NVD))
}
if cfg.Sources.GitHub.Enabled {
    reg.Register(ghsa.New(cfg.Sources.GitHub))
}
if cfg.Sources.ExploitDB.Enabled {
    reg.Register(exploitdb.New(cfg.Sources.ExploitDB))
}
```

**Why a struct over a package-level map:** explicit construction is easier to
test (each test creates its own `Registry`), eliminates global state, and
makes source wiring visible in `main.go` rather than spread across `init()`
calls in multiple packages.

---

## 5. API Client Interface (`internal/apiclient/client.go`)

```go
// APIClient abstracts all REST calls to the api service.
type APIClient interface {
    // CheckExists queries by canonical ID; exactly one of cveID, ghsaID, edbID
    // should be non-empty. Returns the API-assigned UUID if found.
    CheckExists(ctx context.Context, cveID, ghsaID, edbID string) (id string, found bool, err error)

    // CreateVuln posts a new record; returns the assigned UUID.
    CreateVuln(ctx context.Context, v types.Vulnerability) (id string, err error)

    // UpdateVuln replaces an existing record by its API UUID.
    UpdateVuln(ctx context.Context, id string, v types.Vulnerability) error

    // RecordRun posts a completed ingestion run to POST /api/v1/runs/ingestion.
    RecordRun(ctx context.Context, r types.RunRecord) error

    // LastSuccessfulRun returns finished_at for the most recent completed run
    // for the given source, or a zero time.Time if none exists.
    LastSuccessfulRun(ctx context.Context, source types.SourceType) (time.Time, error)
}
```

**Implementation:** unexported `httpClient` struct holding `baseURL`,
`secret`, and `*http.Client`. Constructor:

```go
func New(baseURL, secret string, timeout time.Duration) (APIClient, error)
```

Returns an error if `baseURL` is not a valid absolute HTTP/HTTPS URL (no
trailing slash) or if `secret` contains non-printable or whitespace
characters. Pass `DefaultTimeout` (30 s) when no custom value is needed.

`RecordRun` and `LastSuccessfulRun` are currently stubbed — both panic with
`"not implemented"`. They will be filled in once the run-tracking API
endpoints are designed. See issue #7.

Transient errors (network, 5xx) retry up to 3× with exponential backoff.

**Response envelope:** the API service wraps every response in a standard
envelope (see SPEC.md §3b). The `do` helper decodes this envelope on 2xx
responses and unmarshals the inner `result` field into `dst`. Non-2xx
responses (404, 400, 409) are returned to caller methods unchanged — they
inspect `resp.StatusCode` directly to decide behaviour.

```json
{ "error": "", "errorDetail": "", "statusCode": 200, "result": { ... } }
```

**REST endpoints used** (from SPEC.md §3b):

| Method | Path | Used by |
|--------|------|---------|
| `GET` | `/api/v1/vulns?cve_id=` | `CheckExists` |
| `POST` | `/api/v1/vulns` | `CreateVuln` |
| `PUT` | `/api/v1/vulns/{id}` | `UpdateVuln` |
| `GET` | `/api/v1/runs/ingestion` | `LastSuccessfulRun` |
| `POST` | `/api/v1/runs/ingestion` | `RecordRun` |

---

## 6. Runner (`internal/runner/runner.go`)

```go
type Runner struct {
    client  apiclient.APIClient
    sources []source.Source
}

func New(client apiclient.APIClient, sources []source.Source) *Runner

// Run executes sources sequentially. Source failures are logged and
// accumulated; Run returns a joined non-nil error if any source fails.
func (r *Runner) Run(ctx context.Context) error
```

`main.go` passes `reg.All()` to `runner.New`. Sources are already filtered to
enabled-only by the conditional `reg.Register` calls in `main.go`.

**Per-source loop (pseudocode):**

```
startedAt = time.Now()
since     = client.LastSuccessfulRun(ctx, source.Name())
vulns     = source.Fetch(ctx, since)           // on error: log, record failed run, continue

for each vuln:
    id, found = client.CheckExists(cveID, ghsaID, edbID)  // on error: log + skip record
    if found:
        client.UpdateVuln(id, vuln)
    else:
        client.CreateVuln(vuln)
        itemsNew++

client.RecordRun(ctx, RunRecord{
    Source:       source.Name(),
    StartedAt:    startedAt,
    FinishedAt:   time.Now(),
    ItemsFetched: len(vulns),
    ItemsNew:     itemsNew,
})
```

The dedup loop is sequential within each source (concurrent dedup risks a
race when the same CVE-ID appears twice in a batch). Concurrency lives inside
`Fetch`.

---

## 7. Concurrency Model

Concurrency is **source-internal**, not runner-level.

### NVD

After page 1 returns `totalResults`, remaining pages can be dispatched
concurrently using computed `startIndex` offsets. Uses a semaphore-based
goroutine pool. Default `poolSize: 5`.

### GHSA

GraphQL cursor pagination — cursors are opaque and sequential. `poolSize`
defaults to `1` (effectively sequential).

### Exploit-DB

Single CSV/RSS download. No internal parallelism.

Pool size is read from each source's config subtree.

---

## 8. Configuration

Config loading is implemented **incrementally** — each source type parses its
own section of `config.yaml` when it is built, rather than loading the entire
config structure upfront. This avoids speculating on config fields before the
sources that need them exist.

There is no `internal/config` package yet. The eventual design is:

- `main.go` reads the raw YAML file once and passes each source its own
  config subtree.
- Secrets (`NVD_API_KEY`, `GITHUB_TOKEN`, `API_INTERNAL_SECRET`) are read
  from environment variables and overlaid after the file is parsed.
- The service exits with a descriptive error if `config.yaml` is missing,
  malformed, or a required field is absent.

The config structs below are the **intended target** for when sources are
added — not the current state:

```go
type NVDConfig struct {
    Enabled  bool
    APIKey   string // from env: NVD_API_KEY
    PoolSize int    // default 5
}

type GitHubConfig struct {
    Enabled  bool
    Token    string // from env: GITHUB_TOKEN
    PoolSize int    // default 1
}

type ExploitDBConfig struct {
    Enabled bool
    FeedURL string
}

type APIConfig struct {
    BaseURL        string // e.g. "http://api:8080"
    InternalSecret string // from env: API_INTERNAL_SECRET
}
```

---

## 9. Error Handling

| Scenario | Behavior |
|----------|----------|
| Source `Fetch` fails | Log, record run with zero counts, `continue` to next source |
| Per-record API failure | Log + skip record; run counter reflects successful records only |
| Any source fails | `Runner.Run` returns `errors.Join(errs...)` → `main.go` calls `log.Fatalf` → non-zero exit |
| API transient error (network, 5xx) | `httpClient` retries up to 3× with exponential backoff |
| Context cancellation | All operations check `ctx.Done()`; partial runs are not recorded |

---

## 10. Import Paths

All internal packages use the module prefix `daily-patch/ingestion/internal/`:

**Implemented:**

```
daily-patch/ingestion/internal/types
daily-patch/ingestion/internal/apiclient
daily-patch/ingestion/internal/source
daily-patch/ingestion/internal/runner
```

**Planned:**

```
daily-patch/ingestion/internal/sources/nvd
daily-patch/ingestion/internal/sources/ghsa
daily-patch/ingestion/internal/sources/exploitdb
```
