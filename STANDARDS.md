# STANDARDS.md — Coding standards for the daily-patch project

This document defines language-level conventions and idioms for Go and Python code
in this repository. It is the authoritative reference for _how_ code inside source
files should be written. For project structure, file header format, and development
commands, see `CLAUDE.md`. For CI platform details, see `ACTIONS.md`.

---

## Go Standards

### 1. Error Handling

- Every returned error must be checked — no blank-identifier discards (`_ = f()`)
- Use `log.Fatal` / `log.Fatalf` for errors that make the process unrecoverable
- Wrap errors with context using `fmt.Errorf("doing X: %w", err)` so call-site
  context is preserved through the chain
- Inline assignment preferred: `if err := fn(); err != nil { … }`
- Do not use sentinel errors until a package's public API requires them

### 2. Import Grouping

Three groups separated by blank lines, in order:

1. Standard library
2. Third-party modules
3. Internal (`daily-patch/…`) packages

`goimports` (run by golangci-lint) enforces this automatically.

### 3. Naming

- Exported identifiers: `PascalCase`
- Unexported: `camelCase`
- Acronyms: keep consistent case (`userID` not `userId`, `httpClient` not
  `HTTPClient` in unexported)
- Receiver names: short, single letter or two-letter abbreviation of the type
  (`s` for `Server`)
- Package names: lowercase single word, no underscores

### 4. Context Propagation

- Every function that performs I/O (HTTP, DB, file) must accept `context.Context`
  as its first parameter
- Never store a `Context` in a struct; thread it through function calls

### 5. HTTP Handlers

- Always check errors returned by `ResponseWriter` operations (e.g.,
  `json.NewEncoder(w).Encode(…)`)
- Set `Content-Type` header before writing the body
- Use explicit HTTP status codes via `w.WriteHeader()` — never rely on implicit 200

### 6. Logging

- Use the standard `log` package (no structured logger yet; will be revisited when
  services grow)
- `log.Fatal` / `log.Fatalf` for unrecoverable startup errors
- `log.Printf` for informational runtime messages
- Do not log inside library-style packages; return errors instead

### 7. Testing

- Test files live alongside the source file they test (`foo_test.go` next to
  `foo.go`)
- Prefer table-driven tests for any function with multiple input cases
- Use subtests (`t.Run`) to label cases; each subtest must be independently
  readable
- Use only the standard `testing` package unless a specific helper is clearly
  necessary
- Test function names: `Test<Unit>_<Scenario>` (e.g., `TestParseVuln_MissingCVSS`)

### 8. File Organization

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

### 9. Formatting & Linting

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
