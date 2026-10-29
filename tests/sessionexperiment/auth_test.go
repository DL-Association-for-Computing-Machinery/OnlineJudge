package sessionexperiment

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Test-only authority: no passwords, production account schema or permission cache.
type authAuthority struct {
	pool      *pgxpool.Pool
	namespace string
}

func (a *authAuthority) current(ctx context.Context, subject string) (int64, error) {
	var generation int64
	var enabled bool
	err := a.pool.QueryRow(ctx, `SELECT generation, enabled FROM session_auth_fixture
		WHERE namespace=$1 AND subject=$2`, a.namespace, subject).Scan(&generation, &enabled)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !enabled) {
		return 0, errStale
	}
	return generation, err
}

func (a *authAuthority) withValid(ctx context.Context, subject string, expected int64, operation func() error) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var generation int64
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT generation, enabled FROM session_auth_fixture
		WHERE namespace=$1 AND subject=$2 FOR SHARE`, a.namespace, subject).Scan(&generation, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return errStale
	}
	if err != nil {
		return err
	}
	if !enabled || expected <= 0 || generation != expected {
		return errStale
	}
	// Serialize Redis saves with account invalidation. This is NOT a distributed
	// transaction: an uncertain Redis write can remain, but its old generation
	// cannot authorize later requests. The request deadline bounds lock holding.
	return operation()
}

func newAuthority(t *testing.T, namespace string) *authAuthority {
	t.Helper()
	dsn := os.Getenv("OJ_SESSION_DATABASE_URL")
	if dsn == "" {
		t.Fatal("requires disposable PostgreSQL: run scripts/test-session.py")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 4
	config.ConnConfig.ConnectTimeout = time.Second
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &authAuthority{pool: pool, namespace: namespace}
}

func setupAuth(t *testing.T) *fixture {
	t.Helper()
	namespace := digest(t.Name())
	a := newAuthority(t, namespace)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := a.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS session_auth_fixture (
		namespace text NOT NULL, subject text NOT NULL, generation bigint NOT NULL CHECK (generation>0),
		enabled boolean NOT NULL, PRIMARY KEY(namespace, subject))`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.pool.Exec(ctx, `INSERT INTO session_auth_fixture VALUES ($1,$2,1,true)`, namespace, "fixture-user")
	if err != nil {
		t.Fatal(err)
	}
	return setupAuthorizedReplicas(t, "auth:"+namespace+":", 2,
		[]*authAuthority{a, newAuthority(t, namespace)})
}

func invalidate(t *testing.T, a *authAuthority, enabled bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tag, err := a.pool.Exec(ctx, `UPDATE session_auth_fixture SET generation=generation+1, enabled=$3
		WHERE namespace=$1 AND subject=$2`, a.namespace, "fixture-user", enabled)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatal("account invalidation did not update exactly one row")
	}
}
