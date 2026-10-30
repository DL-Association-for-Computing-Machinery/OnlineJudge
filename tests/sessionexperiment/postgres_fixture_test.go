package sessionexperiment

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

type postgresSessionFixture struct {
	stores [2]*postgresSessionStore
	api    *fixture
}

func setupPostgresSessions(t *testing.T, guarded ...bool) *postgresSessionFixture {
	t.Helper()
	if len(guarded) > 1 {
		t.Fatal("at most one account-guard option is allowed")
	}
	f := &postgresSessionFixture{api: &fixture{loaded: make(chan struct{}, 1), resume: make(chan struct{})}}
	namespace := digest(t.Name())
	for i := range 2 {
		a := newAuthority(t, namespace)
		s := &postgresSessionStore{pool: a.pool, namespace: namespace}
		s.account = len(guarded) == 1 && guarded[0]
		f.stores[i] = s
		if i == 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, err := s.pool.Exec(ctx, postgresSessionSchema)
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			if s.account {
				if _, err := s.pool.Exec(context.Background(), postgresAccountSchema); err != nil {
					t.Fatal(err)
				}
				if _, err := s.pool.Exec(context.Background(), `INSERT INTO session_pg_account_fixture VALUES($1,$2,1,true)`, namespace, digest("fixture-user")); err != nil {
					t.Fatal(err)
				}
			}
		}
		manager := scs.New()
		manager.Store = s
		manager.Lifetime, manager.IdleTimeout = 12*time.Hour, 2*time.Hour
		manager.Cookie.Secure, manager.Cookie.HttpOnly = true, true
		manager.Cookie.SameSite, manager.Cookie.Persist = http.SameSiteStrictMode, false
		f.api.managers[i] = manager
		f.api.servers[i] = httptest.NewTLSServer(sessionOriginProtection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/me" && r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			token := ""
			if c, err := r.Cookie(manager.Cookie.Name); err == nil {
				token = c.Value
			}
			ctx, err := manager.Load(ctx, token)
			if err != nil {
				postgresSessionError(w, err)
				return
			}
			login := r.URL.Path == "/login-fixture" || r.URL.Path == "/late-login-fixture"
			if login {
				if token != "" {
					w.WriteHeader(http.StatusConflict)
					return
				}
				// Pre-authenticated ordinary subject, no real login or authorization.
				manager.Put(ctx, "subject", "fixture-user")
				manager.Put(ctx, "remember_login", false)
				manager.RememberMe(ctx, false)
				if s.account {
					generation, err := s.currentAccount(ctx, "fixture-user")
					if err != nil {
						postgresSessionError(w, err)
						return
					}
					manager.Put(ctx, "auth_generation", generation)
				}
			} else {
				if manager.GetString(ctx, "subject") != "fixture-user" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				switch r.URL.Path {
				case "/me":
					w.WriteHeader(http.StatusOK)
					return
				case "/logout":
					if err := manager.Destroy(ctx); err != nil {
						postgresSessionError(w, err)
						return
					}
					manager.WriteSessionCookie(ctx, w, "", time.Time{})
					w.WriteHeader(http.StatusNoContent)
					return
				case "/activity-fixture":
				case "/late-save":
				default:
					w.WriteHeader(http.StatusNotFound)
					return
				}
			}
			if r.URL.Path == "/late-save" || r.URL.Path == "/late-login-fixture" {
				select {
				case f.api.loaded <- struct{}{}:
				case <-ctx.Done():
					postgresSessionError(w, ctx.Err())
					return
				}
				select {
				case <-f.api.resume:
				case <-ctx.Done():
					postgresSessionError(w, ctx.Err())
					return
				}
			}
			token, expiry, err := manager.Commit(ctx)
			if err != nil {
				postgresSessionError(w, err)
				return
			}
			manager.WriteSessionCookie(ctx, w, token, expiry)
			w.WriteHeader(http.StatusCreated)
		})))
		f.api.servers[i].Client().Timeout = 5 * time.Second
		t.Cleanup(f.api.servers[i].Close)
	}
	return f
}

func postgresSessionError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	if errors.Is(err, errLimit) {
		status = http.StatusConflict
	} else if errors.Is(err, errStale) {
		status = http.StatusUnauthorized
	}
	w.WriteHeader(status)
}
