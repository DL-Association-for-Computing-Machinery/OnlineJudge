package sessionexperiment

import (
	"context"
	"crypto/rand"
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

type authVersion struct {
	account  int64
	recovery string
}

var errRecovering = errors.New("session recovery unavailable")

func (a *authAuthority) current(ctx context.Context, subject string) (authVersion, error) {
	var version authVersion
	var open bool
	err := a.pool.QueryRow(ctx, `SELECT generation, enabled FROM session_recovery_fixture
		WHERE namespace=$1`, a.namespace).Scan(&version.recovery, &open)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !open) {
		return authVersion{}, errRecovering
	}
	if err != nil {
		return authVersion{}, err
	}
	var enabled bool
	err = a.pool.QueryRow(ctx, `SELECT generation, enabled FROM session_auth_fixture
		WHERE namespace=$1 AND subject=$2`, a.namespace, subject).Scan(&version.account, &enabled)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !enabled) {
		return authVersion{}, errStale
	}
	return version, err
}

func (a *authAuthority) withValid(ctx context.Context, subject string, expected authVersion, operation func() error) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	// Always lock recovery before account state. Recovery waits for every guarded
	// save before closing admission, and stale saves recheck after it reopens.
	var recovery string
	var open bool
	err = tx.QueryRow(ctx, `SELECT generation, enabled FROM session_recovery_fixture
		WHERE namespace=$1 FOR SHARE`, a.namespace).Scan(&recovery, &open)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !open) {
		return errRecovering
	}
	if err != nil {
		return err
	}
	if expected.recovery == "" || recovery != expected.recovery {
		return errStale
	}
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
	if !enabled || expected.account <= 0 || generation != expected.account {
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

func setupAuth(t *testing.T, configure ...sessionConfigurer) *fixture {
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
	_, err = a.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS session_recovery_fixture (
		namespace text PRIMARY KEY, generation text NOT NULL CHECK (length(generation)>=26), enabled boolean NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.pool.Exec(ctx, `INSERT INTO session_recovery_fixture VALUES ($1,$2,true)`, namespace, rand.Text())
	if err != nil {
		t.Fatal(err)
	}
	return setupAuthorizedReplicas(t, "auth:"+namespace+":", 2,
		[]*authAuthority{a, newAuthority(t, namespace)}, configure...)
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

func (a *authAuthority) beginRecovery(ctx context.Context) (string, error) {
	var generation string
	// A counter restored from an old database backup can repeat a generation
	// still present in Redis. An independently random ID avoids that deterministic
	// reuse; it is not an external monotonic fence or a restore-operator lock.
	err := a.pool.QueryRow(ctx, `UPDATE session_recovery_fixture
		SET generation=$2, enabled=false WHERE namespace=$1 RETURNING generation`, a.namespace, rand.Text()).Scan(&generation)
	return generation, err
}

func (a *authAuthority) finishRecovery(ctx context.Context, expected string) error {
	tag, err := a.pool.Exec(ctx, `UPDATE session_recovery_fixture SET enabled=true
		WHERE namespace=$1 AND generation=$2 AND enabled=false`, a.namespace, expected)
	if err == nil && tag.RowsAffected() != 1 {
		return errRecovering
	}
	return err
}
