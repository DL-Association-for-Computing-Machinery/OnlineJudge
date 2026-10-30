package sessionexperiment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

type fixture struct {
	stores        [2]*guardedStore
	managers      [2]*scs.SessionManager
	servers       [2]*httptest.Server
	loaded        chan struct{}
	resume        chan struct{}
	adminStores   [2]*guardedStore
	adminManagers [2]*scs.SessionManager
	routePrefix   string
}

func setup(t *testing.T) *fixture {
	return setupReplicas(t, "experiment:"+digest(t.Name())+":", 2)
}

func setupReplicas(t *testing.T, prefix string, replicas int) *fixture {
	return setupAuthorizedReplicas(t, prefix, replicas, nil)
}

type sessionConfigurer func(*scs.SessionManager, *guardedStore)

func setupAuthorizedReplicas(t *testing.T, prefix string, replicas int, authorities []*authAuthority, configure ...sessionConfigurer) *fixture {
	return setupAuthorizedReplicasWithRemember(t, prefix, replicas, authorities, rememberPolicy{7 * 24 * time.Hour, 24 * time.Hour}, configure...)
}

type rememberPolicy struct {
	lifetime, idle time.Duration
}

func setupAuthorizedReplicasWithRemember(t *testing.T, prefix string, replicas int, authorities []*authAuthority, remember rememberPolicy, configure ...sessionConfigurer) *fixture {
	return setupAuthorizedReplicasWithProfiles(t, prefix, replicas, authorities, remember, rememberPolicy{8 * time.Hour, 30 * time.Minute}, configure...)
}

func setupAuthorizedReplicasWithProfiles(t *testing.T, prefix string, replicas int, authorities []*authAuthority, remember, admin rememberPolicy, configure ...sessionConfigurer) *fixture {
	t.Helper()
	addr := os.Getenv("OJ_SESSION_REDIS_ADDR")
	if addr == "" {
		t.Fatal("requires disposable Redis: run scripts/test-session.py")
	}
	f := &fixture{loaded: make(chan struct{}, 1), resume: make(chan struct{})}
	for i := range replicas {
		client := sessionRedisClient(addr, prefix+"api:", sessionRedisPoolLimit)
		t.Cleanup(func() { _ = client.Close() })
		if err := client.Ping(context.Background()).Err(); err != nil {
			t.Fatal(err)
		}
		var handlers [2]http.Handler
		for profile := range 2 {
			store := &guardedStore{client: client, prefix: prefix, admin: profile == 1}
			if store.admin {
				store.prefix += "admin:"
			}
			if authorities != nil {
				store.auth = authorities[i]
			}
			manager := scs.New()
			manager.Store = store
			manager.Lifetime = 12 * time.Hour
			manager.IdleTimeout = 2 * time.Hour
			manager.Cookie.Secure = true
			manager.Cookie.HttpOnly = true
			manager.Cookie.SameSite = http.SameSiteStrictMode
			manager.Cookie.Persist = false
			if store.admin {
				manager.Lifetime, manager.IdleTimeout = admin.lifetime, admin.idle
				manager.Cookie.Name, manager.Cookie.Path = "admin_session", "/admin-fixture"
				f.adminStores[i], f.adminManagers[i] = store, manager
			} else {
				for _, option := range configure {
					option(manager, store)
				}
				f.stores[i], f.managers[i] = store, manager
			}
			handlers[profile] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Configuration is immutable after startup. Each request selects its
				// idle policy without changing another request's shared manager.
				localManager := *manager
				manager := &localManager
				path := r.URL.Path
				if store.admin {
					path = strings.TrimPrefix(path, "/admin-fixture")
				}
				switch path {
				case "/login-fixture", "/late-login-fixture", "/activity-fixture", "/logout", "/late-save", "/rotate-fixture", "/late-rotate-fixture":
					if r.Method != http.MethodPost {
						w.Header().Set("Allow", http.MethodPost)
						w.WriteHeader(http.StatusMethodNotAllowed)
						return
					}
				}
				login := (path == "/login-fixture" || path == "/late-login-fixture") && r.Method == http.MethodPost
				rememberLogin := !store.admin && login && r.URL.Query().Get("remember") == "true"
				if rememberLogin {
					manager.Lifetime = remember.lifetime
				}
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()
				token := ""
				if cookie, err := r.Cookie(manager.Cookie.Name); err == nil {
					token = cookie.Value
				}
				rotate := path == "/rotate-fixture" || path == "/late-rotate-fixture"
				if rotate {
					manager.Store = &rotationStore{guardedStore: store, old: token}
				}
				ctx, err := manager.Load(ctx, token)
				if errors.Is(err, errStale) {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if err != nil {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if login {
					// Policy selection is only allowed for a fresh pre-authenticated
					// login, never by upgrading an existing credential in place.
					if token != "" {
						w.WriteHeader(http.StatusConflict)
						return
					}
					// Pre-authenticated test identity, never a production login endpoint.
					manager.Put(ctx, "subject", "fixture-user")
					manager.Put(ctx, "remember_login", rememberLogin)
					manager.RememberMe(ctx, rememberLogin)
					if store.admin {
						manager.Put(ctx, "session_scope", "admin")
					}
					if store.auth != nil {
						generation, err := store.auth.current(ctx, "fixture-user")
						if errors.Is(err, errStale) {
							w.WriteHeader(http.StatusUnauthorized)
							return
						}
						if err != nil {
							w.WriteHeader(http.StatusServiceUnavailable)
							return
						}
						manager.Put(ctx, "auth_generation", generation.account)
						manager.Put(ctx, "recovery_generation", generation.recovery)
					}
				} else {
					if manager.GetString(ctx, "subject") != "fixture-user" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					switch path {
					case "/rotate-fixture", "/late-rotate-fixture":
					case "/me":
						w.WriteHeader(http.StatusOK)
						return
					case "/activity-fixture":
						// Trusted activity probe, not a production activity/CSRF policy.
					case "/logout":
						if err := manager.Destroy(ctx); err != nil {
							w.WriteHeader(http.StatusServiceUnavailable)
							return
						}
						manager.WriteSessionCookie(ctx, w, "", time.Time{})
						w.WriteHeader(http.StatusNoContent)
						return
					case "/late-save":
						select {
						case f.loaded <- struct{}{}:
						case <-ctx.Done():
							w.WriteHeader(http.StatusServiceUnavailable)
							return
						}
						select {
						case <-f.resume:
						case <-ctx.Done():
							w.WriteHeader(http.StatusServiceUnavailable)
							return
						}
					default:
						w.WriteHeader(http.StatusNotFound)
						return
					}
				}
				if path == "/late-login-fixture" || path == "/late-rotate-fixture" {
					select {
					case f.loaded <- struct{}{}:
					case <-ctx.Done():
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					select {
					case <-f.resume:
					case <-ctx.Done():
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
				}
				if !store.admin && manager.GetBool(ctx, "remember_login") {
					manager.IdleTimeout = remember.idle
				}
				if rotate {
					deadline := manager.Deadline(ctx)
					if err := manager.RenewToken(ctx); err != nil {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					manager.SetDeadline(ctx, deadline)
				}
				token, expiry, err := manager.Commit(ctx)
				if errors.Is(err, errLimit) || errors.Is(err, errRotationConflict) {
					w.WriteHeader(http.StatusConflict)
					return
				}
				if errors.Is(err, errStale) {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if err != nil {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				manager.WriteSessionCookie(ctx, w, token, expiry)
				w.WriteHeader(http.StatusCreated)
			})
		}
		f.servers[i] = httptest.NewTLSServer(sessionOriginProtection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			profile := 0
			if strings.HasPrefix(r.URL.Path, "/admin-fixture/") {
				profile = 1
			}
			handlers[profile].ServeHTTP(w, r)
		})))
		f.servers[i].Client().Timeout = 5 * time.Second
		t.Cleanup(f.servers[i].Close)
	}
	return f
}

func request(f *fixture, replica int, method, path string, cookie *http.Cookie) (*http.Response, error) {
	req, err := http.NewRequest(method, f.servers[replica].URL+f.routePrefix+path, nil)
	if err != nil {
		return nil, err
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	client := f.servers[replica].Client()
	response, err := client.Do(req)
	if err == nil {
		_ = response.Body.Close() // Cleanup; the operation outcome is checked separately.
	}
	return response, err
}

func expect(t *testing.T, f *fixture, replica int, method, path string, cookie *http.Cookie, status int) *http.Response {
	t.Helper()
	r, err := request(f, replica, method, path, cookie)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != status {
		t.Fatalf("%s: got %d want %d", path, r.StatusCode, status)
	}
	return r
}

func TestConcurrentAdmissionAcrossAPIs(t *testing.T) {
	f := setup(t)
	start := make(chan struct{})
	results := make(chan *http.Response, 24)
	errorsCh := make(chan error, 24)
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := request(f, i%2, http.MethodPost, "/login-fixture", nil)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- r
		}()
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
		case http.StatusCreated:
			if len(r.Cookies()) != 1 {
				t.Fatal("missing session cookie")
			}
			cookies = append(cookies, r.Cookies()[0])
		case http.StatusConflict:
			conflicts++
			if len(r.Cookies()) != 0 {
				t.Fatal("failed admission sent cookie")
			}
		default:
			t.Errorf("unexpected status %d", r.StatusCode)
		}
	}
	if len(cookies) != 3 || conflicts != 21 {
		t.Fatalf("admitted=%d rejected=%d", len(cookies), conflicts)
	}
	for _, cookie := range cookies {
		if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
			t.Fatal("cookie attributes")
		}
		for replica := range 2 {
			expect(t, f, replica, "GET", "/me", cookie, 200)
		}
	}
	expect(t, f, 1, "POST", "/logout", cookies[0], 204)
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookies[0], 401)
	}
	expect(t, f, 0, "POST", "/login-fixture", nil, 201)
	expect(t, f, 1, "POST", "/login-fixture", nil, 409)
	keys, err := f.stores[0].client.Keys(context.Background(), f.stores[0].prefix+"*").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		for _, cookie := range cookies {
			if strings.Contains(key, cookie.Value) {
				t.Fatal("raw token in Redis key")
			}
		}
	}
}

func TestRevocationRejectsLoadedRequest(t *testing.T) {
	f := setup(t)
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	finished := make(chan error, 1)
	go func() {
		r, err := request(f, 0, "POST", "/late-save", cookie)
		if err == nil && r.StatusCode != 401 {
			err = fmt.Errorf("late save status %d", r.StatusCode)
		}
		if err == nil && len(r.Cookies()) != 0 {
			err = errors.New("late save sent cookie")
		}
		finished <- err
	}()
	select {
	case <-f.loaded:
	case <-time.After(5 * time.Second):
		close(f.resume)
		t.Fatal("load barrier")
	}
	expect(t, f, 1, "POST", "/logout", cookie, 204)
	close(f.resume)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 401)
	}
	for range 3 {
		expect(t, f, 1, "POST", "/login-fixture", nil, 201)
	}
	expect(t, f, 0, "POST", "/login-fixture", nil, 409)
}

func TestIdleExpiryCannotBeResurrected(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	deadline := time.Now().Add(time.Hour)
	b, err := (scs.GobCodec{}).Encode(deadline, map[string]interface{}{"subject": "fixture-user"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.stores[0].CommitCtx(ctx, "expiry-fixture", b, time.Now().Add(150*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(3 * time.Second)
	for {
		_, found, err := f.stores[1].FindCtx(ctx, "expiry-fixture")
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			break
		}
		if time.Now().After(until) {
			t.Fatal("session did not expire")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := f.stores[0].CommitCtx(ctx, "expiry-fixture", b, deadline); !errors.Is(err, errStale) {
		t.Fatalf("late renewal: %v", err)
	}
	for range 3 {
		expect(t, f, 1, "POST", "/login-fixture", nil, 201)
	}
	expect(t, f, 0, "POST", "/login-fixture", nil, 409)
}

func TestRedisFailureIsNotLogoutSuccess(t *testing.T) {
	f := setup(t)
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	if err := f.stores[1].client.Close(); err != nil {
		t.Fatal(err)
	}
	r := expect(t, f, 1, "POST", "/logout", cookie, 503)
	if len(r.Cookies()) != 0 {
		t.Fatal("failed logout cleared cookie")
	}
	expect(t, f, 0, "GET", "/me", cookie, 200)
}

func TestRevokedTokenCannotChangeSubject(t *testing.T) {
	f := setup(t)
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	ctx := context.Background()
	data, found, err := f.stores[0].FindCtx(ctx, cookie.Value)
	if err != nil || !found {
		t.Fatal("missing admitted session")
	}
	deadline, values, err := (scs.GobCodec{}).Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, f, 1, "POST", "/logout", cookie, 204)
	values["subject"] = "another-fixture-user"
	changed, err := (scs.GobCodec{}).Encode(deadline, values)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.stores[0].CommitCtx(ctx, cookie.Value, changed, time.Now().Add(time.Hour)); !errors.Is(err, errStale) {
		t.Fatalf("revoked token accepted for another subject: %v", err)
	}
	// A rejected write must not occupy a slot for either subject.
	for range 3 {
		expect(t, f, 1, "POST", "/login-fixture", nil, 201)
	}
	for i := range 3 {
		if err := f.stores[1].CommitCtx(ctx, fmt.Sprintf("other-subject-%d", i), changed, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.stores[0].CommitCtx(ctx, "other-subject-overflow", changed, time.Now().Add(time.Hour)); !errors.Is(err, errLimit) {
		t.Fatalf("other subject limit: %v", err)
	}
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 401)
	}
}

func TestRepeatedSaveKeepsOneSlotAndAbsoluteDeadline(t *testing.T) {
	f := setup(t)
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	ctx := context.Background()
	data, found, err := f.stores[0].FindCtx(ctx, cookie.Value)
	if err != nil || !found {
		t.Fatal("missing admitted session")
	}
	deadline, values, err := (scs.GobCodec{}).Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		if err := f.stores[i%2].CommitCtx(ctx, cookie.Value, data, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	changed, err := (scs.GobCodec{}).Encode(deadline.Add(time.Minute), values)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.stores[1].CommitCtx(ctx, cookie.Value, changed, time.Now().Add(time.Hour)); !errors.Is(err, errStale) {
		t.Fatal("absolute deadline changed")
	}
	for range 2 {
		expect(t, f, 1, "POST", "/login-fixture", nil, 201)
	}
	expect(t, f, 0, "POST", "/login-fixture", nil, 409)
	past, err := (scs.GobCodec{}).Encode(time.Now().Add(-time.Second), values)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.stores[0].CommitCtx(ctx, "expired-absolute-fixture", past, time.Now().Add(time.Hour)); !errors.Is(err, errStale) {
		t.Fatal("accepted expired absolute deadline")
	}
}
