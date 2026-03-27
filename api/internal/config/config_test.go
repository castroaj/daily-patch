// config_test.go — tests for config.Load

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// -----------------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------------

func TestLoad_ValidFile(t *testing.T) {
	clearEnv(t)
	yaml := `
api:
  listen: ":9090"
  internal_secret: "file-secret"
database:
  dsn: "postgres://localhost/test"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.API.Listen != ":9090" {
		t.Errorf("Listen = %q, want %q", cfg.API.Listen, ":9090")
	}
	if cfg.API.InternalSecret != "file-secret" {
		t.Errorf("InternalSecret = %q, want %q", cfg.API.InternalSecret, "file-secret")
	}
	if cfg.Database.DSN != "postgres://localhost/test" {
		t.Errorf("DSN = %q, want %q", cfg.Database.DSN, "postgres://localhost/test")
	}
}

func TestLoad_DefaultListen(t *testing.T) {
	clearEnv(t)
	yaml := `
api:
  internal_secret: "s"
database:
  dsn: "postgres://localhost/test"
`
	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.API.Listen != defaultListen {
		t.Errorf("Listen = %q, want default %q", cfg.API.Listen, defaultListen)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	yaml := `
api:
  internal_secret: "file-secret"
database:
  dsn: "file-dsn"
`
	t.Setenv(envInternalSecret, "env-secret")
	t.Setenv(envDatabaseURL, "env-dsn")

	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.API.InternalSecret != "env-secret" {
		t.Errorf("InternalSecret = %q, want env value %q", cfg.API.InternalSecret, "env-secret")
	}
	if cfg.Database.DSN != "env-dsn" {
		t.Errorf("DSN = %q, want env value %q", cfg.Database.DSN, "env-dsn")
	}
}

func TestLoad_EnvOnlyNoFile(t *testing.T) {
	yaml := `
api:
database:
`
	t.Setenv(envInternalSecret, "env-secret")
	t.Setenv(envDatabaseURL, "env-dsn")

	path := writeTemp(t, yaml)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.API.InternalSecret != "env-secret" {
		t.Errorf("InternalSecret = %q, want %q", cfg.API.InternalSecret, "env-secret")
	}
	if cfg.Database.DSN != "env-dsn" {
		t.Errorf("DSN = %q, want %q", cfg.Database.DSN, "env-dsn")
	}
}

func TestLoad_MissingInternalSecret(t *testing.T) {
	clearEnv(t)
	yaml := `
api:
database:
  dsn: "postgres://localhost/test"
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for missing InternalSecret, got nil")
	}
}

func TestLoad_MissingDSN(t *testing.T) {
	clearEnv(t)
	yaml := `
api:
  internal_secret: "s"
database:
`
	path := writeTemp(t, yaml)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for missing DSN, got nil")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	clearEnv(t)
	path := writeTemp(t, ":::invalid yaml:::")
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for malformed YAML, got nil")
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// clearEnv unsets API_INTERNAL_SECRET and DATABASE_URL for the duration of
// the test and restores them afterwards. Call at the top of any test that
// asserts file-sourced values, so a developer's shell environment cannot
// silently override them.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{envInternalSecret, envDatabaseURL} {
		prior, ok := os.LookupEnv(key)
		os.Unsetenv(key)
		if ok {
			t.Cleanup(func() { os.Setenv(key, prior) })
		}
	}
}

// writeTemp writes content to a temporary file and returns its path.
// The file is removed when the test ends.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return filepath.Clean(f.Name())
}
