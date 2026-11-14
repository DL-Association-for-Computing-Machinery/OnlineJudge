//go:build integration

package sessionprotocol_test

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testRuntime(t *testing.T, f *fixture) {
	t.Run("readiness tracks storage recovery independently from liveness", func(t *testing.T) { testRuntimeReadiness(t, f) })
	t.Run("two assembled owners use one authority and cookie lifecycle", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, true)
		c := runtimeConfig(t, f, ns, true)
		connections := redisConnections(t, f)
		a := openRuntime(t, c)
		c.Database.ApplicationName += "_b"
		b := openRuntime(t, c)
		x, y := loginHandlerServer(t, a), loginHandlerServer(t, b)
		jar, err := cookiejar.New(nil)
		must(t, err)
		x.client.Jar, y.client.Jar = jar, jar
		beginLogin(t, x, p)
		credential := completedCredential(t, loginCall(y, "/api/v1/session/login/complete", "", nil, nil))
		loginStatus(t, loginCall(y, "/api/v1/session/activity", "", nil, nil), 200)
		loginStatus(t, loginCall(x, "/api/v1/session", "", nil, func(r *http.Request) { r.Method = "GET" }), 200)
		state, err := f.identities[0].State(f.ctx, p.ID)
		must(t, err)
		if state.CredentialVersion != 2 {
			t.Fatal("assembled legacy credential did not use authoritative upgrade")
		}
		loginStatus(t, loginCall(y, "/api/v1/session/logout", "", nil, nil), 204)
		loginStatus(t, loginCall(x, "/api/v1/session", "", []*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}}, func(r *http.Request) { r.Method = "GET" }), 401)
		must(t, b.Close(t.Context()))
		loginStatus(t, loginCall(y, "/api/v1/session", "", nil, func(r *http.Request) { r.Method = "GET" }), 503)
		noRuntimeConnections(t, f, c.Database.ApplicationName)
		must(t, a.Close(t.Context()))
		noRuntimeConnections(t, f, strings.TrimSuffix(c.Database.ApplicationName, "_b"))
		sameRedisConnections(t, f, connections)
	})
	t.Run("assembled proxy uses verified backend certificate identity", func(t *testing.T) {
		c := runtimeConfig(t, f, namespace(t), true)
		ca := testpki.New(t)
		cert, _, _ := ca.Issue(t, "spiffe://onlinejudge/proxy/runtime", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
		c.Mode, c.Proxy = sessionruntime.ProxyMTLS, transport.ProxyConfig{CertificateSHA256: [][32]byte{sha256.Sum256(cert.Certificate[0])}}
		r := openRuntime(t, c)
		edge := proxyServer(t, httpapi.NewSessionHandler(r), ca, cert)
		x := proxyClient(t, edge.edge, "127.0.0.2", nil)
		loginStatus(t, loginCall(x, "/api/v1/readyz", "", nil, func(r *http.Request) { r.Method = http.MethodGet }), 200)
		other, _, _ := ca.Issue(t, "spiffe://onlinejudge/proxy/unpinned", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
		wrong := proxyClient(t, edge.backend, "127.0.0.2", &other)
		loginStatus(t, loginCall(wrong, "/api/v1/readyz", "", nil, func(r *http.Request) {
			r.Method = http.MethodGet
			r.Header.Set("X-Forwarded-Host", r.Host)
			r.Header.Set("X-Forwarded-Proto", "https")
			r.Header.Set("X-Forwarded-For", "127.0.0.2")
		}), 404)
		pending := beginLogin(t, x, f.subject(t, false))
		completedCredential(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
		plain := httptest.NewRecorder()
		r.ServeHTTP(plain, httptest.NewRequest("GET", "http://127.0.0.1/api/v1/session", nil))
		if plain.Code != 403 {
			t.Fatal("runtime accepted plaintext transport")
		}
	})
	t.Run("startup never provisions missing or mismatched limiter metadata", func(t *testing.T) {
		for _, provision := range []bool{false, true} {
			c := runtimeConfig(t, f, namespace(t)+map[bool]string{false: "_missing", true: "_mismatch"}[provision], provision)
			if provision {
				c.LoginLimits.BeginGlobal.Count++
			}
			before := rateSnapshot(t, f, c.Namespace)
			connections := redisConnections(t, f)
			r, err := sessionruntime.Open(t.Context(), c)
			if r != nil || !errors.Is(err, sessionruntime.ErrStartup) {
				t.Fatal("startup admitted missing/mismatched metadata")
			}
			unchangedRate(t, f, c.Namespace, before)
			noRuntimeConnections(t, f, c.Database.ApplicationName)
			sameRedisConnections(t, f, connections)
		}
	})
	t.Run("startup cancellation and deadline drain real PostgreSQL waiters", func(t *testing.T) {
		for _, kind := range []string{"cancel", "deadline"} {
			t.Run(kind, func(t *testing.T) {
				c := runtimeConfig(t, f, namespace(t), true)
				c.Database.Timeout = 2 * time.Second
				if kind == "deadline" {
					c.StartupTimeout = 500 * time.Millisecond
				}
				before := rateSnapshot(t, f, c.Namespace)
				held, err := f.pools[1].Begin(f.ctx)
				must(t, err)
				defer func() {
					if err := held.Rollback(f.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
						t.Error(err)
					}
				}()
				var blocker int32
				must(t, held.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&blocker))
				_, err = held.Exec(f.ctx, `LOCK TABLE identity_session_recovery IN ACCESS EXCLUSIVE MODE`)
				must(t, err)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				type result struct {
					runtime *sessionruntime.Runtime
					err     error
				}
				finished := make(chan result, 1)
				go func() { r, err := sessionruntime.Open(ctx, c); finished <- result{r, err} }()
				waiter := oneBackend(t, f.ctx, f.pools[0], blocker)
				if kind == "cancel" {
					cancel()
				}
				r := <-finished
				want := context.Canceled
				if kind == "deadline" {
					want = context.DeadlineExceeded
				}
				if r.runtime != nil || !errors.Is(r.err, want) {
					t.Fatal("startup did not honor cancellation/deadline")
				}
				gone(t, f.ctx, f.pools[0], waiter)
				noRuntimeConnections(t, f, c.Database.ApplicationName)
				unchangedRate(t, f, c.Namespace, before)
			})
		}
	})
	t.Run("closed recovery remains closed and startup releases connections", func(t *testing.T) {
		c := runtimeConfig(t, f, namespace(t), true)
		generation, err := f.identities[0].BeginRecovery(f.ctx)
		must(t, err)
		defer func() { must(t, f.identities[0].FinishRecovery(f.ctx, generation)) }()
		before := rateSnapshot(t, f, c.Namespace)
		r, err := sessionruntime.Open(t.Context(), c)
		if r != nil || !errors.Is(err, sessionruntime.ErrStartup) {
			t.Fatal("startup opened closed recovery")
		}
		state, err := f.identities[1].RecoveryState(f.ctx)
		must(t, err)
		if state.Enabled || state.Generation != generation {
			t.Fatal("startup modified recovery state")
		}
		unchangedRate(t, f, c.Namespace, before)
		noRuntimeConnections(t, f, c.Database.ApplicationName)
	})
	t.Run("wrong schema and Redis failure release startup resources", func(t *testing.T) {
		for _, step := range []string{"schema", "redis"} {
			t.Run(step, func(t *testing.T) {
				c := runtimeConfig(t, f, namespace(t), true)
				before := rateSnapshot(t, f, c.Namespace)
				if step == "schema" {
					_, err := f.pools[0].Exec(f.ctx, `INSERT INTO goose_db_version (version_id,is_applied) VALUES (999,true)`)
					must(t, err)
					defer func() {
						_, err := f.pools[0].Exec(f.ctx, `DELETE FROM goose_db_version WHERE version_id=999`)
						must(t, err)
					}()
				} else {
					c.Redis.Addr = "127.0.0.1:1"
				}
				r, err := sessionruntime.Open(t.Context(), c)
				if r != nil || !errors.Is(err, sessionruntime.ErrStartup) {
					t.Fatal("startup ignored schema/storage failure")
				}
				unchangedRate(t, f, c.Namespace, before)
				noRuntimeConnections(t, f, c.Database.ApplicationName)
			})
		}
	})
}
