# ACTIONS.md — CI Platform Guide

This document describes the GitHub Actions CI platform for daily-patch. Update it whenever a workflow is added or materially changed.

---

## Workflows

### `ci.yml` — Build, Test, Lint

**Triggers:** `push` to `main` or `feature/**`; `pull_request` targeting `main`.

A `changes` job runs first using `dorny/paths-filter` to detect which service directories were modified. Each service job is gated on that output, so only the services touched in a given commit run their full pipeline.

| Job | Condition | Steps |
|-----|-----------|-------|
| `api` | `api/**` changed | setup-go 1.23 → cache modules → `make setup` → `make build test lint` |
| `ingestion` | `ingestion/**` changed | setup-go 1.23 → cache modules → `make setup` → `make build test lint` |
| `scorer` | `scorer/**` changed | setup-python 3.12 → cache `.venv` → `make setup test lint` |
| `generator` | `generator/**` changed | setup-python 3.12 → cache `.venv` → `make setup test lint` |

**Caching:**
- Go jobs cache `~/.cache/go-build` and `~/go/pkg/mod`, keyed on `<service>/go.mod`.
- Python jobs cache the service `.venv` directory, keyed on `<service>/requirements.txt`.

**Linting:** `make setup` installs `golangci-lint` at the pinned version defined in each Go service's Makefile before `make lint` runs.

---

### `docker.yml` — Docker Image Builds

**Triggers:** `push` to `main`; `pull_request` targeting `main`.

Builds all four service images unconditionally using BuildKit (`docker/setup-buildx-action`). Images are validated but **not pushed** to any registry at this baseline.

| Job | Context |
|-----|---------|
| `build-api` | `api/` |
| `build-ingestion` | `ingestion/` |
| `build-scorer` | `scorer/` |
| `build-generator` | `generator/` |

Each job contains a commented-out push block — see "Extending Workflows" below.

---

### `codeql.yml` — CodeQL Static Analysis

**Triggers:** `push` to `main`; `pull_request` targeting `main`.

Runs GitHub's CodeQL engine across both language ecosystems using a matrix strategy. Required by the main branch ruleset before merging.

| Job | Languages | Steps |
|-----|-----------|-------|
| `analyze` | `go`, `python` (matrix) | checkout → `codeql-action/init` → `codeql-action/autobuild` → `codeql-action/analyze` |

**Permissions required:** `security-events: write`, `actions: read`, `contents: read`.

---

### `trivy.yml` — Container Image Security Scan

**Triggers:** `push` to `main`; `pull_request` targeting `main`.

Builds each service image locally (not pushed) then runs Trivy twice per image: once in CycloneDX SBOM format (artifact) and once in SARIF format (GitHub Security tab). Uses `fail-fast: false` so a finding in one image does not abort scans of the others. `exit-code: 0` means findings are reported but do not fail the workflow.

| Job | Matrix | Outputs |
|-----|--------|---------|
| `scan` | `api`, `ingestion`, `scorer`, `generator` | `sbom-<service>` artifact (CycloneDX, 90-day retention) + SARIF upload to GitHub Security tab |

**Permissions required:** `contents: read`, `security-events: write`.

---

## Required Secrets

The current baseline requires no user-defined secrets. All four workflows use only the automatic `GITHUB_TOKEN` provided by GitHub Actions. No images are pushed and no external APIs are called.

| Secret / Permission | Purpose | Needed when |
|---------------------|---------|-------------|
| `GITHUB_TOKEN` (auto) | Upload SARIF to GitHub Security tab; push images to ghcr.io | Already active for Trivy/CodeQL; push requires `packages: write` scope |
| `DATABASE_URL` | Integration tests against a live DB | When integration tests are added |

---

## Extending the Workflows

### Adding steps to an existing job

Open `.github/workflows/ci.yml` and add your step to the relevant job's `steps` list. Keep build/test/lint as a single `make` invocation where possible; add new `make` targets to the service Makefile instead of inline shell.

### Adding a new workflow

1. Create `.github/workflows/<name>.yml`.
2. Add a row to the table in this document.
3. Document its trigger, any new secrets it needs, and how to test it locally (e.g., with `act`).

### Enabling image publishing

Each job in `docker.yml` contains a commented-out block for `docker/login-action` and a push-enabled `docker/build-push-action`. To activate:

1. Uncomment the blocks in the target job(s).
2. Ensure the repo has the `packages: write` permission in the workflow (add `permissions: packages: write` at the job level).
3. Verify `GITHUB_TOKEN` has sufficient scope in repository settings.

### Adding integration tests

Once the database schema stabilises, add a `postgres` service container to the `api` job and a separate `integration` job that spins up the full Docker Compose stack and runs a smoke-test suite.

---

## Planned Additions

These items are scoped to future milestones and will be added to this document when implemented:

- **Integration tests** — spin up PostgreSQL, run migrations, exercise the REST API end-to-end.
- **Migration validation** — run `migrate up` / `migrate down` against a clean DB on every schema change.
- **Release workflow** — tag a semver release, build and push versioned images to ghcr.io, publish a GitHub Release.
- **Dependency review** — `actions/dependency-review-action` on PRs to catch newly introduced vulnerable packages.
