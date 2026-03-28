// db.go — pgxpool constructor and embedded migration runner
//
// New opens a connection pool, runs embedded SQL migrations, and pings
// the database within a 5-second deadline. The caller is responsible for
// calling pool.Close() at shutdown.

package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// -----------------------------------------------------------------------------
// Embedded migrations
// -----------------------------------------------------------------------------

//go:embed migrations/*.sql
var migrations embed.FS

// -----------------------------------------------------------------------------
// Constants
// -----------------------------------------------------------------------------

const (
	// pingTimeout is the maximum time New waits for the initial database ping
	// to succeed. A short deadline ensures the service fails fast at startup
	// when the database is unreachable.
	pingTimeout = 5 * time.Second

	// migrationSubdir is the subdirectory within the embedded FS that
	// contains the .sql migration files.
	migrationSubdir = "migrations"

	// migrateSchemePostgres is the standard PostgreSQL DSN scheme that
	// callers provide (e.g. from DATABASE_URL).
	migrateSchemePostgres = "postgres://"

	// migrateSchemePgx5 is the scheme the golang-migrate pgx5 database
	// driver registers under. DSNs must use this scheme for the driver
	// to recognise them.
	migrateSchemePgx5 = "pgx5://"
)

// -----------------------------------------------------------------------------
// Public functions
// -----------------------------------------------------------------------------

// New opens a pgxpool connection pool, runs any pending embedded
// migrations, and pings the database to verify connectivity. Returns an
// error if migrations fail, the pool cannot be created, or the ping does
// not succeed within 5 seconds. The caller must call pool.Close() when
// the pool is no longer needed.
func New(ctx context.Context, dsn string, log *slog.Logger) (*pgxpool.Pool, error) {

	// --- Run embedded migrations --------------------------------------------
	// Migrations must run before the pool is created so that tables exist
	// for any queries issued immediately after New returns.
	log.Info("running embedded database migrations")
	if err := runMigrations(dsn, log); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// --- Create connection pool ---------------------------------------------
	log.Info("creating pgxpool connection pool")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	// --- Ping ---------------------------------------------------------------
	// A short deadline ensures the service fails fast when the database is
	// unreachable, rather than blocking indefinitely at startup.
	log.Info("pinging database to verify connectivity", "timeout", pingTimeout.String())
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Info("database connection established and verified")
	return pool, nil
}

// -----------------------------------------------------------------------------
// Private functions
// -----------------------------------------------------------------------------

// runMigrations applies any pending SQL migrations embedded in the binary.
// The golang-migrate pgx5 driver registers under the "pgx5://" scheme, so
// standard "postgres://" DSNs are converted before being passed to the
// migration runner. Returns nil if all migrations succeed or if there are
// no new migrations to apply.
func runMigrations(dsn string, log *slog.Logger) error {

	source, err := iofs.New(migrations, migrationSubdir)
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}

	// Convert postgres:// → pgx5:// so golang-migrate recognises the driver.
	migrateDSN := toPgx5Scheme(dsn)

	m, err := migrate.NewWithSourceInstance("iofs", source, migrateDSN)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	defer m.Close()

	// Log the current migration version before applying.
	version, dirty, verErr := m.Version()
	if verErr != nil && !errors.Is(verErr, migrate.ErrNilVersion) {
		return fmt.Errorf("read migration version: %w", verErr)
	}
	log.Info("current migration state", "version", version, "dirty", dirty)

	// Up applies all pending migrations. ErrNoChange means the database is
	// already at the latest version — this is expected on subsequent starts.
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info("migrations already up to date, no changes applied")
			return nil
		}
		return fmt.Errorf("apply migrations: %w", err)
	}

	// Log the new version after successful migration.
	newVersion, _, _ := m.Version()
	log.Info("migrations applied successfully", "previous_version", version, "new_version", newVersion)

	return nil
}

// toPgx5Scheme replaces a leading "postgres://" with "pgx5://" so the
// DSN is compatible with the golang-migrate pgx5 database driver. If the
// DSN does not start with "postgres://", it is returned unchanged.
func toPgx5Scheme(dsn string) string {
	if after, ok := strings.CutPrefix(dsn, migrateSchemePostgres); ok {
		return migrateSchemePgx5 + after
	}
	return dsn
}
