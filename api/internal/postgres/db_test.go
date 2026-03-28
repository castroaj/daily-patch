// db_test.go — tests for the database pool constructor and migration runner
//
// Unit tests (no database required) verify embedded migration files and
// error handling for invalid DSNs. Integration tests require a running
// PostgreSQL instance and are skipped when TEST_DATABASE_URL is not set.

package postgres

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"testing"
)

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	envTestDatabaseURL = "TEST_DATABASE_URL"

	// expectedUpMigration is the filename of the up migration that must be
	// present in the embedded FS.
	expectedUpMigration = "migrations/001_create_ingestion_tables.up.sql"

	// expectedDownMigration is the filename of the down migration that must
	// be present in the embedded FS.
	expectedDownMigration = "migrations/001_create_ingestion_tables.down.sql"
)

// -----------------------------------------------------------------------------
// Unit tests (no database required)
// -----------------------------------------------------------------------------

func TestMigrationsEmbedded(t *testing.T) {
	t.Parallel()

	files := []string{expectedUpMigration, expectedDownMigration}

	for _, name := range files {
		f, err := fs.ReadFile(migrations, name)
		if err != nil {
			t.Fatalf("expected embedded file %s, got error: %v", name, err)
		}
		if len(f) == 0 {
			t.Fatalf("embedded file %s is empty", name)
		}
	}
}

func TestNew_InvalidDSN(t *testing.T) {
	t.Parallel()

	_, err := New(context.Background(), "not-a-valid-dsn", discardLogger())
	if err == nil {
		t.Fatal("expected error for invalid DSN, got nil")
	}
}

// -----------------------------------------------------------------------------
// Integration tests (require running PostgreSQL)
// -----------------------------------------------------------------------------

func TestNew_ConnectsAndMigrates(t *testing.T) {
	dsn := testDSN(t)

	pool, err := New(context.Background(), dsn, discardLogger())
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer pool.Close()

	// Verify both tables exist by querying information_schema.
	tables := []string{"vulnerabilities", "ingestion_runs"}
	for _, table := range tables {
		var exists bool
		err := pool.QueryRow(context.Background(),
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)",
			table,
		).Scan(&exists)

		if err != nil {
			t.Fatalf("query for table %s failed: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected table %s to exist after migration", table)
		}
	}
}

func TestNew_MigrationsIdempotent(t *testing.T) {
	dsn := testDSN(t)

	// First call — applies migrations.
	pool1, err := New(context.Background(), dsn, discardLogger())
	if err != nil {
		t.Fatalf("first New() returned error: %v", err)
	}
	pool1.Close()

	// Second call — migrations already applied, should succeed.
	pool2, err := New(context.Background(), dsn, discardLogger())
	if err != nil {
		t.Fatalf("second New() returned error: %v", err)
	}
	pool2.Close()
}

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// testDSN reads TEST_DATABASE_URL from the environment and skips the test
// if it is not set. This gates integration tests behind a real Postgres.
func testDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv(envTestDatabaseURL)
	if dsn == "" {
		t.Skipf("skipping: %s not set (requires running PostgreSQL)", envTestDatabaseURL)
	}

	return dsn
}

// discardLogger returns a slog.Logger that writes to io.Discard, keeping
// test output clean while still exercising all log call sites.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
