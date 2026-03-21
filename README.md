# daily-patch

A self-hosted vulnerability intelligence pipeline. It ingests CVE and advisory data from
NVD, GitHub Security Advisories, and Exploit-DB; scores each finding for personal relevance
using Claude; and delivers a daily prioritized digest as a static HTML file and/or email.

- [daily-patch](#daily-patch)
  - [Architecture](#architecture)
    - [Services](#services)
  - [Project Layout](#project-layout)
  - [Prerequisites](#prerequisites)
  - [Setup](#setup)
  - [Running the Pipeline](#running-the-pipeline)
  - [Local Development](#local-development)
  - [Docker Builds](#docker-builds)
  - [Configuration Reference](#configuration-reference)
  - [Environment Variables](#environment-variables)
  - [Further Reading](#further-reading)

---

## Architecture

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
```

PostgreSQL is an internal implementation detail owned exclusively by the API service.
No other service holds a direct database connection — all reads and writes go through
the REST API. This makes the API the natural extension point for a future web UI or
additional consumers.


### Services

| Service | Runtime | Role | Lifecycle |
|---------|---------|------|-----------|
| `api` | Go | REST API server; sole DB owner | Long-lived |
| `ingestion` | Go | Fetches CVEs/advisories from sources | One-shot |
| `scorer` | Python | LLM relevance scoring via Claude | One-shot |
| `generator` | Python | Newsletter rendering + delivery | One-shot |
| `postgres` | — | Data store | Long-lived |

---

## Project Layout

```
api/            REST API server (Go)
ingestion/      Data fetcher (Go)
scorer/         LLM relevance scorer (Python)
generator/      Newsletter generator + delivery (Python)
db/migrations/  SQL migrations (golang-migrate)
output/         HTML delivery output (bind-mounted)
config.yaml     Shared configuration
.env.example    Secret template
SPEC.md         Full project specification
CLAUDE.md       Claude Code guidance
STANDARDS.md    Go + Python coding standards
ACTIONS.md      CI platform reference
```

---

## Prerequisites

- Docker + Docker Compose v2
- Go 1.23+ (for local development of Go services)
- Python 3.12+ (for local development of Python services)
- An Anthropic API key

---

## Setup

**Step 1 — Clone and configure**

```sh
cp .env.example .env
# Edit .env: set ANTHROPIC_API_KEY, DATABASE_URL, API_INTERNAL_SECRET, etc.
```

**Step 2 — Edit your interest profile**

Open `config.yaml` and update the `interests` section:

```yaml
interests:
  prose: |
    I care about Linux kernel CVEs, authentication bypass vulnerabilities,
    memory safety issues in C/C++ projects, and anything with a public PoC.
  filters:
    min_cvss: 6.0
    vendors: []      # e.g. ["microsoft", "cisco"]
    products: []     # e.g. ["openssl", "linux kernel"]
    keywords: []     # e.g. ["remote code execution", "zero-day"]
```

`prose` is sent verbatim to Claude as your interest profile. `filters` are hard pre-filters
applied before any LLM calls to reduce cost.

**Step 3 — Start the stack**

```sh
docker compose up
```

---

## Running the Pipeline

The cron scheduler is deferred to a later version. Pipeline steps are run manually:

```sh
docker compose run --rm ingestion
docker compose run --rm scorer
docker compose run --rm generator
```

Output HTML is written to `./output/daily-patch.html` (bind-mounted from the container).

---

## Local Development

```sh
# Install dev tools (golangci-lint + Python venvs)
make setup

# Build Go services
make build

# Test all services
make test

# Lint all services
make lint

# Per-service
cd api && make build test lint
cd ingestion && make build test lint
cd scorer && make test lint
cd generator && make test lint
```

---

## Docker Builds

```sh
# Build all four service images
make docker-build

# Rebuild a specific service
docker compose build {SERVICE}
```

---

## Configuration Reference

| Field | Description |
|-------|-------------|
| `interests.prose` | Natural language interest description sent to Claude |
| `interests.filters.min_cvss` | Drop findings below this CVSS score before scoring |
| `interests.filters.vendors` | Allowlist of vendors (empty = all) |
| `interests.filters.products` | Allowlist of products (empty = all) |
| `interests.filters.keywords` | Keyword allowlist (empty = all) |
| `llm.model` | Claude model used for scoring (default: `claude-opus-4-6`) |
| `delivery.html.output_path` | Where the HTML digest is written |
| `delivery.email.enabled` | Set to `true` to enable email delivery |
| `delivery.email.smtp_host` | SMTP host for email delivery |
| `delivery.email.ses_region` | Set to use AWS SES instead of SMTP |

See `SPEC.md` Section 5 for the full configuration schema.

---

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `DATABASE_URL` | PostgreSQL DSN (API service only) |
| `API_INTERNAL_SECRET` | Shared service-to-service auth header |
| `ANTHROPIC_API_KEY` | Used directly by the `anthropic` Python SDK |
| `NVD_API_KEY` | NVD API v2 credential |
| `GITHUB_TOKEN` | GitHub Security Advisories credential |
| `SMTP_PASSWORD` | Email delivery (optional) |

Secrets are defined in `.env` (gitignored) and loaded by Docker Compose. Copy
`.env.example` to get started.

---

## Further Reading

- `SPEC.md` — full architecture, data model, API endpoints, config schema
- `STANDARDS.md` — Go and Python coding conventions enforced by CI
- `CLAUDE.md` — instructions for working with Claude Code in this repo
- `ACTIONS.md` — CI/CD pipeline details
