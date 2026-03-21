# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**daily-patch** is a vulnerability intelligence pipeline. It ingests CVE/advisory data, scores findings for relevance using Claude, and delivers a daily HTML/email digest. See `SPEC.md` for the full specification.

## Architecture

The system is split across two runtimes and five services:

- **Go** — `api/` (REST API server) and `ingestion/` (data fetcher). Go is used for I/O-heavy services.
- **Python** — `scorer/` (LLM scoring) and `generator/` (newsletter + delivery). Python is used for LLM/templating workloads.
- **PostgreSQL** — accessed **only** by the API service. All other services read and write through the REST API.

### Key architectural constraint

No service other than the API server holds a database connection. The scoring service and newsletter generator interact with data exclusively via REST calls to the API service.

### Pipeline execution order

The cron scheduler (Ofelia) runs steps sequentially: `ingestion` → `scorer` → `generator`. Each step is a one-shot container that exits when done. Only the `api` service runs as a long-lived process.

## Development Commands

> These commands will be fleshed out as the services are built. Placeholders reflect the intended structure.

### Go services (`api/`, `ingestion/`)

```sh
# Build
go build ./...

# Test
go test ./...

# Run a single test
go test ./path/to/package -run TestName

# Lint
golangci-lint run

# Database migrations (golang-migrate)
migrate -path db/migrations -database "$DATABASE_URL" up
migrate -path db/migrations -database "$DATABASE_URL" down 1
```

### Python services (`scorer/`, `generator/`)

```sh
# Install dependencies
pip install -r requirements.txt

# Run a service
python -m scorer
python -m generator

# Tests
pytest
pytest path/to/test_file.py::test_name
```

### Docker Compose (full stack)

```sh
# Start the stack
docker compose up

# Run the pipeline manually (one-shot)
docker compose run --rm ingestion
docker compose run --rm scorer
docker compose run --rm generator

# Rebuild a specific service
docker compose build api
```

## Configuration

All services share a single `config.yaml` mounted into containers. Secrets are injected via environment variables — never stored in `config.yaml`. Copy `.env.example` to `.env` and populate it before running the stack.

Key environment variables:
- `DATABASE_URL` — PostgreSQL DSN (used only by the API service)
- `API_INTERNAL_SECRET` — shared secret for service-to-service API auth
- `ANTHROPIC_API_KEY` — used directly by the `anthropic` Python SDK
- `NVD_API_KEY`, `GITHUB_TOKEN` — source API credentials

## Database Migrations

Migrations live in `db/migrations/` and are managed via `golang-migrate`. They are embedded in the API service binary. Migration files follow the naming convention `{version}_{description}.up.sql` / `{version}_{description}.down.sql`.

## REST API Conventions

- All endpoints are versioned under `/api/v1/`
- Service-to-service calls include `X-Internal-Secret` header
- The scoring service writes results via `POST /api/v1/vulns/{id}/scores`
- The ingestion service creates records via `POST /api/v1/vulns` and updates via `PUT /api/v1/vulns/{id}`
