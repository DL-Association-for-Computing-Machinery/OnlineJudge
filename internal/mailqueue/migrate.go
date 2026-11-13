package mailqueue

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// ErrMigrationBusy reports that another application or River migration holds
// the database maintenance lock.
var ErrMigrationBusy = errors.New("database migration already running")

// Keep this tied to the pinned River release. Upgrades require a migration rehearsal.
const riverSchemaVersion = 8

// MigrateUp runs River's separately committed migrations under the shared
// Goose maintenance lock. The pool needs at least two connections. It uses
// the same database and search_path as the application and enqueuer.
func MigrateUp(ctx context.Context, pool *pgxpool.Pool) error {
	if ctx == nil || pool == nil || pool.Config().MaxConns < 2 {
		return identity.ErrInvalid
	}
	lock, err := pool.Acquire(ctx)
	if err != nil {
		return errQueue
	}
	conn := lock.Hijack()
	// Close instead of returning a session potentially still holding a lock.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = conn.Close(cleanup)
	}()
	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", db.MigrationLockID).Scan(&locked); err != nil {
		return errQueue
	}
	if !locked {
		return ErrMigrationBusy
	}
	if err := db.CheckSchema(ctx, conn); err != nil {
		return err
	}
	m, err := migrator(riverpgxv5.New(pool))
	if err != nil {
		return err
	}
	if _, err := currentVersion(ctx, m); err != nil {
		return err
	}
	if _, err := m.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{TargetVersion: riverSchemaVersion}); err != nil {
		return errQueue
	}
	return checkVersions(ctx, m)
}

func migrator(driver *riverpgxv5.Driver) (*rivermigrate.Migrator[pgx.Tx], error) {
	m, err := rivermigrate.New(driver, &rivermigrate.Config{Logger: quietLogger()})
	if err != nil {
		return nil, errQueue
	}
	return m, nil
}

func validateMigrations(ctx context.Context, driver *riverpgxv5.Driver) error {
	m, err := migrator(driver)
	if err != nil {
		return err
	}
	return checkVersions(ctx, m)
}

func checkVersions(ctx context.Context, m *rivermigrate.Migrator[pgx.Tx]) error {
	version, err := currentVersion(ctx, m)
	if err != nil || version != riverSchemaVersion {
		return errQueue
	}
	return nil
}

func currentVersion(ctx context.Context, m *rivermigrate.Migrator[pgx.Tx]) (int, error) {
	versions, err := m.ExistingVersions(ctx)
	if err != nil || len(versions) > riverSchemaVersion {
		return 0, errQueue
	}
	for i, migration := range versions {
		if migration.Version != i+1 {
			return 0, errQueue
		}
	}
	return len(versions), nil
}
