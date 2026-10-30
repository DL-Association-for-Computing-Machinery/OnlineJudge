package sessionexperiment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

func postgresAccountState(t *testing.T, s *postgresSessionStore) (int64, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var generation int64
	var enabled bool
	err := s.pool.QueryRow(ctx, `SELECT generation,enabled FROM session_pg_account_fixture
		WHERE namespace=$1 AND subject=$2`, s.namespace, digest("fixture-user")).Scan(&generation, &enabled)
	if err != nil {
		t.Fatal(err)
	}
	return generation, enabled
}

func postgresAccountPayload(t *testing.T, subject string, generation int64) []byte {
	t.Helper()
	b, err := (scs.GobCodec{}).Encode(time.Now().Add(time.Hour), map[string]interface{}{
		"subject": subject, "remember_login": false, "__rememberMe": false, "auth_generation": generation,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPostgresAccountInvalidationAcrossAPIs(t *testing.T) {
	f := setupPostgresSessions(t, true)
	ctx := context.Background()
	var old []*http.Cookie
	expectedGeneration := int64(1)
	for cycle := range 3 {
		for range 3 {
			cookie := expect(t, f.api, cycle%2, "POST", "/login-fixture", nil, 201).Cookies()[0]
			old = append(old, cookie)
			for i := range 2 {
				expect(t, f.api, i, "GET", "/me", cookie, 200)
			}
		}
		expect(t, f.api, 1, "POST", "/login-fixture", nil, 409)
		if err := f.stores[1].invalidateAccount(ctx, "fixture-user", cycle != 1); err != nil {
			t.Fatal(err)
		}
		expectedGeneration++
		generation, enabled := postgresAccountState(t, f.stores[0])
		if generation != expectedGeneration || enabled != (cycle != 1) || postgresRows(t, f.stores[0], true) != 0 {
			t.Fatal("account change did not invalidate all slots")
		}
		for _, cookie := range old {
			for i := range 2 {
				r := expect(t, f.api, i, "GET", "/me", cookie, 401)
				if len(r.Header.Values("Set-Cookie")) != 0 {
					t.Fatal("rejected old cookie was replaced")
				}
			}
		}
		if cycle == 1 {
			before := postgresSnapshot(t, f.stores[0])
			for i := range 2 {
				r := expect(t, f.api, i, "POST", "/login-fixture", nil, 401)
				if len(r.Header.Values("Set-Cookie")) != 0 || !bytes.Equal(before, postgresSnapshot(t, f.stores[0])) {
					t.Fatal("disabled account admitted or changed sessions")
				}
			}
			if err := f.stores[0].invalidateAccount(ctx, "fixture-user", true); err != nil {
				t.Fatal(err)
			}
			expectedGeneration++
			for _, cookie := range old {
				for i := range 2 {
					expect(t, f.api, i, "GET", "/me", cookie, 401)
				}
			}
		}
	}
}

func TestPostgresAccountRejectsLoadedSaveAndLogin(t *testing.T) {
	for _, path := range []string{"/late-save", "/late-login-fixture"} {
		t.Run(path, func(t *testing.T) {
			f := setupPostgresSessions(t, true)
			var cookie *http.Cookie
			if path == "/late-save" {
				cookie = expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			}
			finished := make(chan error, 1)
			var resume sync.Once
			t.Cleanup(func() { resume.Do(func() { close(f.api.resume) }) })
			go func() {
				r, err := request(f.api, 0, "POST", path, cookie)
				if err == nil && (r.StatusCode != 401 || len(r.Header.Values("Set-Cookie")) != 0) {
					err = fmt.Errorf("stale request returned %d or cookie", r.StatusCode)
				}
				finished <- err
			}()
			select {
			case <-f.api.loaded:
			case <-time.After(4 * time.Second):
				t.Fatal("request never reached loaded barrier")
			}
			// Cross a complete disable/re-enable window; the account is usable
			// again, but the request must retain its originally captured generation.
			for _, enabled := range []bool{false, true} {
				if err := f.stores[1].invalidateAccount(context.Background(), "fixture-user", enabled); err != nil {
					t.Fatal(err)
				}
			}
			for range 3 {
				expect(t, f.api, 1, "POST", "/login-fixture", nil, 201)
			}
			before := postgresSnapshot(t, f.stores[1])
			resume.Do(func() { close(f.api.resume) })
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			if cookie != nil {
				if err := f.stores[0].DeleteCtx(context.Background(), cookie.Value); err != nil {
					t.Fatal(err)
				}
			}
			if !bytes.Equal(before, postgresSnapshot(t, f.stores[1])) || postgresRows(t, f.stores[1], true) != 3 {
				t.Fatal("old save/login/delete changed new generation sessions")
			}
			expect(t, f.api, 0, "POST", "/login-fixture", nil, 409)
		})
	}
}

func TestPostgresAccountRejectsInvalidGenerations(t *testing.T) {
	for _, kind := range []string{"missing", "wrong-type", "zero", "negative", "future", "old", "disabled", "absent-account"} {
		t.Run(kind, func(t *testing.T) {
			f := setupPostgresSessions(t, true)
			s := f.stores[0]
			ctx := context.Background()
			cookie := expect(t, f.api, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			b := postgresAccountPayload(t, "fixture-user", 1)
			deadline, values, err := (scs.GobCodec{}).Decode(b)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing":
				delete(values, "auth_generation")
			case "wrong-type":
				values["auth_generation"] = "1"
			case "zero":
				values["auth_generation"] = int64(0)
			case "negative":
				values["auth_generation"] = int64(-1)
			case "future":
				values["auth_generation"] = int64(2)
			case "old":
				if err := s.invalidateAccount(ctx, "fixture-user", true); err != nil {
					t.Fatal(err)
				}
			case "disabled":
				// Deliberately retain both generation 1 and its live session:
				// rejection must depend on enabled, not a different generation
				// or the tombstone produced by normal invalidation.
				if _, err := s.pool.Exec(ctx, `UPDATE session_pg_account_fixture SET enabled=false WHERE namespace=$1`, s.namespace); err != nil {
					t.Fatal(err)
				}
			case "absent-account":
				if _, err := s.pool.Exec(ctx, `DELETE FROM session_pg_account_fixture WHERE namespace=$1`, s.namespace); err != nil {
					t.Fatal(err)
				}
			}
			b, err = (scs.GobCodec{}).Encode(deadline, values)
			if err != nil {
				t.Fatal(err)
			}
			before := postgresSnapshot(t, s)
			accounts := postgresAccountsSnapshot(t, s)
			if err := f.stores[1].CommitCtx(ctx, "invalid-generation-token", b, time.Now().Add(time.Minute)); !errors.Is(err, errStale) {
				t.Fatal("expected stale generation rejection", err)
			}
			if !bytes.Equal(before, postgresSnapshot(t, s)) || !bytes.Equal(accounts, postgresAccountsSnapshot(t, s)) {
				t.Fatal("rejected generation mutated sessions or accounts")
			}
			if kind == "disabled" || kind == "absent-account" {
				for i, store := range f.stores {
					if _, found, err := store.FindCtx(ctx, cookie.Value); err != nil || found {
						t.Fatal("unavailable account still authorized its otherwise live session", err)
					}
					r := expect(t, f.api, i, "GET", "/me", cookie, 401)
					if len(r.Header.Values("Set-Cookie")) != 0 || !bytes.Equal(before, postgresSnapshot(t, s)) || !bytes.Equal(accounts, postgresAccountsSnapshot(t, s)) {
						t.Fatal("account-guard rejection changed cookie or state")
					}
				}
			}
		})
	}
}

func TestPostgresAccountRejectsStoredGenerationMismatch(t *testing.T) {
	for _, generation := range []interface{}{nil, "1", int64(0), int64(2)} {
		t.Run(fmt.Sprint(generation), func(t *testing.T) {
			f := setupPostgresSessions(t, true)
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
			values["auth_generation"] = generation
			if generation == nil {
				delete(values, "auth_generation")
			}
			b, err := (scs.GobCodec{}).Encode(deadline, values)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.pool.Exec(ctx, `UPDATE session_pg_store_fixture SET data=$3 WHERE namespace=$1 AND token=$2`, s.namespace, digest(cookie.Value), b); err != nil {
				t.Fatal(err)
			}
			before := postgresSnapshot(t, s)
			for i, store := range f.stores {
				if _, found, err := store.FindCtx(ctx, cookie.Value); found || !errors.Is(err, errStale) {
					t.Fatal("stored mismatched generation accepted", err)
				}
				r := expect(t, f.api, i, "GET", "/me", cookie, 401)
				if len(r.Header.Values("Set-Cookie")) != 0 {
					t.Fatal("invalid generation emitted cookie")
				}
			}
			if err := s.CommitCtx(ctx, cookie.Value, original, time.Now().Add(time.Minute)); !errors.Is(err, errStale) || !bytes.Equal(before, postgresSnapshot(t, s)) {
				t.Fatal("save rewrote inconsistent stored generation", err)
			}
		})
	}
}
