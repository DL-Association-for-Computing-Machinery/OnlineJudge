package sessionexperiment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type postgresCleanupRow struct {
	token            string
	deadline, expiry int64
	data             []byte
}

func postgresCleanupClock(t *testing.T, s *postgresSessionStore) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var now int64
	if err := s.pool.QueryRow(ctx, `SELECT floor(extract(epoch FROM clock_timestamp())*1000)::bigint`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return now
}

func seedPostgresCleanup(t *testing.T, s *postgresSessionStore, rows ...postgresCleanupRow) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := s.pool.Exec(ctx, `INSERT INTO session_pg_subject_fixture VALUES($1,$2) ON CONFLICT DO NOTHING`, s.namespace, digest("fixture-user")); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		_, err := s.pool.Exec(ctx, `INSERT INTO session_pg_store_fixture VALUES($1,$2,$3,$4,$5,$6)`,
			s.namespace, digest(row.token), digest("fixture-user"), row.data, row.deadline, row.expiry)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Compare every column of all four tables, allowing only the named deletions.
func assertPostgresCleanup(t *testing.T, s *postgresSessionStore, before []byte, removed ...string) {
	t.Helper()
	var expected, actual map[string]interface{}
	if err := json.Unmarshal(before, &expected); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(postgresRestoreState(t, s), &actual); err != nil {
		t.Fatal(err)
	}
	tokens := make(map[string]bool)
	for _, token := range removed {
		tokens[digest(token)] = true
	}
	kept := make([]interface{}, 0)
	for _, row := range expected["sessions"].([]interface{}) {
		if !tokens[row.(map[string]interface{})["token"].(string)] {
			kept = append(kept, row)
		}
	}
	expected["sessions"] = kept
	if !reflect.DeepEqual(expected, actual) {
		t.Fatal("cleanup changed state beyond the expected token deletions")
	}
}

func TestPostgresCleanupBoundariesAndIsolation(t *testing.T) {
	f := setupPostgresSessionOptions(t, true, true)
	s := f.stores[0]
	cutoff := postgresCleanupClock(t, s)
	seedPostgresCleanup(t, s,
		postgresCleanupRow{"oldest", cutoff - 3, cutoff - 3, nil},
		postgresCleanupRow{"expired-data", cutoff - 2, cutoff - 2, []byte{1}},
		postgresCleanupRow{"before", cutoff - 1, cutoff - 1, nil},
		postgresCleanupRow{"equal", cutoff, cutoff, []byte{2}},
		postgresCleanupRow{"after", cutoff + 1, cutoff + 1, nil},
		postgresCleanupRow{"revoked", cutoff + 3600000, cutoff + 60000, nil},
		postgresCleanupRow{"idle-expired", cutoff + 3600000, cutoff - 1, []byte{3}},
		postgresCleanupRow{"active", cutoff + 3600000, cutoff + 60000, []byte{4}})
	other := *s
	other.namespace = digest(t.Name() + "/other")
	seedPostgresCleanup(t, &other, postgresCleanupRow{"equal", cutoff, cutoff, []byte{5}})
	before, otherBefore := postgresRestoreState(t, s), postgresRestoreState(t, &other)
	for _, batch := range []int{-1, 0, postgresCleanupMaxBatch + 1} {
		if n, err := s.cleanupExpired(context.Background(), batch); n != 0 || !errors.Is(err, errCleanupBatch) {
			t.Fatal("invalid batch was accepted", n, err)
		}
	}
	assertPostgresCleanup(t, s, before)
	removed := []string{}
	for i, expected := range []int64{2, 2, 0} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		var n int64
		err := s.transact(ctx, func(tx pgx.Tx) error {
			var err error
			n, err = s.cleanupExpiredTx(ctx, tx, cutoff, 2)
			return err
		})
		cancel()
		if err != nil || n != expected {
			t.Fatal("bounded batch at fixed database cutoff", n, expected, err)
		}
		switch i {
		case 0:
			removed = append(removed, "oldest", "expired-data")
		case 1:
			removed = append(removed, "before", "equal")
		}
		assertPostgresCleanup(t, s, before, removed...)
		assertPostgresCleanup(t, &other, otherBefore)
	}
}

func TestPostgresCleanupCompetingBatches(t *testing.T) {
	for _, commit := range []bool{true, false} {
		name := "commit"
		if !commit {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			f := setupPostgresSessionOptions(t, true, true)
			s := f.stores[0]
			cutoff := postgresCleanupClock(t, s)
			seedPostgresCleanup(t, s,
				postgresCleanupRow{"one", cutoff - 3, cutoff - 3, nil},
				postgresCleanupRow{"two", cutoff - 2, cutoff - 2, nil},
				postgresCleanupRow{"three", cutoff - 1, cutoff - 1, nil})
			before := postgresRestoreState(t, s)
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			tx, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer postgresRollback(tx)
			if n, err := s.cleanupExpiredTx(ctx, tx, cutoff, 1); err != nil || n != 1 {
				t.Fatal("first real cleanup transaction", n, err)
			}
			// The first DELETE remains uncommitted and holds its token lock.
			// Another pool must skip it, commit the other two, and not double count.
			if n, err := f.stores[1].cleanupExpired(ctx, 3); err != nil || n != 2 {
				t.Fatal("competing cleanup did not skip the locked deletion", n, err)
			}
			assertPostgresCleanup(t, s, before, "two", "three")
			remaining := int64(1)
			if commit {
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				remaining = 0
			} else {
				postgresRollback(tx)
			}
			if n, err := s.cleanupExpired(ctx, 3); err != nil || n != remaining {
				t.Fatal("released token was not handled exactly once", n, err)
			}
			assertPostgresCleanup(t, s, before, "one", "two", "three")
		})
	}
}

func TestPostgresCleanupCannotResurrect(t *testing.T) {
	f := setupPostgresSessionOptions(t, true, true)
	s := f.stores[0]
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, values, err := (scs.GobCodec{}).Decode(postgresRecoveryPayload(t, s, "fixture-user"))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.UnixMilli(postgresCleanupClock(t, s) + 350)
	b, err := (scs.GobCodec{}).Encode(deadline, values)
	if err != nil {
		t.Fatal(err)
	}
	const token = "cleanup-late-token"
	if err := s.CommitCtx(ctx, token, b, deadline); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := f.stores[1].FindCtx(ctx, token)
	if err != nil || !found || !bytes.Equal(b, loaded) {
		t.Fatal("original payload was not loaded before real expiry", err)
	}
	until := time.Now().Add(2 * time.Second)
	for postgresCleanupClock(t, s) < deadline.UnixMilli() {
		if time.Now().After(until) {
			t.Fatal("database absolute expiry not observed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	before := postgresRestoreState(t, s)
	if n, err := s.cleanupExpired(ctx, 1); err != nil || n != 1 {
		t.Fatal("absolute-expired row was not physically removed", n, err)
	}
	assertPostgresCleanup(t, s, before, token)
	for _, expiry := range []time.Time{deadline, time.Now().Add(time.Minute)} {
		for _, store := range f.stores {
			if err := store.CommitCtx(ctx, token, loaded, expiry); !errors.Is(err, errStale) {
				t.Fatal("removed token was recreated by a loaded payload", err)
			}
			assertPostgresCleanup(t, s, before, token)
		}
	}
	cookie := &http.Cookie{Name: f.api.managers[0].Cookie.Name, Value: token}
	for i := range 2 {
		expect(t, f.api, i, "GET", "/me", cookie, http.StatusUnauthorized)
	}
	assertPostgresCleanup(t, s, before, token)
	for i := range 4 {
		status := http.StatusCreated
		if i == 3 {
			status = http.StatusConflict
		}
		expect(t, f.api, i%2, "POST", "/login-fixture", nil, status)
	}
	after := postgresRestoreState(t, s)
	for _, store := range f.stores {
		if err := store.CommitCtx(ctx, token, loaded, time.Now().Add(time.Minute)); !errors.Is(err, errStale) {
			t.Fatal("late save after replacement admission was not stale", err)
		}
	}
	if !bytes.Equal(after, postgresRestoreState(t, s)) {
		t.Fatal("late save displaced a replacement or changed authority state")
	}
}

func TestPostgresCleanupDeleteFailureRollsBack(t *testing.T) {
	f := setupPostgresSessionOptions(t, true, true)
	s := f.stores[0]
	cutoff := postgresCleanupClock(t, s)
	seedPostgresCleanup(t, s,
		postgresCleanupRow{"one", cutoff - 2, cutoff - 2, nil},
		postgresCleanupRow{"two", cutoff - 1, cutoff - 1, []byte{1}})
	before := postgresRestoreState(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := s.pool.Exec(ctx, `CREATE FUNCTION session_pg_cleanup_reject_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'synthetic cleanup delete failure'; END $$;
		CREATE TRIGGER session_pg_cleanup_reject_fixture AFTER DELETE ON session_pg_store_fixture
		FOR EACH STATEMENT EXECUTE FUNCTION session_pg_cleanup_reject_fixture()`)
	if err != nil {
		t.Fatal(err)
	}
	drop := `DROP TRIGGER IF EXISTS session_pg_cleanup_reject_fixture ON session_pg_store_fixture;
		DROP FUNCTION IF EXISTS session_pg_cleanup_reject_fixture()`
	t.Cleanup(func() {
		if _, err := s.pool.Exec(context.Background(), drop); err != nil {
			t.Error(err)
		}
	})
	n, err := s.cleanupExpired(ctx, 2)
	var pgErr *pgconn.PgError
	if n != 0 || !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
		t.Fatal("cleanup did not reach the AFTER DELETE statement failure", n, err)
	}
	assertPostgresCleanup(t, s, before)
	if _, err := s.pool.Exec(ctx, drop); err != nil {
		t.Fatal(err)
	}
	if n, err := s.cleanupExpired(ctx, 2); err != nil || n != 2 {
		t.Fatal("cleanup did not recover after definite rollback", n, err)
	}
	assertPostgresCleanup(t, s, before, "one", "two")
}

func TestPostgresCleanupTableLockDeadline(t *testing.T) {
	f := setupPostgresSessionOptions(t, true, true)
	s := f.stores[0]
	cutoff := postgresCleanupClock(t, s)
	seedPostgresCleanup(t, s, postgresCleanupRow{"locked", cutoff, cutoff, nil})
	before := postgresRestoreState(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer postgresRollback(tx)
	if _, err := tx.Exec(ctx, `LOCK TABLE session_pg_store_fixture IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	var holder int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	request, stop := context.WithTimeout(ctx, time.Second)
	defer stop()
	type result struct {
		n   int64
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := f.stores[1].cleanupExpired(request, 1)
		done <- result{n, err}
	}()
	postgresBlockedPID(t, ctx, s.pool, holder)
	select {
	case got := <-done:
		if got.n != 0 || !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatal("table-lock wait did not obey caller deadline", got.n, got.err)
		}
	case <-ctx.Done():
		t.Fatal("cleanup did not finish after its caller deadline")
	}
	postgresRollback(tx)
	assertPostgresCleanup(t, s, before)
	if n, err := s.cleanupExpired(ctx, 1); err != nil || n != 1 {
		t.Fatal("cleanup unavailable after lock timeout", n, err)
	}
	assertPostgresCleanup(t, s, before, "locked")
}
