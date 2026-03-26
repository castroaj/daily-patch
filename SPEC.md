# daily-patch — Project Specification

## 1. Overview

**daily-patch** is a self-hosted vulnerability intelligence pipeline that ingests CVE/advisory data from multiple sources, scores findings for personal relevance using an LLM, and delivers a daily digest as a static HTML file and/or email.

### Goals

- Aggregate vulnerability data from NVD, GitHub Security Advisories, and Exploit-DB into a single normalized store
- Score each finding against a user-defined interest profile (prose + structured filters) using the Claude API
- Generate a concise, prioritized daily newsletter and deliver it via HTML file and/or email
- Expose all data through a clean REST API to serve as the foundation for future consumers (web UI, alerting, etc.)

### Non-Goals (v1)

See [Section 9](#9-non-goals-v1).

---

## 2. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         Cron Scheduler                          │
└────────┬───────────────────┬───────────────────┬───────────────┘
         │ trigger            │ trigger            │ trigger
         ▼                   ▼                   ▼
┌────────────────┐   ┌────────────────┐   ┌──────────────────────┐
│  Ingestion Svc │   │  Scoring Svc   │   │   Newsletter Gen     │
│    (Go)        │   │   (Python)     │   │     (Python)         │
└───────┬────────┘   └───────┬────────┘   └──────────┬───────────┘
        │ write                │ GET + POST             │ GET
        ▼                     ▼                        ▼
┌───────────────────────────────────────────────────────────────┐
│                    REST API Service (Go)                      │
└───────────────────────────┬───────────────────────────────────┘
                            │ read/write
                            ▼
                   ┌─────────────────┐
                   │   PostgreSQL    │
                   └─────────────────┘

Delivery outputs:
  Newsletter Gen ──▶  Static HTML file
                 ──▶  Email (SMTP / SES)

Future consumers:
  Web UI ──▶ REST API Service
```

### Runtime Split

The project uses two runtimes chosen for their strengths:

- **Go** — used for the ingestion service and REST API server. Ideal for high-throughput I/O, concurrent HTTP fan-out across sources, and operating a production-grade HTTP server with low overhead.
- **Python** — used for the scoring service and newsletter generator. Ideal for LLM/NLP workloads, where the Anthropic SDK, Jinja2 templating, and quick iteration matter more than raw throughput.

### API-Centric Design

The REST API is the **single source of truth interface** for all consumers. PostgreSQL is an internal implementation detail owned exclusively by the API service. No other service holds a direct database connection. All reads and writes — including those performed by the ingestion, scoring, and newsletter services — go through the REST API. This enforces a clean separation of concerns and makes the API the natural extension point for a future web UI or additional consumers.

---

## 3. Components

### 3a. Ingestion Service (Go)

**Responsibilities:** fetch vulnerability data from configured sources, normalize it into a common schema, deduplicate against existing records, and persist new/updated records via the REST API.

**Concurrency model:** a goroutine pool is spun up per source with configurable parallelism. Each pool handles pagination concurrently within its source; sources themselves run sequentially or in parallel based on configuration.

**Sources:**

| Source | Protocol | Incremental Strategy | ID Key |
|--------|----------|----------------------|--------|
| NVD API v2 | Paginated REST | `lastModStartDate` query param | CVE-ID |
| GitHub Security Advisories | GraphQL cursor pagination | `updatedAt` cursor | GHSA-ID |
| Exploit-DB | CSV dump or RSS feed | EDB-ID comparison | EDB-ID |

**Deduplication:** before inserting, the ingestion service checks whether a record with the same canonical ID already exists by calling the REST API (`GET /api/v1/vulns?cve_id=...` or equivalent). The endpoint returns a single object or 404 — not a list — because canonical IDs are `UNIQUE` in the database. If found, it issues a `PUT` to update; otherwise a `POST` to create.

**Run recording:** each completed ingestion run (start time, finish time, items fetched, items new) is recorded via the API at the end of each source run.

---

### 3b. REST API Service (Go)

**Framework:** chi or gin (to be decided at implementation time).

The API service is the sole owner of the PostgreSQL connection pool. It exposes all data via a versioned REST interface under `/api/v1/`.

**Authentication:** service-to-service calls use a shared internal secret passed via a request header (`X-Internal-Secret`). The design supports upgrading to JWT-based auth for multi-tenant use without breaking existing service integrations.

**Response envelope:**

Every endpoint returns a consistent JSON envelope regardless of success or failure:

```json
{
  "error":       "",
  "errorDetail": "",
  "statusCode":  201,
  "result":      { ... }
}
```

- `error` and `errorDetail` are empty strings on success; they carry a short error code and human-readable detail on failure.
- `result` contains the response payload on success and is `null` on error responses.
- `statusCode` always mirrors the HTTP response status code.

All handlers write this envelope via the shared `api/internal/response` package (`response.Write`).

**Endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/vulns` | List/filter vulnerabilities. Query params: `source`, `min_cvss`, `max_cvss`, `since`, `until`, `scored` |
| `GET` | `/api/v1/vulns?cve_id={id}` | Look up a vulnerability by canonical ID. Also accepts `ghsa_id` or `edb_id`. Returns a single object or 404. Because `cve_id`, `ghsa_id`, and `edb_id` are `UNIQUE` in the database, these params are point lookups, not list filters — the response is a single vulnerability object, not a list. |
| `GET` | `/api/v1/vulns/{id}` | Single vulnerability detail |
| `POST` | `/api/v1/vulns` | Ingest a new vulnerability record (used by ingestion service) |
| `PUT` | `/api/v1/vulns/{id}` | Update an existing vulnerability record |
| `GET` | `/api/v1/vulns/{id}/scores` | Retrieve scoring results for a vulnerability |
| `POST` | `/api/v1/vulns/{id}/scores` | Submit a scoring result (used by scoring service) |
| `GET` | `/api/v1/runs/ingestion` | List ingestion run logs |
| `GET` | `/api/v1/runs/newsletter` | List newsletter run logs |
| `POST` | `/api/v1/runs/newsletter` | Record a completed newsletter run |

---

### 3c. Relevance Scoring Service (Python)

**Workflow:**

1. Calls `GET /api/v1/vulns?scored=false` to retrieve all unscored vulnerabilities
2. Loads the interest profile from `config.yaml` (prose description + structured filters)
3. Applies hard pre-filters to reduce LLM calls:
   - Drop records below `interests.filters.min_cvss`
   - Filter to matching vendors, products, CWEs, or keywords if allowlists are non-empty
4. For each remaining record, calls the Claude API with a structured prompt containing the interest profile and vulnerability metadata
5. POSTs the resulting score (0–100) and rationale back via `POST /api/v1/vulns/{id}/scores`

**LLM integration:** uses the `anthropic` Python SDK. The prompt is structured to return a JSON object with `score` and `rationale` fields for reliable parsing.

---

### 3d. Newsletter Generator (Python)

**Workflow:**

1. Calls `GET /api/v1/vulns?scored=true&min_score=X&since=<last_run>` to fetch relevant, recently scored vulnerabilities
2. Calls Claude API to generate an executive summary paragraph across all findings
3. Renders a Jinja2 HTML template and a plain-text fallback

**Newsletter sections:**

- Executive Summary (LLM-generated)
- Critical / High Severity (CVSS ≥ 9.0 / 7.0)
- Notable PoCs (findings sourced from Exploit-DB or with known PoC references)
- Full List (all findings above the score threshold, sorted by score descending)

4. POSTs the completed run record to `POST /api/v1/runs/newsletter`

---

### 3e. Delivery Layer (Python — part of Newsletter Generator)

**Static HTML:** writes the rendered HTML to the path configured in `delivery.html.output_path`. This path is bind-mounted in Docker Compose for host access.

**Email:** sends a multipart MIME message (HTML + plain-text) using either:
- **SMTP** — configurable host, port, credentials
- **AWS SES** — via the `boto3` SDK when `delivery.email.ses_region` is set

Email delivery is gated by `delivery.email.enabled` and only runs when at least one `to_addresses` entry is configured.

---

## 4. Data Model

All tables are managed via versioned migrations using `golang-migrate`. The migration files live in `db/migrations/` and are embedded in the API service binary.

```sql
-- Canonical vulnerability records
CREATE TABLE vulnerabilities (
  id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  source           TEXT        NOT NULL CHECK (source IN ('nvd', 'ghsa', 'exploitdb')),
  cve_id           TEXT        UNIQUE,
  ghsa_id          TEXT        UNIQUE,
  edb_id           TEXT        UNIQUE,
  title            TEXT        NOT NULL,
  description      TEXT,
  cvss_score       NUMERIC(4,1),
  cvss_vector      TEXT,
  published_at     TIMESTAMPTZ,
  updated_at       TIMESTAMPTZ,
  raw_json         JSONB,
  ingested_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- LLM-generated relevance scores
CREATE TABLE scored_vulns (
  id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  vuln_id           UUID        NOT NULL REFERENCES vulnerabilities(id),
  score             INTEGER     NOT NULL CHECK (score BETWEEN 0 AND 100),
  rationale         TEXT,
  scored_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  newsletter_run_id UUID        REFERENCES newsletter_runs(id)
);

-- Ingestion pipeline run logs
CREATE TABLE ingestion_runs (
  id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  source         TEXT        NOT NULL CHECK (source IN ('nvd', 'ghsa', 'exploitdb')),
  started_at     TIMESTAMPTZ NOT NULL,
  finished_at    TIMESTAMPTZ,
  items_fetched  INTEGER     NOT NULL DEFAULT 0,
  items_new      INTEGER     NOT NULL DEFAULT 0
);

-- Newsletter generation run logs
CREATE TABLE newsletter_runs (
  id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  run_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  item_count       INTEGER     NOT NULL DEFAULT 0,
  delivery_targets JSONB
);
```

**Notes:**
- `cve_id`, `ghsa_id`, and `edb_id` are nullable because not every record has all identifiers (e.g. an Exploit-DB entry may reference a CVE, but a standalone PoC may not)
- `cve_id`, `ghsa_id`, and `edb_id` carry `UNIQUE` constraints. This is a hard requirement, not a convenience index. The ingestion service deduplicates by querying `GET /api/v1/vulns?cve_id=...` (or equivalent) and relies on the guarantee that at most one record can match any canonical ID. Without these constraints a single real-world vulnerability ingested from multiple sources (e.g. NVD and GitHub Advisories) could produce duplicate records, causing it to be scored and delivered multiple times. **Never remove these constraints.**
- `raw_json` stores the original source payload for auditability and future re-processing
- `newsletter_run_id` on `scored_vulns` is set when a score is included in a newsletter run, enabling per-run reporting

---

## 5. Configuration Schema

The project is configured via a single `config.yaml` file mounted into all containers. Secrets are never stored in the config file directly — they are referenced via environment variables and injected at runtime (via `.env` in Docker Compose).

```yaml
schedule:
  ingestion: "0 5 * * *"       # cron expression (runs at 05:00 daily)
  newsletter: "0 6 * * *"      # cron expression (runs at 06:00 daily)

sources:
  nvd:
    enabled: true
    api_key: ""                 # from env: NVD_API_KEY
  github:
    enabled: true
    token: ""                   # from env: GITHUB_TOKEN
  exploitdb:
    enabled: true
    feed_url: ""                # CSV dump URL or RSS feed URL

database:
  dsn: ""                       # from env: DATABASE_URL

api:
  listen: ":8080"
  internal_secret: ""           # from env: API_INTERNAL_SECRET

interests:
  prose: |
    I care about Linux kernel CVEs, authentication bypass vulnerabilities,
    memory safety issues in C/C++ projects, and anything with a public PoC.
  filters:
    min_cvss: 6.0
    vendors: []                 # e.g. ["microsoft", "cisco"]
    products: []                # e.g. ["openssl", "linux kernel"]
    cwes: []                    # e.g. ["CWE-79", "CWE-89"]
    keywords: []                # e.g. ["remote code execution", "zero-day"]

llm:
  provider: anthropic
  model: claude-opus-4-6
  max_tokens: 1024

delivery:
  html:
    output_path: "/output/daily-patch.html"
  email:
    enabled: false
    smtp_host: ""
    smtp_port: 587
    from_address: ""
    to_addresses: []
    ses_region: ""              # set to use SES instead of SMTP (e.g. "us-east-1")
```

**Secret injection pattern:**

```
NVD_API_KEY        → sources.nvd.api_key
GITHUB_TOKEN       → sources.github.token
DATABASE_URL       → database.dsn
API_INTERNAL_SECRET → api.internal_secret
ANTHROPIC_API_KEY  → consumed directly by the anthropic SDK
SMTP_PASSWORD      → consumed directly by the delivery layer
```

Secrets are defined in a `.env` file (gitignored) and loaded by Docker Compose.

---

## 6. Deployment

The project ships as a Docker Compose stack. All services share `config.yaml` via a bind-mounted volume.

```
services:
  postgres      — PostgreSQL 16; named volume for data persistence
  api           — Go REST API server (sole owner of DB connection)
  ingestion     — Go ingestion binary; one-shot, exits when done
  scorer        — Python scoring service; one-shot, exits when done
  generator     — Python newsletter generator + delivery; one-shot, exits when done
  scheduler     — Ofelia (Docker-native cron); triggers pipeline steps in sequence
```

**Volume layout:**

| Volume / Bind | Purpose |
|---------------|---------|
| `postgres_data` (named) | PostgreSQL data persistence |
| `./config.yaml` → `/app/config.yaml` | Shared config for all services |
| `./output` → `/output` | HTML delivery output, accessible on the host |

**Environment variables** are sourced from `.env` at the project root. The `.env` file is gitignored and must be created manually from `.env.example`.

---

## 7. Scheduling

Pipeline scheduling is managed by an **Ofelia** container — a lightweight, Docker-native cron scheduler that triggers jobs by running Docker containers directly.

**Pipeline execution order** (enforced by Ofelia job dependencies):

```
05:00  ingestion  ──▶  completes
                         │
                         ▼
                       scorer  ──▶  completes
                                      │
                                      ▼
                                    generator  ──▶  delivers HTML + email
06:00
```

Each step is a **one-shot container invocation** — the service binary runs, completes its work, and exits. There are no long-running daemon processes for ingestion, scoring, or generation.

The `api` service is the only long-running process in the stack (aside from `postgres` and `scheduler`).

Cron expressions are defined in `config.yaml` under `schedule.*` and referenced by the Ofelia configuration, keeping schedule management in one place.

---

## 8. File Header Standard

Every source file begins with a header comment using the file's native comment character:

```
<comment> <filename> — <one-line description>
<comment>
<comment> <optional additional context>
```

| File type | Comment character |
|-----------|-------------------|
| Go | `//` |
| Python | `#` |
| Makefile | `#` |
| Dockerfile | `#` |
| YAML | `#` |

See `CLAUDE.md` for full examples and guidance.

---

## 9. Non-Goals (v1)

The following are explicitly out of scope for the initial version:

- **Multi-tenant authentication** — the API is designed to support JWTs and per-user interest profiles later, but v1 uses a single shared secret for internal service auth only
- **Web UI** — the REST API is the foundation for a future web UI, but no frontend is included in v1
- **Vector / semantic search** — deduplication and matching are ID-based and filter-based; no embeddings or similarity search
- **Real-time alerting** — delivery is batch/scheduled only; no push notifications or webhooks
- **Discord / Slack delivery** — only HTML file and email are supported in v1

---

## 9. Future Considerations

| Feature | Notes |
|---------|-------|
| Multi-tenant web UI | Built on top of the existing REST API; requires JWT auth |
| JWT-based API auth | Replace shared secret with per-user JWTs; API is already structured for this |
| Vector embeddings | Semantic deduplication and similarity-based interest matching |
| Discord / Slack delivery | Additional delivery targets in the newsletter generator |
| Tag-based subscriptions | Allow multiple interest profiles with different delivery targets |
| Additional vendor sources | Microsoft Patch Tuesday, Cisco PSIRT, Red Hat Security Advisories |
| Re-scoring | Allow manual or scheduled re-scoring of previously scored findings |
