# STANDARDS.md — Coding standards for the daily-patch project

This document defines language-level conventions and idioms for Go and Python code
in this repository. It is the authoritative reference for _how_ code inside source
files should be written. For project structure, file header format, and development
commands, see `CLAUDE.md`. For CI platform details, see `ACTIONS.md`.

- [STANDARDS.md — Coding standards for the daily-patch project](#standardsmd--coding-standards-for-the-daily-patch-project)
  - [Development Methodology](#development-methodology)
    - [Test-Driven Development (TDD)](#test-driven-development-tdd)
      - [Workflow](#workflow)
      - [Rules](#rules)
  - [Go Standards](#go-standards)
    - [1. Constants — No Magic Strings or Numbers](#1-constants--no-magic-strings-or-numbers)
    - [3. Error Handling](#3-error-handling)
    - [4. Import Grouping](#4-import-grouping)
    - [5. Naming](#5-naming)
    - [6. Context Propagation](#6-context-propagation)
    - [7. HTTP Handlers](#7-http-handlers)
    - [8. Logging](#8-logging)
    - [9. Testing](#9-testing)
      - [General](#general)
      - [Test file structure](#test-file-structure)
      - [HTTP client testing](#http-client-testing)
    - [10. File Organization](#10-file-organization)
    - [11. Formatting \& Linting](#11-formatting--linting)
  - [Python Standards](#python-standards)
    - [1. Style \& Formatting](#1-style--formatting)
    - [2. Type Hints](#2-type-hints)
    - [3. Error Handling](#3-error-handling-1)
    - [4. Import Ordering](#4-import-ordering)
    - [5. Naming](#5-naming-1)
    - [6. Testing](#6-testing)
    - [7. Dependencies \& Virtual Environment](#7-dependencies--virtual-environment)
  - [Shared Standards](#shared-standards)
    - [Secrets](#secrets)
    - [File Headers](#file-headers)

---

## Development Methodology

### Test-Driven Development (TDD)

All new functions and methods must be developed using TDD. The workflow is
mandatory and must not be skipped, even for simple functions.

#### Workflow

1. **Stub** — Write the function signature with an empty or `panic("not implemented")` body. No logic yet.
2. **Scope discussion** — Before writing any tests, enumerate:
   - The happy-path scenarios
   - Edge cases and boundary conditions
   - Error conditions (invalid input, dependency failures, etc.)
   - Any requirements derived from the spec or API contract
   Discuss with the team until the scope is agreed on.
3. **Write tests** — Implement the full test suite against the stub. All tests
   must fail at this point (red).
4. **Implement** — Write the minimum code needed to make the tests pass (green).
5. **Refactor** — Clean up the implementation without changing behaviour;
   tests must remain green.

#### Rules

- Never write implementation code before the tests exist for it.
- The stub must be committed (or at least present) before the scope discussion
  begins, so the signature and return types are concrete.
- Scope discussion is not optional — it is the step that prevents missing edge
  cases. Do not shortcut it even when the function looks trivial.
- Tests are written to the agreed scope only; do not add tests during
  implementation that were not discussed.

---

## Go Standards

### 1. Constants — No Magic Strings or Numbers

- Never use a string or numeric literal more than once in source or test code
  without giving it a named constant
- This applies to both source files and `_test.go` files — test code is not
  exempt
- Constants must be declared in the `Constants` section of the file where they
  are first needed (see §10 for file organisation)
- If a constant is shared across multiple files in the same package, declare it
  once in the most relevant source file; test files in the same package access
  it directly without re-declaring it
- Acceptable one-off literals: standard HTTP status codes used inline
  (`http.StatusOK`), zero values (`""`, `0`, `nil`), and boolean literals

### 3. Error Handling

- Every returned error must be checked — no blank-identifier discards (`_ = f()`)
- Use `log.Fatal` / `log.Fatalf` for errors that make the process unrecoverable
- Wrap errors with context using `fmt.Errorf("doing X: %w", err)` so call-site
  context is preserved through the chain
- Inline assignment preferred: `if err := fn(); err != nil { … }`
- Do not use sentinel errors until a package's public API requires them

### 4. Import Grouping

Three groups separated by blank lines, in order:

1. Standard library
2. Third-party modules
3. Internal (`daily-patch/…`) packages

`goimports` (run by golangci-lint) enforces this automatically.

### 5. Naming

- Exported identifiers: `PascalCase`
- Unexported: `camelCase`
- Acronyms: keep consistent case (`userID` not `userId`, `httpClient` not
  `HTTPClient` in unexported)
- Receiver names: short, single letter or two-letter abbreviation of the type
  (`s` for `Server`)
- Package names: lowercase single word, no underscores

### 6. Context Propagation

- Every function that performs I/O (HTTP, DB, file) must accept `context.Context`
  as its first parameter
- Never store a `Context` in a struct; thread it through function calls

### 7. HTTP Handlers

- Always check errors returned by `ResponseWriter` operations (e.g.,
  `json.NewEncoder(w).Encode(…)`)
- Set `Content-Type` header before writing the body
- Use explicit HTTP status codes via `w.WriteHeader()` — never rely on implicit 200
- **Always use `response.Write`** from `api/internal/response` to send responses.
  Never write raw JSON directly from a handler — every response must go through
  the standard envelope (`error`, `errorDetail`, `statusCode`, `result`). See
  SPEC.md §3b for the envelope shape.

### 8. Logging

- Go services use `log/slog` for structured logging
- Loggers are constructed in `main.go` with `slog.NewJSONHandler(os.Stdout, nil)`
  and passed as dependencies — not accessed globally
- Prefer `slog.ErrorContext` / `slog.InfoContext` inside handlers so the context
  (and request ID) is preserved
- Do not use the `log` package in new code; `log.Fatal` / `log.Fatalf` are still
  acceptable in `main.go` for unrecoverable startup errors before the slog logger
  is initialised
- Do not log inside library-style packages; return errors instead

### 9. Testing

#### General

- Test files live alongside the source file they test, named after it:
  `foo_test.go` tests `foo.go`. All tests for a source file go in its single
  corresponding `_test.go` — do not split tests into multiple files per source
  file (e.g. no `foo_bar_test.go` alongside `foo_test.go`)
- Use `package <pkg>` (whitebox) when tests need access to unexported fields or
  types; use `package <pkg>_test` (blackbox) only when the public API alone is
  sufficient
- Prefer table-driven tests for any function with multiple input cases
- Use subtests (`t.Run`) to label cases; each subtest must be independently
  readable
- Use only the standard `testing` package unless a specific helper is clearly
  necessary
- Test function names: `Test<Unit>_<Scenario>` (e.g., `TestParseVuln_MissingCVSS`)
- Every `Test*` function must have a one-line comment immediately above it stating
  its objective — what property it asserts or what contract it verifies. The comment
  must start with the function name followed by a verb (e.g., `asserts`, `verifies`):
  ```go
  // TestNew_storesBaseURL asserts that New stores the provided base URL on the httpClient.
  func TestNew_storesBaseURL(t *testing.T) { ... }
  ```

#### Test file structure

Test files must be divided into named sections using the same 80-column divider
format as source files (see §10):

```
// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// <SourceUnit> (e.g. constructor name or struct being tested)
// -----------------------------------------------------------------------------
```

Required sections, in order:

1. **One section per public unit under test** — group all `Test<Unit>_*`
   functions under a divider named after that unit (e.g. `// New`,
   `// CheckExists`). Omit sections for units with no tests yet.
2. **Test helpers** — shared handler factories, server starters, and assertion
   utilities scoped to this test file. Always last. Keep helpers small and
   composable.

Helpers go at the bottom so test functions — the primary content — are
immediately visible when opening the file. Helpers defined here are
test-private; do not export them or move them to a shared package unless they
are needed by more than one test file.

#### HTTP client testing

Use `net/http/httptest.NewServer` to spin up a real in-process HTTP server
rather than mocking interfaces. This exercises the full request/response path
including URL construction, header setting, and JSON encoding/decoding.

**Server helper pattern** — create a `startServer` helper in the test file that:
1. Registers the handler on `httptest.NewServer`
2. Calls `t.Cleanup(srv.Close)` to shut down after the test
3. Calls `New(srv.URL, secret, DefaultTimeout)` to build the client
4. Sets `retryDelay = 0` on the underlying `httpClient` so retry-path tests
   complete instantly without sleeping

```go
func startServer(t *testing.T, secret string, handler http.HandlerFunc) *httpClient {
    t.Helper()
    srv := httptest.NewServer(handler)
    t.Cleanup(srv.Close)
    c, err := New(srv.URL, secret, DefaultTimeout)
    if err != nil {
        t.Fatalf("New: %v", err)
    }
    hc := c.(*httpClient)
    hc.retryDelay = 0
    return hc
}
```

**Handler composition** — build handlers from small, composable functions rather
than one large switch statement. Common helpers:

- `requireSecret(secret, next)` — returns 401 and stops if the
  `X-Internal-Secret` header does not match; delegates to `next` otherwise
- `respond(w, status, body)` — marshals body to JSON, writes the header, and
  writes the body
- Scenario-specific handlers (e.g., `vulnFound(id)`, `vulnEmpty(status)`) built
  on top of `respond`

**What to cover for each public method:**

| Criterion | What to assert |
|-----------|----------------|
| Status codes | Correct `(value, error)` for 200-with-data, 200-empty, 404, 401, 5xx |
| Payload handling | Correct field extraction; error on malformed JSON |
| Query / path parameters | Capture `r.URL.Query()` or `r.URL.Path` in the handler and assert exact values; assert absent params are not sent |
| Authorization | Valid secret succeeds; wrong secret returns error; header name and value are exact |

**Capturing request details** — close over a variable in a handler to record
what the client sent, then assert on it after the call returns:

```go
var capturedQuery map[string][]string
handler := func(w http.ResponseWriter, r *http.Request) {
    capturedQuery = map[string][]string(r.URL.Query())
    respond(w, http.StatusOK, map[string]any{"data": []any{}})
}
```

**retryDelay** — `httpClient` exposes an unexported `retryDelay time.Duration`
field (zero-valued = no backoff). Always set it to `0` in tests. The production
`New` constructor sets it to `500ms`.

### 10. File Organization

Go source files must follow this section order, each separated by a comment
divider of the form `// --- <Section> ---` (dashes padded to column 80):

```
// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------
```

Order:

1. **Constants** — all `const` declarations
2. **Public types** — exported interfaces and structs (no methods yet)
3. **Public functions** — exported non-method functions (e.g. constructors)
4. **Private types and methods** — unexported structs and all methods defined on them
5. **Private functions** — unexported helpers with no receiver

Omit sections that are empty. Methods always live with their type in the
"Private types and methods" section, never mixed into the public or private
functions sections.

### 11. Formatting & Linting

- Code must pass `golangci-lint run` at the pinned version (currently `v1.64.8`)
- `gofmt` / `goimports` formatting is enforced by the linter; run before committing
- No `//nolint` directives without a comment explaining why

---

## Python Standards

### 1. Style & Formatting

- Follow PEP 8; target line length 88 (Black default, even if Black is not yet
  enforced by a linter step)
- One blank line between top-level functions; two blank lines between top-level
  classes
- Use f-strings for all string interpolation — not `.format()` or `%` syntax

### 2. Type Hints

- Required on all public function signatures (parameters and return type)
- Use `from __future__ import annotations` at the top of files that need forward
  references
- Internal/private helpers should have hints where the types are non-obvious

### 3. Error Handling

- Never use a bare `except:` — always name the exception type(s)
- Catch the most specific exception possible; let unexpected exceptions propagate
- Use `raise ... from err` when re-raising to preserve the original traceback

### 4. Import Ordering

Three groups separated by blank lines, in order:

1. Standard library
2. Third-party packages
3. Local / intra-package imports

### 5. Naming

- Functions and variables: `snake_case`
- Classes: `PascalCase`
- Constants: `UPPER_SNAKE_CASE`
- Private identifiers: single leading underscore (`_helper`)
- Dunder methods / names: as per Python convention

### 6. Testing

- All tests live in the service's `tests/` subdirectory
- Test files named `test_<module>.py`; test functions named
  `test_<what>_<condition>`
- Use plain `assert` statements (pytest rewrites them for readable output)
- No mocking of internal logic; mock only at external boundaries (HTTP calls,
  file I/O, Anthropic API)
- Fixtures go in `tests/conftest.py`

### 7. Dependencies & Virtual Environment

- All runtime and dev dependencies declared in `requirements.txt`
- Never install packages globally; always run inside the service `.venv` created
  by `make setup`
- Pin all dependencies to exact versions in `requirements.txt` (`anthropic==0.72.0`,
  not `anthropic`). Update versions deliberately and regenerate the full pin list
  via `pip freeze` after testing.

---

## Shared Standards

### Secrets

- No secrets, tokens, or credentials in source files or `config.yaml`
- All secrets injected via environment variables at runtime
- `.env` is gitignored; copy from `.env.example`

### File Headers

Defined in `CLAUDE.md` — not duplicated here.
