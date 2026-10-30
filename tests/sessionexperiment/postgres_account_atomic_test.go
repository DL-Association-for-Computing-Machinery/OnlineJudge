package sessionexperiment

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func postgresAccountsSnapshot(t *testing.T, s *postgresSessionStore) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var b []byte
	err := s.pool.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY subject),'[]'::jsonb)
		FROM (SELECT subject,generation,enabled FROM session_pg_account_fixture WHERE namespace=$1) r`, s.namespace).Scan(&b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func postgresTokenSnapshot(t *testing.T, s *postgresSessionStore, token string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var b []byte
	err := s.pool.QueryRow(ctx, `SELECT to_jsonb(s) FROM session_pg_store_fixture s WHERE namespace=$1 AND token=$2`, s.namespace, digest(token)).Scan(&b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func postgresBlockedPID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, holder int32) int32 {
	t.Helper()
	for {
		var pid int32
		err := pool.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid))`, holder).Scan(&pid)
		if err == nil {
			return pid
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("actual PostgreSQL lock wait not observed", err)
		}
		select {
		case <-ctx.Done():
			t.Fatal("lock wait deadline")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func postgresRollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func TestPostgresAccountCommitWaitsForInvalidation(t *testing.T) {
	f := setupPostgresSessions(t, true)
	s := f.stores[0]
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cookie := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	b, _, err := s.FindCtx(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer postgresRollback(tx)
	// Execute the real invalidation body, holding its uncommitted account,
	// registry and token changes until the competing save is observed waiting.
	if err := s.invalidateAccountTx(ctx, tx, "fixture-user", true); err != nil {
		t.Fatal(err)
	}
	var holder int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- f.stores[1].CommitCtx(ctx, cookie.Value, b, time.Now().Add(time.Minute)) }()
	postgresBlockedPID(t, ctx, s.pool, holder)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; !errors.Is(err, errStale) {
		t.Fatal("save did not recheck the newly committed account generation", err)
	}
	if generation, enabled := postgresAccountState(t, s); generation != 2 || !enabled || postgresRows(t, s, true) != 0 {
		t.Fatal("invalidation was not atomic")
	}
}

func TestPostgresAccountInvalidationWaitsForCommit(t *testing.T) {
	f := setupPostgresSessions(t, true)
	s := f.stores[0]
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cookie := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	b, _, err := s.FindCtx(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer postgresRollback(tx)
	if err := s.lockSubject(ctx, tx, digest("fixture-user")); err != nil {
		t.Fatal(err)
	}
	var holder int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	saved, invalidated := make(chan error, 1), make(chan error, 1)
	go func() { saved <- f.stores[1].CommitCtx(ctx, cookie.Value, b, time.Now().Add(3*time.Minute)) }()
	saver := postgresBlockedPID(t, ctx, s.pool, holder)
	// The saver has acquired account SHARE and is waiting for the registry.
	go func() { invalidated <- s.invalidateAccount(ctx, "fixture-user", true) }()
	postgresBlockedPID(t, ctx, s.pool, saver)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Fatal("admitted save failed", err)
	}
	if err := <-invalidated; err != nil {
		t.Fatal("invalidation did not resume after save committed", err)
	}
	if generation, enabled := postgresAccountState(t, s); generation != 2 || !enabled || postgresRows(t, s, true) != 0 {
		t.Fatal("invalidation missed the save that committed before it")
	}
	for i := range 2 {
		expect(t, f.api, i, "GET", "/me", cookie, 401)
	}
}

func TestPostgresAccountUpdateFailureRollsBack(t *testing.T) {
	f := setupPostgresSessions(t, true)
	s := f.stores[0]
	ctx := context.Background()
	cookie := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	for range 2 {
		expect(t, f.api, 1, "POST", "/login-fixture", nil, 201)
	}
	other := *s
	other.namespace = digest(t.Name() + "/other-namespace")
	for _, seed := range []struct {
		store   *postgresSessionStore
		subject string
	}{{s, "other-user"}, {&other, "fixture-user"}} {
		_, err := s.pool.Exec(ctx, `INSERT INTO session_pg_account_fixture VALUES($1,$2,1,true)`, seed.store.namespace, digest(seed.subject))
		if err != nil {
			t.Fatal(err)
		}
		if err := seed.store.CommitCtx(ctx, "unrelated-token", postgresAccountPayload(t, seed.subject, 1), time.Now().Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	before, accounts := postgresSnapshot(t, s), postgresAccountsSnapshot(t, s)
	otherSubject := postgresTokenSnapshot(t, s, "unrelated-token")
	otherBefore, otherAccounts := postgresSnapshot(t, &other), postgresAccountsSnapshot(t, &other)
	_, err := s.pool.Exec(ctx, `CREATE FUNCTION session_pg_account_reject_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'synthetic account update failure'; END $$;
		CREATE TRIGGER session_pg_account_reject_fixture AFTER UPDATE ON session_pg_store_fixture
		FOR EACH ROW EXECUTE FUNCTION session_pg_account_reject_fixture()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := s.pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS session_pg_account_reject_fixture ON session_pg_store_fixture;
			DROP FUNCTION IF EXISTS session_pg_account_reject_fixture()`)
		if err != nil {
			t.Error(err)
		}
	})
	err = f.stores[1].invalidateAccount(ctx, "fixture-user", false)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
		t.Fatal("invalidation did not reach the session UPDATE trigger", err)
	}
	if !bytes.Equal(before, postgresSnapshot(t, s)) || !bytes.Equal(accounts, postgresAccountsSnapshot(t, s)) ||
		!bytes.Equal(otherBefore, postgresSnapshot(t, &other)) || !bytes.Equal(otherAccounts, postgresAccountsSnapshot(t, &other)) {
		t.Fatal("UPDATE failure did not roll back account and every session field")
	}
	for i := range 2 {
		expect(t, f.api, i, "GET", "/me", cookie, 200)
	}
	r := expect(t, f.api, 0, "POST", "/activity-fixture", cookie, 503)
	if len(r.Header.Values("Set-Cookie")) != 0 || !bytes.Equal(before, postgresSnapshot(t, s)) || !bytes.Equal(accounts, postgresAccountsSnapshot(t, s)) {
		t.Fatal("failed HTTP UPDATE sent cookie or changed state")
	}
	if _, err := s.pool.Exec(ctx, `DROP TRIGGER session_pg_account_reject_fixture ON session_pg_store_fixture;
		DROP FUNCTION session_pg_account_reject_fixture()`); err != nil {
		t.Fatal(err)
	}
	if err := s.invalidateAccount(ctx, "fixture-user", false); err != nil {
		t.Fatal(err)
	}
	if generation, enabled := postgresAccountState(t, s); generation != 2 || enabled || postgresRows(t, s, true) != 1 {
		t.Fatal("recovered invalidation failed to clear exactly the target subject")
	}
	for _, store := range []*postgresSessionStore{s, &other} {
		if _, found, err := store.FindCtx(ctx, "unrelated-token"); err != nil || !found {
			t.Fatal("invalidation affected another subject or namespace", err)
		}
	}
	if !bytes.Equal(otherBefore, postgresSnapshot(t, &other)) || !bytes.Equal(otherAccounts, postgresAccountsSnapshot(t, &other)) {
		t.Fatal("other namespace changed after successful invalidation")
	}
	if !bytes.Equal(otherSubject, postgresTokenSnapshot(t, s, "unrelated-token")) {
		t.Fatal("other subject's session fields changed after successful invalidation")
	}
}
