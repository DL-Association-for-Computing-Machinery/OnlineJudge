// Package db owns schema migrations; API processes only check compatibility.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrations embed.FS

// MigrationLockID is shared by every independent migration process.
const MigrationLockID int64 = 2026102501

// Provider creates a Goose provider with a bounded PostgreSQL advisory lock.
// The caller owns and closes conn after migrations finish.
func Provider(conn *sql.DB) (*goose.Provider, error) {
	files, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockID(MigrationLockID), lock.WithLockTimeout(1, 10))
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, conn, files, goose.WithSessionLocker(locker))
}

// CheckSchema refuses uninitialized, older or newer schemas without migrating.
func CheckSchema(ctx context.Context, connection interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	var version int64
	if err := connection.QueryRow(ctx, `SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1`).Scan(&version); err != nil {
		return errors.New("database schema unavailable; run the independent migration step")
	}
	if version != 23 {
		return errors.New("incompatible database schema")
	}
	return nil
}
