package sessionexperiment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func rejectWithoutCookie(t *testing.T, f *fixture, replica int, method, path string, cookie *http.Cookie, status int) {
	t.Helper()
	if len(expect(t, f, replica, method, path, cookie, status).Cookies()) != 0 {
		t.Fatal("rejected request sent cookie")
	}
}

func TestAccountGenerationAcrossAPIs(t *testing.T) {
	f := setupAuth(t)
	initial, err := f.stores[0].auth.current(context.Background(), "fixture-user")
	if err != nil {
		t.Fatal(err)
	}
	var old []*http.Cookie
	for i := range 3 {
		old = append(old, expect(t, f, i%2, "POST", "/login-fixture", nil, 201).Cookies()[0])
	}
	invalidate(t, f.stores[1].auth, true)
	for replica := range 2 {
		for _, cookie := range old {
			rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
		}
	}
	// Fresh DB connections read the persisted generation, with no local cache.
	fresh := newAuthority(t, f.stores[0].auth.namespace)
	generation, err := fresh.current(context.Background(), "fixture-user")
	if err != nil || generation != (authVersion{2, initial.recovery}) {
		t.Fatalf("persisted generation=%+v err=%v", generation, err)
	}
	var current []*http.Cookie
	for i := range 3 {
		current = append(current, expect(t, f, i%2, "POST", "/login-fixture", nil, 201).Cookies()[0])
	}
	rejectWithoutCookie(t, f, 0, "POST", "/login-fixture", nil, 409)
	invalidate(t, fresh, false)
	for replica := range 2 {
		for _, cookie := range current {
			rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
		}
		rejectWithoutCookie(t, f, replica, "POST", "/login-fixture", nil, 401)
	}
	// Reactivation must never revive any prior generation.
	invalidate(t, fresh, true)
	for _, cookie := range append(old, current...) {
		rejectWithoutCookie(t, f, 0, "GET", "/me", cookie, 401)
	}
	expect(t, f, 0, "POST", "/login-fixture", nil, 201)
}

func TestAccountGenerationRejectsLoadedSaveAndLogin(t *testing.T) {
	for _, path := range []string{"/late-save", "/late-login-fixture"} {
		t.Run(path, func(t *testing.T) {
			f := setupAuth(t)
			var cookie *http.Cookie
			if path == "/late-save" {
				cookie = expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			}
			finished := make(chan error, 1)
			go func() {
				r, err := request(f, 0, "POST", path, cookie)
				if err == nil && (r.StatusCode != 401 || len(r.Cookies()) != 0) {
					err = fmt.Errorf("late operation status=%d cookies=%d", r.StatusCode, len(r.Cookies()))
				}
				finished <- err
			}()
			select {
			case <-f.loaded:
			case <-time.After(5 * time.Second):
				close(f.resume)
				t.Fatal("load barrier")
			}
			invalidate(t, f.stores[1].auth, true)
			close(f.resume)
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			for i := range 3 {
				expect(t, f, i%2, "POST", "/login-fixture", nil, 201)
			}
			rejectWithoutCookie(t, f, 1, "POST", "/login-fixture", nil, 409)
		})
	}
}

func TestAccountAuthorityFailureIsClosed(t *testing.T) {
	f := setupAuth(t)
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	f.stores[1].auth.pool.Close()
	for _, request := range []struct {
		method, path string
		cookie       *http.Cookie
	}{
		{"GET", "/me", cookie}, {"POST", "/logout", cookie}, {"POST", "/login-fixture", nil},
	} {
		rejectWithoutCookie(t, f, 1, request.method, request.path, request.cookie, 503)
	}
	expect(t, f, 0, "GET", "/me", cookie, 200)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := f.stores[0].auth.pool.Exec(ctx, `DELETE FROM session_auth_fixture WHERE namespace=$1 AND subject=$2`, f.stores[0].auth.namespace, "fixture-user")
	if err != nil {
		t.Fatal(err)
	}
	rejectWithoutCookie(t, f, 0, "GET", "/me", cookie, 401)
	rejectWithoutCookie(t, f, 0, "POST", "/login-fixture", nil, 401)
}

func TestAccountInvalidationWaitsForSaveGuard(t *testing.T) {
	f := setupAuth(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	version, err := f.stores[0].auth.current(ctx, "fixture-user")
	if err != nil {
		t.Fatal(err)
	}
	err = f.stores[0].auth.withValid(ctx, "fixture-user", version, func() error {
		tx, err := f.stores[1].auth.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='100ms'`); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE session_auth_fixture SET generation=generation+1 WHERE namespace=$1 AND subject=$2`, f.stores[1].auth.namespace, "fixture-user")
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
			return fmt.Errorf("expected blocked invalidation, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidate(t, f.stores[1].auth, true)
	err = f.stores[0].auth.withValid(ctx, "fixture-user", version, func() error { t.Error("stale operation executed"); return nil })
	if !errors.Is(err, errStale) {
		t.Fatalf("old generation: %v", err)
	}
}
