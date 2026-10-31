package sessionexperiment

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresRecoveryMissingGate(t *testing.T) {
	f := setupPostgresSessionOptions(t, true, true)
	s := f.stores[0]
	ctx := context.Background()
	cookie := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	b, _, err := s.FindCtx(ctx, cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM session_pg_recovery_fixture WHERE namespace=$1`, s.namespace); err != nil {
		t.Fatal(err)
	}
	before, accounts := postgresSnapshot(t, s), postgresAccountsSnapshot(t, s)
	for i, store := range f.stores {
		for _, token := range []string{cookie.Value, "unknown"} {
			if _, found, err := store.FindCtx(ctx, token); found || !errors.Is(err, errRecovering) {
				t.Fatal("missing gate lookup failed open", err)
			}
		}
		for _, path := range []string{"/me", "/activity-fixture", "/logout", "/login-fixture"} {
			method, c := "POST", cookie
			if path == "/me" {
				method = "GET"
			}
			if path == "/login-fixture" {
				c = nil
			}
			rejectWithoutCookie(t, f.api, i, method, path, c, 503)
		}
		if err := store.CommitCtx(ctx, cookie.Value, b, time.Now().Add(time.Minute)); !errors.Is(err, errRecovering) {
			t.Fatal("missing gate save accepted", err)
		}
		if err := store.invalidateAccount(ctx, "fixture-user", false); !errors.Is(err, errRecovering) {
			t.Fatal("missing gate account mutation accepted", err)
		}
	}
	if _, err := s.beginRecovery(ctx); !errors.Is(err, errRecovering) {
		t.Fatal("begin silently recreated missing authority", err)
	}
	if !bytes.Equal(before, postgresSnapshot(t, s)) || !bytes.Equal(accounts, postgresAccountsSnapshot(t, s)) {
		t.Fatal("missing gate operations changed persistent state")
	}
}

func TestPostgresRecoveryCommitWaitsForClosure(t *testing.T) {
	f := setupPostgresSessionOptions(t, true, true)
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
	generation := rand.Text()
	if err := s.beginRecoveryTx(ctx, tx, generation); err != nil {
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
	if err := <-finished; !errors.Is(err, errRecovering) {
		t.Fatal("save did not recheck committed gate closure", err)
	}
	if got, open := postgresRecoveryState(t, s); got != generation || open || postgresRows(t, s, true) != 0 {
		t.Fatal("recovery was not atomic")
	}
}

func TestPostgresRecoveryClosureWaitsForCommit(t *testing.T) {
	f := setupPostgresSessionOptions(t, true, true)
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
	saved, recovered := make(chan error, 1), make(chan error, 1)
	go func() { saved <- f.stores[1].CommitCtx(ctx, cookie.Value, b, time.Now().Add(3*time.Minute)) }()
	saver := postgresBlockedPID(t, ctx, s.pool, holder)
	go func() { _, err := s.beginRecovery(ctx); recovered <- err }()
	postgresBlockedPID(t, ctx, s.pool, saver)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Fatal("guarded save failed", err)
	}
	if err := <-recovered; err != nil {
		t.Fatal("recovery did not resume after save", err)
	}
	if _, open := postgresRecoveryState(t, s); open || postgresRows(t, s, true) != 0 {
		t.Fatal("recovery missed the save preceding closure")
	}
}

func TestPostgresRecoveryUpdateFailureAndNamespaceIsolation(t *testing.T) {
	f := setupPostgresSessionOptions(t, true, true)
	s := f.stores[0]
	ctx := context.Background()
	cookie := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	other := *s
	other.namespace = digest(t.Name() + "/other")
	if err := other.initializeRecovery(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO session_pg_account_fixture VALUES($1,$2,1,true)`, other.namespace, digest("fixture-user")); err != nil {
		t.Fatal(err)
	}
	if err := other.CommitCtx(ctx, "other-token", postgresRecoveryPayload(t, &other, "fixture-user"), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	before, accounts := postgresSnapshot(t, s), postgresAccountsSnapshot(t, s)
	initial, open := postgresRecoveryState(t, s)
	otherBefore, otherAccounts := postgresSnapshot(t, &other), postgresAccountsSnapshot(t, &other)
	otherID, otherOpen := postgresRecoveryState(t, &other)
	_, err := s.pool.Exec(ctx, `CREATE FUNCTION session_pg_recovery_reject_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'synthetic recovery update failure'; END $$;
		CREATE TRIGGER session_pg_recovery_reject_fixture AFTER UPDATE ON session_pg_store_fixture
		FOR EACH ROW EXECUTE FUNCTION session_pg_recovery_reject_fixture()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := s.pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS session_pg_recovery_reject_fixture ON session_pg_store_fixture;
			DROP FUNCTION IF EXISTS session_pg_recovery_reject_fixture()`)
		if err != nil {
			t.Error(err)
		}
	})
	generation, err := s.beginRecovery(ctx)
	var pgErr *pgconn.PgError
	if generation != "" || !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
		t.Fatal("recovery did not reach UPDATE failure", err)
	}
	if got, enabled := postgresRecoveryState(t, s); got != initial || enabled != open || !bytes.Equal(before, postgresSnapshot(t, s)) || !bytes.Equal(accounts, postgresAccountsSnapshot(t, s)) {
		t.Fatal("failed recovery did not roll back every field")
	}
	for i := range 2 {
		expect(t, f.api, i, "GET", "/me", cookie, http.StatusOK)
	}
	if _, err := s.pool.Exec(ctx, `DROP TRIGGER session_pg_recovery_reject_fixture ON session_pg_store_fixture;
		DROP FUNCTION session_pg_recovery_reject_fixture()`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.beginRecovery(ctx); err != nil {
		t.Fatal(err)
	}
	if got, enabled := postgresRecoveryState(t, &other); got != otherID || enabled != otherOpen || !bytes.Equal(otherBefore, postgresSnapshot(t, &other)) || !bytes.Equal(otherAccounts, postgresAccountsSnapshot(t, &other)) {
		t.Fatal("recovery changed another namespace")
	}
	if _, found, err := other.FindCtx(ctx, "other-token"); err != nil || !found {
		t.Fatal("other namespace no longer authorizes", err)
	}
}
