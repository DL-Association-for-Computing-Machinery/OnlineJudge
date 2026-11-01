//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testLimitedLoginHTTP(t *testing.T, f *fixture) {
	t.Run("limited preflight rejects before any Redis call", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		l := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), loadedLoginSecrets(t, loginKey(t), loginKey(t)), rateLimits(), true)
		x := loginServer(t, l)
		pending := beginLogin(t, x, p)
		counter := &commandCounter{}
		f.clients[0].AddHook(counter)
		for _, tc := range []struct {
			name, path, body string
			cookies          []*http.Cookie
			mutate           func(*http.Request)
			status           int
		}{
			{"origin", "begin", loginBodyFor(t, p), nil, func(r *http.Request) { r.Header.Set("Origin", "https://evil.invalid") }, 403},
			{"method", "begin", loginBodyFor(t, p), nil, func(r *http.Request) { r.Method = "GET" }, 405},
			{"JSON", "begin", `{"email":"a@example.invalid","email":"b@example.invalid","password":"a"}`, nil, nil, 400},
			{"invalid email", "begin", `{"email":"bad","password":"a"}`, nil, nil, 400},
			{"bad session before begin", "begin", loginBodyFor(t, p), []*http.Cookie{{Name: transport.CookieName, Value: "bad"}}, nil, 400},
			{"bad session before complete", "complete", "", []*http.Cookie{pending, {Name: transport.CookieName, Value: "bad"}}, nil, 400},
			{"missing attempt", "complete", "", nil, nil, 400},
			{"bad attempt", "abandon", "", []*http.Cookie{{Name: transport.AttemptCookieName, Value: "bad"}}, nil, 400},
		} {
			t.Run(tc.name, func(t *testing.T) {
				before := rateSnapshot(t, f, ns)
				calls := counter.calls.Load()
				loginStatus(t, loginCall(x, "/api/v1/session/login/"+tc.path, tc.body, tc.cookies, tc.mutate), tc.status)
				if counter.calls.Load() != calls {
					t.Fatal("preflight rejection reached Redis")
				}
				unchangedRate(t, f, ns, before)
			})
		}
	})
	t.Run("socket source beats forwarded headers and throttles before KDF", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, true)
		limits := rateLimits()
		limits.BeginSource.Count = 1
		secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
		a := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), secrets, limits, true)
		probe := &loginAuthorityProbe{Authority: f.identities[1]}
		store, err := protocol.New(f.ctx, f.clients[1], probe, f.budget, ns, protocol.OrdinaryPolicy())
		must(t, err)
		b := limitedManager(t, f, store, secrets, limits, false)
		x, y := loginServer(t, a), loginServer(t, b)
		wrong := strings.Replace(loginBodyFor(t, p), "synthetic protocol password", "incorrect password", 1)
		fake := func(r *http.Request) {
			r.Header.Set("X-Forwarded-For", "198.51.100.1")
			r.Header.Set("Forwarded", "for=198.51.100.1")
		}
		loginStatus(t, loginCall(x, "/api/v1/session/login/begin", wrong, nil, fake), 401)
		before := rateSnapshot(t, f, ns)
		body := strings.Replace(loginBodyFor(t, p), p.Email.Original(), strings.ToUpper(p.Email.Original()), 1)
		r := loginCall(y, "/api/v1/session/login/begin", body, nil, func(r *http.Request) {
			r.Header.Set("X-Forwarded-For", "203.0.113.2")
			r.Header.Set("X-Real-IP", "203.0.113.2")
		})
		loginStatus(t, r, 429)
		unchangedRate(t, f, ns, before)
		var version int64
		must(t, f.pools[0].QueryRow(f.ctx, `SELECT credential_version FROM identity_credentials WHERE id=$1`, p.ID).Scan(&version))
		if probe.entries.Load() != 0 || version != 1 || len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("throttled correct password reached legacy upgrade or prepared a session")
		}
	})
	t.Run("begin exhaustion preserves separate completion and shared abandonment quota", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		limits := rateLimits()
		limits.BeginGlobal.Count = 1
		limits.RecoveryGlobal.Count = 1
		secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
		a := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), secrets, limits, true)
		b := limitedManager(t, f, f.store(t, ns, 1, protocol.OrdinaryPolicy()), secrets, limits, false)
		h, err := transport.NewLimitedLogin(a, transport.Config{Origin: loginOrigin, Timeout: time.Second})
		must(t, err)
		x, y := loginHandlerServer(t, h), loginServer(t, b)
		pending := beginLogin(t, x, p)
		loginStatus(t, loginCall(y, "/api/v1/session/login/begin", loginBodyFor(t, p), nil, nil), 429)
		credential := completedCredential(t, loginCall(y, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
		before := rateSnapshot(t, f, ns)
		loginStatus(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 429)
		loginStatus(t, loginCall(x, "/api/v1/session/login/abandon", "", []*http.Cookie{pending}, nil), 429)
		unchangedRate(t, f, ns, before)
		cookie := &http.Cookie{Name: transport.CookieName, Value: credential.Value()}
		loginStatus(t, loginCall(x, "/api/v1/session", "", []*http.Cookie{cookie}, func(r *http.Request) { r.Method = "GET" }), 200)
		loginStatus(t, loginCall(y, "/api/v1/session/logout", "", []*http.Cookie{cookie}, nil), 204)
	})
	for _, failure := range []string{"lost reply", "HTTP deadline"} {
		t.Run("charged but unavailable "+failure, func(t *testing.T) {
			ns := namespace(t)
			p := f.subject(t, true)
			limits := rateLimits()
			limits.BeginGlobal.Count = 1
			secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
			h := &hook{op: "login-admit", after: func(ctx context.Context, _ redis.Cmder) error {
				if failure == "HTTP deadline" {
					<-ctx.Done()
					return nil
				}
				return errors.New("synthetic quota reply lost")
			}}
			probe := &loginAuthorityProbe{Authority: f.identities[0]}
			l := limitedManager(t, f, f.faultStoreAuthority(t, ns, h, time.Second, probe), secrets, limits, true)
			x := loginServerBudget(t, l, 100*time.Millisecond)
			loginStatus(t, loginCall(x, "/api/v1/session/login/begin", loginBodyFor(t, p), nil, nil), 503)
			if !h.used.Load() || len(rateCounterKeys(t, f, ns)) != 3 {
				t.Fatal("admitted unknown result was refunded or retried")
			}
			other := limitedManager(t, f, f.store(t, ns, 1, protocol.OrdinaryPolicy()), secrets, limits, false)
			loginStatus(t, loginCall(loginServer(t, other), "/api/v1/session/login/begin", loginBodyFor(t, p), nil, nil), 429)
			var version int64
			must(t, f.pools[0].QueryRow(f.ctx, `SELECT credential_version FROM identity_credentials WHERE id=$1`, p.ID).Scan(&version))
			if probe.entries.Load() != 0 || version != 1 || len(namespaceRecords(t, f, ns)) != 0 {
				t.Fatal("unknown admission reached password upgrade or session write")
			}
		})
	}
	t.Run("actual Redis write pause fails closed within HTTP budget", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, true)
		h := &hook{op: "login-admit", before: func(ctx context.Context, _ redis.Cmder) error {
			return f.control.Do(ctx, "CLIENT", "PAUSE", 600, "WRITE").Err()
		}}
		l := limitedManager(t, f, f.faultStore(t, ns, h, 50*time.Millisecond), loadedLoginSecrets(t, loginKey(t), loginKey(t)), rateLimits(), true)
		defer func() { must(t, f.control.Do(f.ctx, "CLIENT", "UNPAUSE").Err()) }()
		x := loginServerBudget(t, l, 100*time.Millisecond)
		started := time.Now()
		loginStatus(t, loginCall(x, "/api/v1/session/login/begin", loginBodyFor(t, p), nil, nil), 503)
		if !h.used.Load() || time.Since(started) > 3*time.Second || len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("paused limiter was unbounded or admitted session work")
		}
		must(t, f.control.Do(f.ctx, "CLIENT", "UNPAUSE").Err())
	})
}
