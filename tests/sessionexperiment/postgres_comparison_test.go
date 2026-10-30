package sessionexperiment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgconn"
)

func postgresPayload(t *testing.T, subject string, deadline time.Time) []byte {
	t.Helper()
	b, err := (scs.GobCodec{}).Encode(deadline, map[string]interface{}{
		"subject": subject, "remember_login": false, "__rememberMe": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func postgresRows(t *testing.T, s *postgresSessionStore, active bool) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM session_pg_store_fixture
		WHERE namespace=$1 AND (NOT $2::boolean OR (data IS NOT NULL AND expiry_ms>floor(extract(epoch FROM clock_timestamp())*1000)::bigint))`, s.namespace, active).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func postgresSnapshot(t *testing.T, s *postgresSessionStore) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var b []byte
	err := s.pool.QueryRow(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY token),'[]'::jsonb)
		FROM (SELECT token,subject,encode(data,'hex') AS data,deadline_ms,expiry_ms
		FROM session_pg_store_fixture WHERE namespace=$1) r`, s.namespace).Scan(&b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPostgresSessionConcurrentAdmission(t *testing.T) {
	f := setupPostgresSessions(t)
	start := make(chan struct{})
	results := make(chan *http.Response, 24)
	errorsCh := make(chan error, 24)
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Go(func() {
			<-start
			r, err := request(f.api, i%2, "POST", "/login-fixture", nil)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- r
		})
	}
	close(start)
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		t.Error(err)
	}
	var cookies []*http.Cookie
	conflicts := 0
	for r := range results {
		switch r.StatusCode {
		case 201:
			if len(r.Cookies()) != 1 {
				t.Fatal("successful admission must send one cookie")
			}
			c := r.Cookies()[0]
			if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || !c.Expires.IsZero() || c.MaxAge != 0 {
				t.Fatal("ordinary cookie policy changed")
			}
			cookies = append(cookies, c)
		case 409:
			conflicts++
			if len(r.Header.Values("Set-Cookie")) != 0 {
				t.Fatal("rejected admission sent a cookie")
			}
		default:
			t.Errorf("unexpected admission status %d", r.StatusCode)
		}
	}
	if len(cookies) != 3 || conflicts != 21 || postgresRows(t, f.stores[0], true) != 3 {
		t.Fatalf("admitted=%d rejected=%d", len(cookies), conflicts)
	}
	for _, c := range cookies {
		for i := range 2 {
			r := expect(t, f.api, i, "GET", "/me", c, 200)
			if len(r.Header.Values("Set-Cookie")) != 0 {
				t.Fatal("read renewed session")
			}
		}
	}
	expect(t, f.api, 1, "POST", "/logout", cookies[0], 204)
	for i := range 2 {
		expect(t, f.api, i, "GET", "/me", cookies[0], 401)
	}
	expect(t, f.api, 0, "POST", "/login-fixture", nil, 201)
	expect(t, f.api, 1, "POST", "/login-fixture", nil, 409)
	if postgresRows(t, f.stores[1], true) != 3 || postgresRows(t, f.stores[1], false) != 4 {
		t.Fatal("revocation did not free exactly one slot while retaining its tombstone")
	}
}

func TestPostgresSessionRevokedLoadedSave(t *testing.T) {
	f := setupPostgresSessions(t)
	cookie := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	finished := make(chan error, 1)
	var resume sync.Once
	t.Cleanup(func() { resume.Do(func() { close(f.api.resume) }) })
	go func() {
		r, err := request(f.api, 0, "POST", "/late-save", cookie)
		if err == nil && (r.StatusCode != 401 || len(r.Header.Values("Set-Cookie")) != 0) {
			err = fmt.Errorf("late save: status=%d cookie=%d", r.StatusCode, len(r.Header.Values("Set-Cookie")))
		}
		finished <- err
	}()
	select {
	case <-f.api.loaded:
	case <-time.After(4 * time.Second):
		t.Fatal("late save never loaded")
	}
	expect(t, f.api, 1, "POST", "/logout", cookie, 204)
	for range 3 {
		expect(t, f.api, 1, "POST", "/login-fixture", nil, 201)
	}
	resume.Do(func() { close(f.api.resume) })
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	expect(t, f.api, 0, "POST", "/login-fixture", nil, 409)
	if postgresRows(t, f.stores[1], true) != 3 || postgresRows(t, f.stores[1], false) != 4 {
		t.Fatal("late save recreated or occupied a slot")
	}
}

func TestPostgresSessionRepeatedSaveAndImmutableIdentity(t *testing.T) {
	f := setupPostgresSessions(t)
	s := f.stores[0]
	ctx := context.Background()
	// Deliberately not aligned to PostgreSQL's microsecond timestamp precision.
	deadline := time.Now().Truncate(time.Millisecond).Add(time.Hour + 123*time.Nanosecond)
	b := postgresPayload(t, "fixture-user", deadline)
	for i := range 12 {
		if err := f.stores[i%2].CommitCtx(ctx, "repeat-token", b, time.Now().Add(time.Duration(i+1)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	before := postgresSnapshot(t, s)
	for _, candidate := range []struct {
		subject  string
		deadline time.Time
	}{{"other-user", deadline}, {"fixture-user", deadline.Add(time.Minute)}} {
		err := s.CommitCtx(ctx, "repeat-token", postgresPayload(t, candidate.subject, candidate.deadline), time.Now().Add(time.Minute))
		if !errors.Is(err, errStale) {
			t.Fatal("changed token identity or deadline accepted", err)
		}
		if !bytes.Equal(before, postgresSnapshot(t, s)) {
			t.Fatal("rejected identity change mutated authoritative state")
		}
	}
	for _, store := range f.stores {
		got, found, err := store.FindCtx(ctx, "repeat-token")
		if err != nil || !found || !bytes.Equal(got, b) {
			t.Fatal("save/read changed original payload", err)
		}
	}
	var storedDeadline, storedExpiry int64
	var token string
	if err := s.pool.QueryRow(ctx, `SELECT token,deadline_ms,expiry_ms FROM session_pg_store_fixture WHERE namespace=$1`, s.namespace).Scan(&token, &storedDeadline, &storedExpiry); err != nil {
		t.Fatal(err)
	}
	if storedDeadline != deadline.UnixMilli() || storedExpiry <= time.Now().Add(11*time.Minute).UnixMilli() || token != digest("repeat-token") || postgresRows(t, s, true) != 1 {
		t.Fatal("repeated save altered deadline, token digest or slot count")
	}
	var subjects int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM session_pg_subject_fixture WHERE namespace=$1`, s.namespace).Scan(&subjects); err != nil || subjects != 1 {
		t.Fatal("rejected subject change left registry state")
	}
}

func TestPostgresSessionRejectsOtherProfilesAndInvalidExpiry(t *testing.T) {
	f := setupPostgresSessions(t)
	s := f.stores[0]
	now := time.Now()
	for _, kind := range []string{"remember", "persist", "scope", "too-long", "past", "beyond-deadline", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			deadline, expiry := now.Add(time.Hour), now.Add(time.Minute)
			values := map[string]interface{}{"subject": "fixture-user", "remember_login": false, "__rememberMe": false}
			switch kind {
			case "remember":
				values["remember_login"] = true
			case "persist":
				values["__rememberMe"] = true
			case "scope":
				values["session_scope"] = "admin"
			case "too-long":
				deadline = now.Add(13 * time.Hour)
			case "past":
				expiry = now.Add(-time.Second)
			case "beyond-deadline":
				expiry = deadline.Add(time.Minute)
			}
			b, err := (scs.GobCodec{}).Encode(deadline, values)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "malformed" {
				b = []byte("not a session")
			}
			if err := s.CommitCtx(context.Background(), "rejected-token", b, expiry); err == nil || (kind != "malformed" && !errors.Is(err, errStale)) {
				t.Fatal("expected policy rejection, not infrastructure failure", err)
			}
			if postgresRows(t, s, false) != 0 {
				t.Fatal("rejection left a session row")
			}
		})
	}
	if err := s.DeleteCtx(context.Background(), "unknown-token"); err != nil || postgresRows(t, s, false) != 0 {
		t.Fatal("unknown deletion must be idempotent without writes", err)
	}
	var subjects int
	if err := s.pool.QueryRow(context.Background(), `SELECT count(*) FROM session_pg_subject_fixture WHERE namespace=$1`, s.namespace).Scan(&subjects); err != nil || subjects != 0 {
		t.Fatal("invalid admission or unknown deletion left registry state", err)
	}
}

func TestPostgresSessionRejectsMisplacedPayload(t *testing.T) {
	f := setupPostgresSessions(t)
	s := f.stores[0]
	ctx := context.Background()
	cookie := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	original, found, err := s.FindCtx(ctx, cookie.Value)
	if err != nil || !found {
		t.Fatal("initial session missing", err)
	}
	for _, kind := range []string{"subject", "deadline", "scope", "remember"} {
		t.Run(kind, func(t *testing.T) {
			deadline, values, err := (scs.GobCodec{}).Decode(original)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "subject":
				values["subject"] = "other-user"
			case "deadline":
				deadline = deadline.Add(time.Minute)
			case "scope":
				values["session_scope"] = "admin"
			case "remember":
				values["remember_login"] = true
			}
			b, err := (scs.GobCodec{}).Encode(deadline, values)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.pool.Exec(ctx, `UPDATE session_pg_store_fixture SET data=$3 WHERE namespace=$1 AND token=$2`, s.namespace, digest(cookie.Value), b); err != nil {
				t.Fatal(err)
			}
			snapshot := postgresSnapshot(t, s)
			for i, store := range f.stores {
				if _, found, err := store.FindCtx(ctx, cookie.Value); found || !errors.Is(err, errStale) {
					t.Fatal("misplaced payload returned as authenticated", err)
				}
				r := expect(t, f.api, i, "GET", "/me", cookie, 401)
				if len(r.Header.Values("Set-Cookie")) != 0 || !bytes.Equal(snapshot, postgresSnapshot(t, s)) {
					t.Fatal("rejected read changed cookie or authoritative state")
				}
			}
		})
	}
}

func TestPostgresSessionExpiryCannotResurrect(t *testing.T) {
	for _, kind := range []string{"idle", "absolute"} {
		t.Run(kind, func(t *testing.T) {
			f := setupPostgresSessions(t)
			s := f.stores[0]
			ctx := context.Background()
			expiry := time.Now().Add(350 * time.Millisecond)
			deadline := expiry.Add(time.Hour)
			if kind == "absolute" {
				deadline = expiry
			}
			b := postgresPayload(t, "fixture-user", deadline)
			if err := s.CommitCtx(ctx, "expired-token", b, expiry); err != nil {
				t.Fatal(err)
			}
			if got, found, err := f.stores[1].FindCtx(ctx, "expired-token"); err != nil || !found || !bytes.Equal(got, b) {
				t.Fatal("new session was not readable before real expiry", err)
			}
			for until := time.Now().Add(2 * time.Second); ; {
				_, found, err := f.stores[1].FindCtx(ctx, "expired-token")
				if err != nil {
					t.Fatal(err)
				}
				if !found {
					break
				}
				if time.Now().After(until) {
					t.Fatal("real database expiry not observed")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := s.CommitCtx(ctx, "expired-token", b, time.Now().Add(time.Minute)); !errors.Is(err, errStale) {
				t.Fatal("expired loaded save was accepted", err)
			}
			for i := range 4 {
				data := postgresPayload(t, "fixture-user", time.Now().Add(time.Hour))
				err := f.stores[i%2].CommitCtx(ctx, fmt.Sprintf("replacement-%d", i), data, time.Now().Add(time.Minute))
				if (i < 3 && err != nil) || (i == 3 && !errors.Is(err, errLimit)) {
					t.Fatal("expiry slot accounting", err)
				}
			}
			if postgresRows(t, s, true) != 3 || postgresRows(t, s, false) != 4 {
				t.Fatal("expiry deleted tombstone or leaked slot")
			}
		})
	}
}

func TestPostgresSessionWriteFailureRollsBack(t *testing.T) {
	f := setupPostgresSessions(t)
	s := f.stores[0]
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `CREATE FUNCTION session_pg_reject_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'synthetic write failure'; END $$;
		CREATE TRIGGER session_pg_reject_fixture AFTER INSERT OR UPDATE ON session_pg_store_fixture
		FOR EACH ROW EXECUTE FUNCTION session_pg_reject_fixture()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := s.pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS session_pg_reject_fixture ON session_pg_store_fixture; DROP FUNCTION IF EXISTS session_pg_reject_fixture()`)
		if err != nil {
			t.Error(err)
		}
	})
	err = s.CommitCtx(ctx, "failed-write-token", postgresPayload(t, "fixture-user", time.Now().Add(time.Hour)), time.Now().Add(time.Minute))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
		t.Fatal("write did not reach the synthetic AFTER trigger", err)
	}
	r := expect(t, f.api, 1, "POST", "/login-fixture", nil, 503)
	var subjects int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM session_pg_subject_fixture WHERE namespace=$1`, s.namespace).Scan(&subjects); err != nil {
		t.Fatal(err)
	}
	if len(r.Header.Values("Set-Cookie")) != 0 || subjects != 0 || postgresRows(t, s, false) != 0 {
		t.Fatal("write failure left cookie, registry or session row")
	}
	if _, err := s.pool.Exec(ctx, `DROP TRIGGER session_pg_reject_fixture ON session_pg_store_fixture; DROP FUNCTION session_pg_reject_fixture()`); err != nil {
		t.Fatal(err)
	}
	expect(t, f.api, 1, "POST", "/login-fixture", nil, 201)
}

func TestPostgresSessionPoolFailureIsNotLogout(t *testing.T) {
	f := setupPostgresSessions(t)
	cookie := expect(t, f.api, 1, "POST", "/login-fixture", nil, 201).Cookies()[0]
	before, found, err := f.stores[1].FindCtx(context.Background(), cookie.Value)
	if err != nil || !found {
		t.Fatal("missing session", err)
	}
	snapshot := postgresSnapshot(t, f.stores[1])
	f.stores[0].pool.Close()
	for _, path := range []string{"/me", "/activity-fixture", "/logout", "/login-fixture"} {
		method, c := "POST", cookie
		switch path {
		case "/me":
			method = "GET"
		case "/login-fixture":
			c = nil
		}
		r := expect(t, f.api, 0, method, path, c, 503)
		if len(r.Header.Values("Set-Cookie")) != 0 {
			t.Fatal("database failure sent or cleared cookie")
		}
	}
	after, found, err := f.stores[1].FindCtx(context.Background(), cookie.Value)
	if err != nil || !found || !bytes.Equal(before, after) || !bytes.Equal(snapshot, postgresSnapshot(t, f.stores[1])) || postgresRows(t, f.stores[1], true) != 1 {
		t.Fatal("failed replica changed authoritative session", err)
	}
	expect(t, f.api, 1, "GET", "/me", cookie, 200)
}

func TestPostgresSessionRegistryLockRechecksRevocation(t *testing.T) {
	f := setupPostgresSessions(t)
	s := f.stores[0]
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	b := postgresPayload(t, "fixture-user", time.Now().Add(time.Hour))
	if err := s.CommitCtx(ctx, "locked-token", b, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	var holder int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid() FROM session_pg_subject_fixture
		WHERE namespace=$1 AND subject=$2 FOR UPDATE`, s.namespace, digest("fixture-user")).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- f.stores[1].CommitCtx(ctx, "locked-token", b, time.Now().Add(2*time.Minute)) }()
	for {
		var waiting int
		err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type='Lock' AND $1=ANY(pg_blocking_pids(pid))`, holder).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting == 1 {
			break
		}
		select {
		case err := <-finished:
			t.Fatal("save did not wait for registry lock", err)
		case <-ctx.Done():
			t.Fatal("lock wait not observed")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE session_pg_store_fixture SET data=NULL WHERE namespace=$1 AND token=$2`, s.namespace, digest("locked-token")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; !errors.Is(err, errStale) {
		t.Fatal("waiting save did not recheck committed revocation", err)
	}
}

func TestPostgresSessionAdmissionQueryPlan(t *testing.T) {
	f := setupPostgresSessions(t)
	s := f.stores[0]
	ctx := context.Background()
	for i := range 3 {
		b := postgresPayload(t, "fixture-user", time.Now().Add(time.Hour))
		if err := s.CommitCtx(ctx, fmt.Sprintf("plan-token-%d", i), b, time.Now().Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	// Synthetic unrelated subjects make the subject/range access path visible;
	// this is a query-plan check, not a representative production load.
	_, err := s.pool.Exec(ctx, `INSERT INTO session_pg_subject_fixture
		SELECT $1,md5(i::text) FROM generate_series(1,2000) i`, s.namespace)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO session_pg_store_fixture
		SELECT $1,md5(i::text),md5(i::text),decode('01','hex'),$2::bigint,$3::bigint FROM generate_series(1,2000) i`,
		s.namespace, time.Now().Add(2*time.Hour).UnixMilli(), time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `ANALYZE session_pg_store_fixture`); err != nil {
		t.Fatal(err)
	}
	var plan []byte
	err = s.pool.QueryRow(ctx, `EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) SELECT count(*) FROM session_pg_store_fixture
		WHERE namespace=$1 AND subject=$2 AND data IS NOT NULL AND expiry_ms>$3`,
		s.namespace, digest("fixture-user"), time.Now().UnixMilli()).Scan(&plan)
	if err != nil || !json.Valid(plan) || !bytes.Contains(plan, []byte(`"Index Name": "session_pg_active_fixture"`)) {
		t.Fatal("expected bounded subject index access", err, string(plan))
	}
	t.Log("admission count EXPLAIN ANALYZE over 2000 unrelated rows:", string(plan))
}
