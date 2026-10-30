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
	"github.com/redis/go-redis/v9"
)

type fixture struct {
	stores   [2]*guardedStore
	managers [2]*scs.SessionManager
	servers  [2]*httptest.Server
	loaded   chan struct{}
	resume   chan struct{}
}

func setup(t *testing.T) *fixture {
	return setupReplicas(t, "experiment:"+digest(t.Name())+":", 2)
}

func setupReplicas(t *testing.T, prefix string, replicas int) *fixture {
	return setupAuthorizedReplicas(t, prefix, replicas, nil)
}

type sessionConfigurer func(*scs.SessionManager, *guardedStore)

func setupAuthorizedReplicas(t *testing.T, prefix string, replicas int, authorities []*authAuthority, configure ...sessionConfigurer) *fixture {
	t.Helper()
	addr := os.Getenv("OJ_SESSION_REDIS_ADDR")
	if addr == "" {
		t.Fatal("requires disposable Redis: run scripts/test-session.py")
	}
	f := &fixture{loaded: make(chan struct{}, 1), resume: make(chan struct{})}
	for i := range replicas {
		client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1,
			DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second,
			PoolSize: 8, PoolTimeout: time.Second, ContextTimeoutEnabled: true})
		t.Cleanup(func() { _ = client.Close() })
		if err := client.Ping(context.Background()).Err(); err != nil {
			t.Fatal(err)
		}
		store := &guardedStore{client: client, prefix: prefix}
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
		for _, option := range configure {
			option(manager, store)
		}
		f.stores[i], f.managers[i] = store, manager
		f.servers[i] = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			token := ""
			if cookie, err := r.Cookie(manager.Cookie.Name); err == nil {
				token = cookie.Value
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
			if (r.URL.Path == "/login-fixture" || r.URL.Path == "/late-login-fixture") && r.Method == http.MethodPost {
				// Pre-authenticated test identity, never a production login endpoint.
				manager.Put(ctx, "subject", "fixture-user")
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
				switch r.URL.Path {
				case "/me":
					w.WriteHeader(http.StatusOK)
					return
				case "/logout":
					if r.Method != http.MethodPost {
						w.WriteHeader(http.StatusMethodNotAllowed)
						return
					}
					if err := manager.Destroy(ctx); err != nil {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					manager.WriteSessionCookie(ctx, w, "", time.Time{})
					w.WriteHeader(http.StatusNoContent)
					return
				case "/late-save":
					f.loaded <- struct{}{}
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
			if r.URL.Path == "/late-login-fixture" {
				f.loaded <- struct{}{}
				select {
				case <-f.resume:
				case <-ctx.Done():
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
			}
			token, expiry, err := manager.Commit(ctx)
			if errors.Is(err, errLimit) {
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
		}))
		f.servers[i].Client().Timeout = 5 * time.Second
		t.Cleanup(f.servers[i].Close)
	}
	return f
}

func request(f *fixture, replica int, method, path string, cookie *http.Cookie) (*http.Response, error) {
	req, err := http.NewRequest(method, f.servers[replica].URL+path, nil)
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
