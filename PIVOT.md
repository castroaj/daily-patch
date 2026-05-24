# Risk Graph Pivot Plan

## What This Is

A pivot of daily-patch from a personal vulnerability digest (LLM-scored CVEs → HTML newsletter)
to a **CI/CD risk graph API**. Instead of running expensive LLM inference during a developer's
workflow, the system pre-computes a living risk graph asynchronously and exposes a
sub-millisecond lookup endpoint for CI pipelines.

## New Pipeline Order

```
02:00  ingestion     — fetches CVE records (NVD, GHSA, ExploitDB) — unchanged
03:00  signals       — new: fetches enrichment data (CISA KEV, EPSS, GitHub PoC velocity)
04:00  graph-builder — new: computes composite risk scores deterministically
05:00  generator     — pivoted: renders risk graph health dashboard HTML (no LLM)
```

## Key Design Decisions

- `GET /api/v1/risk` is **public** (no auth) — CI pipelines across teams can call it freely
- Signal sources run as a **new separate `signals` binary** — follows the one-concern-per-binary pattern
- `scorer/` is **removed entirely** — replaced by deterministic `graph-builder` Go service (no LLM)
- All existing patterns preserved: Source interface, Registry, Runner, APIClient, response envelope

---

## Phase 1 — Database Schema

Single baseline migration: `db/migrations/001_baseline_schema.up.sql`

### New Tables

**`kev_entries`** — CISA Known Exploited Vulnerabilities catalog
```sql
id UUID PK, cve_id TEXT UNIQUE NOT NULL, vendor TEXT, product TEXT,
vuln_name TEXT, date_added DATE NOT NULL, due_date DATE, notes TEXT, ingested_at TIMESTAMPTZ
```

**`epss_scores`** — FIRST.org EPSS probability scores per CVE
```sql
id UUID PK, cve_id TEXT NOT NULL, score NUMERIC(6,5), percentile NUMERIC(6,5),
model_version TEXT, scored_date DATE,
UNIQUE(cve_id, scored_date, model_version)
```
Indexes: `(cve_id)`, `(cve_id, scored_date DESC)` for fast "latest score" lookups.

**`poc_signals`** — GitHub PoC commit velocity, aggregated per CVE per ISO week
```sql
id UUID PK, cve_id TEXT NOT NULL, week_start DATE NOT NULL,
commit_count INT, repo_count INT,
UNIQUE(cve_id, week_start)
```

**`packages`** — normalized package names per ecosystem
```sql
id UUID PK, ecosystem TEXT CHECK IN ('npm','pip','cargo'), name TEXT,
UNIQUE(ecosystem, name)
```

**`package_vulns`** — OSV.dev mapping of CVEs to package version ranges
```sql
id UUID PK, package_id UUID FK packages, cve_id TEXT, ghsa_id TEXT,
versions_affected JSONB DEFAULT '[]', source_osv_id TEXT
```

**`risk_scores`** — pre-computed composite risk (the CI lookup hot path)
```sql
id UUID PK, entity_type TEXT CHECK IN ('cve','package'), entity_id TEXT,
composite_score INT, component_cvss INT, component_kev INT,
component_epss INT, component_poc INT, computed_at TIMESTAMPTZ,
UNIQUE(entity_type, entity_id)
```
Critical index: `(entity_type, entity_id)` — single index scan per CI request.

**`graph_build_runs`** — log per graph-builder execution
```sql
id UUID PK, started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ,
cves_scored INT, packages_scored INT, error_detail TEXT
```

**`dashboard_runs`** — replaces `newsletter_runs`
```sql
id UUID PK, run_at TIMESTAMPTZ, item_count INT, output_path TEXT
```

### Modified Tables

**`vulnerabilities`** — unchanged from current SPEC.md design

**`ingestion_runs`** — `source` column CHECK constraint relaxed (removed) to accommodate
`'kev'`, `'epss'`, `'poc'` values from the signal runner

---

## Phase 2 — Types and Signal Interfaces

### `ingestion/internal/types/vuln.go` (extend, do not replace)

Add constants:
```go
SourceKEV  SourceType = "kev"
SourceEPSS SourceType = "epss"
SourcePoC  SourceType = "poc"
```

Add types (same file, same leaf-package pattern):
```go
type KEVEntry struct {
    CVEID, Vendor, Product, VulnName string
    DateAdded, DueDate time.Time
    Notes   string
    RawJSON json.RawMessage
}

type EPSSScore struct {
    CVEID, ModelVersion string
    Score, Percentile   float64  // 0.0–1.0
    ScoredDate          time.Time
}

type PoCSignal struct {
    CVEID       string
    WeekStart   time.Time
    CommitCount int
    RepoCount   int
}
```

### New: `signals/internal/signal/signal.go`

Three typed source interfaces (separate per type, no generics):
```go
type KEVSource  interface { Name() types.SourceType; Fetch(ctx, since) ([]types.KEVEntry, error) }
type EPSSSource interface { Name() types.SourceType; Fetch(ctx, since) ([]types.EPSSScore, error) }
type PoCSource  interface { Name() types.SourceType; Fetch(ctx, since) ([]types.PoCSignal, error) }
```

---

## Phase 3 — Signal Service (`signals/`)

Mirrors `ingestion/` structure. New top-level `go.mod`.

### Sources

**`signals/internal/sources/kev/kev.go`**
- Downloads full CISA KEV JSON dump (no pagination)
- Filters client-side by `DateAdded >= since`
- Upstream: `https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json`

**`signals/internal/sources/epss/epss.go`**
- Paginates FIRST.org EPSS API (`?offset=N&limit=N`) — ~220k records
- EPSS values are floats encoded as strings; use `strconv.ParseFloat`
- Upstream: `https://api.first.org/data/1.0/epss`

**`signals/internal/sources/poc/poc.go`**
- Queries GitHub REST: `GET /repos/nomi-sec/PoC-in-GitHub/commits?since=...` (paginated)
- Extracts CVE IDs from commit paths (e.g. `2024/CVE-2024-1234/README.md`)
- Aggregates commit_count + repo_count per CVE per ISO week
- Reuses `GITHUB_TOKEN` env var — no new credential

### `signals/internal/apiclient/client.go`

Same pattern as `ingestion/internal/apiclient/client.go`. New path constants and methods:
```go
const (
    pathSignalsKEV  = "/api/v1/signals/kev"
    pathSignalsEPSS = "/api/v1/signals/epss"
    pathSignalsPoc  = "/api/v1/signals/poc"
    pathRunsSignal  = "/api/v1/runs/signal"
)

UpsertKEVEntries(ctx, []types.KEVEntry) error
UpsertEPSSScores(ctx, []types.EPSSScore) error
UpsertPoCSignals(ctx, []types.PoCSignal) error
RecordRun(ctx, types.RunRecord) error
LastSuccessfulRun(ctx, source types.SourceType) (time.Time, error)
```

### `signals/internal/runner/runner.go`

Mirrors `ingestion/internal/runner/runner.go`. Runs KEV → EPSS → PoC sequentially.
Uses `errors.Join` for multi-source error aggregation.

---

## Phase 4 — API Service Expansion (`api/`)

### New: `api/internal/store/store.go`

All SQL lives here. `Store` wraps `*sql.DB`. No SQL appears outside this package.

### New: `api/internal/router/router.go`

Mounts all routes on `*http.ServeMux`. Receives injected `*store.Store`. Separates routing from `main.go`.

### New: `api/internal/handlers/` (one file per group)

**Signal ingestion — internal, `X-Internal-Secret` required:**
```
POST /api/v1/signals/kev    upsert KEV entries  (ON CONFLICT cve_id DO UPDATE)
POST /api/v1/signals/epss   upsert EPSS scores  (ON CONFLICT cve_id, scored_date, model_version)
POST /api/v1/signals/poc    upsert PoC signals  (ON CONFLICT cve_id, week_start)
```

**Risk lookup — public, no auth:**
```
GET /api/v1/risk?cve=CVE-2024-1234
GET /api/v1/risk?pkg=lodash&version=4.17.20&ecosystem=npm
GET /api/v1/risk?min_score=70&limit=50      (for dashboard generator)
```
Returns `riskLookupResult` (named type): `composite_score`, `component_*`, `computed_at`.
Lookup by `pkg`: joins `packages` → `package_vulns` → `risk_scores`. Pure DB scan, no inference.

**Risk write — internal:**
```
PUT /api/v1/risk/{entity_type}/{entity_id}   upsert pre-computed score
```

**Package mapping — internal:**
```
POST /api/v1/packages/map           OSV.dev lookup → upsert packages + package_vulns
GET  /api/v1/packages/{id}/vulns    retrieve package_vuln IDs for graph-builder
```

**Run logs (extended):**
```
POST /api/v1/runs/signal      signal runner records runs
GET  /api/v1/runs/signal      last successful run per source
POST /api/v1/runs/graph       graph-builder records build runs
GET  /api/v1/runs/graph       dashboard generator reads build history
POST /api/v1/runs/dashboard   dashboard generator records completed runs
```

All existing `/api/v1/vulns/*` endpoints retained — ingestion still uses them.

---

## Phase 5 — graph-builder Service (`graph-builder/`)

New top-level directory. New `go.mod`. Go (deterministic rules, no LLM).

### `graph-builder/internal/scorer/scorer.go`

Pure function — no I/O:
```
component_cvss  = int(cvss / 10.0 * 30)           0–30
component_kev   = 50 if in KEV, else 0             0 or 50
component_epss  = int(epss_percentile * 30)        0–30
component_poc   = tiered by commits/week:
                    0 → 0,  1–4 → 5,  5–19 → 10,  20–99 → 15,  ≥100 → 20
composite       = min(sum, 100)
```
Weights can sum to 130; the cap enforces 100. Table-driven tests cover all boundary cases.

### `graph-builder/internal/osvmapper/mapper.go`

Calls OSV.dev: `POST https://api.osv.dev/v1/query` with `{"query":{"id":"CVE-..."}}`.
Returns affected packages with ecosystem, name, and version ranges.

### `graph-builder/main.go` — main loop

1. `POST /api/v1/runs/graph` (start)
2. Collect union of CVE IDs: all KEV + EPSS (percentile > 0.3) + PoC signals
3. For each CVE: gather signals → `scorer.Score` → `PUT /api/v1/risk/cve/{cve_id}`
4. For CVEs with composite ≥ 30: `POST /api/v1/packages/map` per ecosystem (npm, pip, cargo)
5. For each `package_vuln`: `PUT /api/v1/risk/package/{id}` with same score
6. `PATCH /api/v1/runs/graph/{id}` (finish with counts)

---

## Phase 6 — Generator Pivot + Remove Scorer

**`scorer/`** — deleted entirely (Dockerfile, Makefile, Python source, requirements.txt).

**`generator/`** — repurposed for Risk Graph Health Dashboard. No LLM calls. Remove `ANTHROPIC_API_KEY` from env.

New modules:
- `generator/generator/api_client.py` — HTTP client for API reads
- `generator/generator/renderer.py` — Jinja2 template rendering

Dashboard sections:
1. New KEV additions since last run
2. EPSS spikes (percentile up > 0.1 week-over-week)
3. Newly high-risk packages (composite ≥ 70)
4. Top 10 by composite score

Output: `/output/risk-graph.html`

---

## Phase 7 — Config and Docker Compose

### `config.yaml` additions

```yaml
schedule:
  ingestion:     "0 2 * * *"
  signals:       "0 3 * * *"
  graph_builder: "0 4 * * *"
  dashboard:     "0 5 * * *"

signals:
  kev:
    enabled: true
    api_url: "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"
  epss:
    enabled: true
    api_url: "https://api.first.org/data/1.0/epss"
  poc:
    enabled: true
    repo: "nomi-sec/PoC-in-GitHub"
    lookback_days: 7

graph_builder:
  osv_api_url: "https://api.osv.dev/v1"
  ecosystems: [npm, pip, cargo]
  min_score_for_package_mapping: 30
```

### `docker-compose.yml` changes

- Remove `scorer` service
- Add `signals` service (`restart: "no"`, `GITHUB_TOKEN` env)
- Add `graph-builder` service (`restart: "no"`, `API_INTERNAL_SECRET` env)
- Remove `ANTHROPIC_API_KEY` from `generator` env

---

## File Summary

| Action | Path |
|--------|------|
| Create | `db/migrations/001_baseline_schema.up.sql` |
| Create | `db/migrations/001_baseline_schema.down.sql` |
| Modify | `ingestion/internal/types/vuln.go` |
| Create | `signals/` (full service) |
| Create | `graph-builder/` (full service) |
| Create | `api/internal/store/store.go` |
| Create | `api/internal/router/router.go` |
| Create | `api/internal/handlers/` (signals, risk, packages, runs) |
| Modify | `api/main.go` |
| Modify | `generator/generator/__main__.py` |
| Create | `generator/generator/api_client.py` |
| Create | `generator/generator/renderer.py` |
| Modify | `config.yaml` |
| Modify | `docker-compose.yml` |
| Delete | `scorer/` (entire directory) |

**Unchanged:** `ingestion/internal/source/`, `ingestion/internal/runner/`, `ingestion/internal/apiclient/`, `api/internal/response/`

---

## Verification

```sh
# 1. Migration round-trip
migrate -path db/migrations -database "$DATABASE_URL" up
migrate -path db/migrations -database "$DATABASE_URL" down 1
migrate -path db/migrations -database "$DATABASE_URL" up

# 2. Unit tests
go test ./...                     # signals + graph-builder scorer
pytest                            # generator

# 3. API smoke test (after docker compose up postgres api)
curl -X POST http://localhost:8080/api/v1/signals/kev \
  -H "X-Internal-Secret: $SECRET" -H "Content-Type: application/json" \
  -d '[{"cve_id":"CVE-2024-1234","vendor":"Acme","product":"Widget","vuln_name":"Test","date_added":"2024-01-01"}]'

curl "http://localhost:8080/api/v1/risk?cve=CVE-2024-1234"   # 404 until graph-builder runs

# 4. Full pipeline
docker compose run --rm ingestion
docker compose run --rm signals
docker compose run --rm graph-builder
docker compose run --rm generator

# 5. CI latency check (should average < 10ms)
for i in $(seq 1 100); do
  curl -w "%{time_total}\n" -o /dev/null -s \
    "http://localhost:8080/api/v1/risk?cve=CVE-2024-1234"
done | awk '{s+=$1;n++} END {print "avg:", s/n*1000, "ms"}'
```
