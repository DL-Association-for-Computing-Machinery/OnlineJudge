package sessionexperiment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

func postgresRecoveryState(t *testing.T, s *postgresSessionStore) (string, bool) {
	t.Helper()
	var generation string
	var enabled bool
	if err := s.pool.QueryRow(context.Background(), `SELECT generation,enabled FROM session_pg_recovery_fixture WHERE namespace=$1`, s.namespace).Scan(&generation, &enabled); err != nil {
		t.Fatal(err)
	}
	return generation, enabled
}

func postgresRecoveryPayload(t *testing.T, s *postgresSessionStore, subject string) []byte {
	t.Helper()
	generation, err := s.currentAccount(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := s.currentRecovery(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := (scs.GobCodec{}).Encode(time.Now().Add(time.Hour), map[string]interface{}{
		"subject": subject, "remember_login": false, "__rememberMe": false,
		"auth_generation": generation, "recovery_generation": recovery,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPostgresRecoveryLifecycle(t *testing.T) {
	f := setupPostgresSessionOptions(t, true, true)
	s := f.stores[0]
	ctx := context.Background()
	initial, _ := postgresRecoveryState(t, s)
	var old []*http.Cookie
	for i := range 3 {
		old = append(old, expect(t, f.api, i%2, "POST", "/login-fixture", nil, 201).Cookies()[0])
	}
	// A second subject in the same namespace is also invalidated.
	if _, err := s.pool.Exec(ctx, `INSERT INTO session_pg_account_fixture VALUES($1,$2,1,true)`, s.namespace, digest("other-user")); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitCtx(ctx, "other-token", postgresRecoveryPayload(t, s, "other-user"), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	accounts := postgresAccountsSnapshot(t, s)
	original := postgresSnapshot(t, s)
	generation, err := f.stores[1].beginRecovery(ctx)
	if err != nil || len(generation) < 26 || generation == initial {
		t.Fatal("recovery did not get a fresh ID", err)
	}
	if got, open := postgresRecoveryState(t, s); got != generation || open || postgresRows(t, s, true) != 0 || postgresRows(t, s, false) != 4 {
		t.Fatal("recovery failed to close and preserve all tombstones")
	}
	before := postgresSnapshot(t, s)
	var priorRows, clearedRows []map[string]interface{}
	if err := json.Unmarshal(original, &priorRows); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(before, &clearedRows); err != nil {
		t.Fatal(err)
	}
	for _, row := range priorRows {
		row["data"] = nil
	}
	if !reflect.DeepEqual(priorRows, clearedRows) {
		t.Fatal("recovery changed fields beyond clearing every payload")
	}
	for i := range 2 {
		for _, token := range []string{old[0].Value, "unknown"} {
			if _, found, err := f.stores[i].FindCtx(ctx, token); found || !errors.Is(err, errRecovering) {
				t.Fatal("closed store lookup failed open", err)
			}
		}
		for _, cookie := range []*http.Cookie{old[0], {Name: old[0].Name, Value: "unknown"}, nil} {
			rejectWithoutCookie(t, f.api, i, "GET", "/me", cookie, 503)
		}
		for _, path := range []string{"/login-fixture", "/activity-fixture", "/logout"} {
			cookie := old[0]
			if path == "/login-fixture" {
				cookie = nil
			}
			rejectWithoutCookie(t, f.api, i, "POST", path, cookie, 503)
		}
	}
	if err := s.invalidateAccount(ctx, "fixture-user", false); !errors.Is(err, errRecovering) || !bytes.Equal(accounts, postgresAccountsSnapshot(t, s)) {
		t.Fatal("account operation crossed a closed recovery gate", err)
	}
	if !bytes.Equal(before, postgresSnapshot(t, s)) {
		t.Fatal("closed requests changed tombstones")
	}
	if err := s.finishRecovery(ctx, initial); !errors.Is(err, errRecovering) {
		t.Fatal("stale finish opened gate", err)
	}
	if err := s.finishRecovery(ctx, generation); err != nil {
		t.Fatal(err)
	}
	if err := s.finishRecovery(ctx, generation); !errors.Is(err, errRecovering) {
		t.Fatal("duplicate finish reported success", err)
	}
	for i := range 2 {
		for _, cookie := range old {
			rejectWithoutCookie(t, f.api, i, "GET", "/me", cookie, 401)
		}
	}
	for i := range 3 {
		expect(t, f.api, i%2, "POST", "/login-fixture", nil, 201)
	}
	rejectWithoutCookie(t, f.api, 1, "POST", "/login-fixture", nil, 409)
	fresh := postgresSnapshot(t, s)
	if err := s.DeleteCtx(ctx, old[0].Value); err != nil || !bytes.Equal(fresh, postgresSnapshot(t, s)) {
		t.Fatal("old deletion affected fresh sessions", err)
	}
	if !bytes.Equal(accounts, postgresAccountsSnapshot(t, s)) {
		t.Fatal("global recovery changed account generations")
	}
	// An older completion cannot open a newer still-closed recovery.
	newer, err := s.beginRecovery(ctx)
	if err != nil || newer == generation {
		t.Fatal(err)
	}
	if err := s.finishRecovery(ctx, generation); !errors.Is(err, errRecovering) {
		t.Fatal("prior recovery completion accepted", err)
	}
	if got, open := postgresRecoveryState(t, s); got != newer || open {
		t.Fatal("prior completion changed current recovery")
	}
}

func TestPostgresRecoveryLookupAuthorization(t *testing.T) {
	for _, scenario := range []string{"missing-account", "disabled-account", "new-account-generation", "unknown-token", "expired-token"} {
		t.Run(scenario, func(t *testing.T) {
			f := setupPostgresSessionOptions(t, true, true)
			s := f.stores[0]
			ctx := context.Background()
			cookie := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			for i := range 2 {
				expect(t, f.api, i, "GET", "/me", cookie, 200)
			}
			var err error
			wantStale := true
			switch scenario {
			case "missing-account":
				_, err = s.pool.Exec(ctx, `DELETE FROM session_pg_account_fixture WHERE namespace=$1`, s.namespace)
			case "disabled-account":
				_, err = s.pool.Exec(ctx, `UPDATE session_pg_account_fixture SET enabled=false WHERE namespace=$1`, s.namespace)
			case "new-account-generation":
				_, err = s.pool.Exec(ctx, `UPDATE session_pg_account_fixture SET generation=generation+1 WHERE namespace=$1`, s.namespace)
			case "unknown-token":
				cookie.Value = "unknown"
				wantStale = false
			case "expired-token":
				_, err = s.pool.Exec(ctx, `UPDATE session_pg_store_fixture SET expiry_ms=1 WHERE namespace=$1`, s.namespace)
				wantStale = false
			}
			if err != nil {
				t.Fatal(err)
			}
			before, accounts := postgresSnapshot(t, s), postgresAccountsSnapshot(t, s)
			for i, store := range f.stores {
				_, found, err := store.FindCtx(ctx, cookie.Value)
				if found || (wantStale && !errors.Is(err, errStale)) || (!wantStale && err != nil) {
					t.Fatal("lookup did not enforce joined authorization", err)
				}
				rejectWithoutCookie(t, f.api, i, "GET", "/me", cookie, 401)
			}
			if !bytes.Equal(before, postgresSnapshot(t, s)) || !bytes.Equal(accounts, postgresAccountsSnapshot(t, s)) {
				t.Fatal("read changed authority or session state")
			}
		})
	}
}

func TestPostgresRecoveryRejectsLoadedRequests(t *testing.T) {
	for _, path := range []string{"/late-save", "/late-login-fixture"} {
		t.Run(path, func(t *testing.T) {
			f := setupPostgresSessionOptions(t, true, true)
			s := f.stores[0]
			ctx := context.Background()
			var cookie *http.Cookie
			if path == "/late-save" {
				cookie = expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			}
			finished := make(chan error, 1)
			done := make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(f.api.resume) }) }
			go func() {
				defer close(done)
				r, err := request(f.api, 0, "POST", path, cookie)
				if err == nil && (r.StatusCode != 401 || len(r.Header.Values("Set-Cookie")) != 0) {
					err = fmt.Errorf("stale operation status=%d", r.StatusCode)
				}
				finished <- err
			}()
			defer func() { release(); <-done }()
			select {
			case <-f.api.loaded:
			case <-time.After(time.Second):
				t.Fatal("load barrier not reached")
			}
			generation, err := s.beginRecovery(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.finishRecovery(ctx, generation); err != nil {
				t.Fatal(err)
			}
			for i := range 3 {
				expect(t, f.api, i%2, "POST", "/login-fixture", nil, 201)
			}
			before := postgresSnapshot(t, s)
			release()
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, postgresSnapshot(t, s)) {
				t.Fatal("late request modified fresh slots")
			}
			rejectWithoutCookie(t, f.api, 1, "POST", "/login-fixture", nil, 409)
		})
	}
}

func TestPostgresRecoveryPayloadGuards(t *testing.T) {
	for _, value := range []interface{}{nil, int64(1), "", strings.Repeat("x", 26)} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			f := setupPostgresSessionOptions(t, true, true)
			s := f.stores[0]
			ctx := context.Background()
			cookie := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			original, _, err := s.FindCtx(ctx, cookie.Value)
			if err != nil {
				t.Fatal(err)
			}
			deadline, values, err := (scs.GobCodec{}).Decode(original)
			if err != nil {
				t.Fatal(err)
			}
			values["recovery_generation"] = value
			if value == nil {
				delete(values, "recovery_generation")
			}
			bad, err := (scs.GobCodec{}).Encode(deadline, values)
			if err != nil {
				t.Fatal(err)
			}
			before := postgresSnapshot(t, s)
			for _, token := range []string{cookie.Value, "new-candidate"} {
				if err := s.CommitCtx(ctx, token, bad, time.Now().Add(time.Minute)); !errors.Is(err, errStale) || !bytes.Equal(before, postgresSnapshot(t, s)) {
					t.Fatal("bad incoming generation changed state", err)
				}
			}
			if _, err := s.pool.Exec(ctx, `UPDATE session_pg_store_fixture SET data=$3 WHERE namespace=$1 AND token=$2`, s.namespace, digest(cookie.Value), bad); err != nil {
				t.Fatal(err)
			}
			before = postgresSnapshot(t, s)
			for i, store := range f.stores {
				if _, found, err := store.FindCtx(ctx, cookie.Value); found || !errors.Is(err, errStale) {
					t.Fatal("stored bad recovery accepted", err)
				}
				rejectWithoutCookie(t, f.api, i, "GET", "/me", cookie, 401)
			}
			if err := s.CommitCtx(ctx, cookie.Value, original, time.Now().Add(time.Minute)); !errors.Is(err, errStale) || !bytes.Equal(before, postgresSnapshot(t, s)) {
				t.Fatal("save repaired mismatched stored recovery", err)
			}
		})
	}
}
