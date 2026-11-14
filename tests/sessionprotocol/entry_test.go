//go:build integration

package sessionprotocol_test

import (
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testEntry(t *testing.T, f *fixture) {
	binary := entryBinary(t)
	t.Run("own active devices over HTTPS", func(t *testing.T) { testEntryDevices(t, f, binary) })
	t.Run("private draft HTTP", func(t *testing.T) { testEntryPrivateDrafts(t, f, binary) })
	t.Run("personal training history HTTP", func(t *testing.T) { testEntryTrainingHistory(t, f, binary) })
	t.Run("training intake from an ordinary session", func(t *testing.T) { testTrainingIntake(t, f, binary) })
	t.Run("default bootstrap has no session entry", func(t *testing.T) {
		p := entryStart(t, binary, "", "127.0.0.1:0")
		event := p.waitEvent(t, "API starting")
		if event.Scope != "bootstrap-only" {
			t.Fatal("default API selected a privileged scope")
		}
		client := &http.Client{Timeout: 3 * time.Second}
		for path, want := range map[string]int{"/api/v1/healthz": 200, "/api/v1/session": 404, "/api/v1/session/login/begin": 404, entryTrainingHistory: 404} {
			response, err := client.Get("http://" + event.Address + path)
			must(t, err)
			_, err = io.Copy(io.Discard, response.Body)
			must(t, err)
			must(t, response.Body.Close())
			if response.StatusCode != want {
				t.Fatal("default bootstrap route contract changed")
			}
		}
		p.signal(t)
		p.wait(t, true)
		if _, ok := p.event("API stopped gracefully"); !ok {
			t.Fatal("bootstrap did not confirm graceful shutdown")
		}
	})
	t.Run("two real direct TLS APIs share cookie lifecycle and revocation", func(t *testing.T) {
		c := runtimeConfig(t, f, namespace(t), true)
		ca := testpki.New(t)
		before := redisConnections(t, f)
		a := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
		x := entryClient(t, a, ca)
		other := c
		other.Database.ApplicationName += "_b"
		b := entryStart(t, binary, entryConfig(t, other, ca, 3*time.Second), "127.0.0.1:0")
		y := entryClient(t, b, ca)
		jar, err := cookiejar.New(nil)
		must(t, err)
		x.client.Jar, y.client.Jar = jar, jar
		subject := f.subject(t, true)
		beginLogin(t, x, subject)
		credential := completedCredential(t, loginCall(y, "/api/v1/session/login/complete", "", nil, nil))
		loginStatus(t, loginCall(x, "/api/v1/session", "", nil, func(r *http.Request) { r.Method = "GET" }), 200)
		loginStatus(t, loginCall(y, "/api/v1/session/activity", "", nil, nil), 200)
		state, err := f.identities[0].State(f.ctx, subject.ID)
		must(t, err)
		if state.CredentialVersion != 2 {
			t.Fatal("real API did not upgrade legacy password via authority")
		}
		loginStatus(t, loginCall(y, "/api/v1/session/logout", "", nil, nil), 204)
		cookie := []*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}}
		loginStatus(t, loginCall(x, "/api/v1/session", "", cookie, func(r *http.Request) { r.Method = "GET" }), 401)
		beginLogin(t, y, subject)
		completedCredential(t, loginCall(x, "/api/v1/session/login/complete", "", nil, nil))
		_, err = f.identities[1].DisableCAS(f.ctx, subject.ID, state.Revision)
		must(t, err)
		loginStatus(t, loginCall(y, "/api/v1/session", "", nil, func(r *http.Request) { r.Method = "GET" }), 401)
		for _, p := range []*entryProcess{a, b} {
			p.signal(t)
			p.wait(t, true)
			if _, ok := p.event("API stopped gracefully"); !ok {
				t.Fatal("real API omitted confirmed drain")
			}
		}
		noRuntimeConnections(t, f, c.Database.ApplicationName)
		noRuntimeConnections(t, f, other.Database.ApplicationName)
		sameRedisConnections(t, f, before)
	})
	t.Run("failed startup never provisions metadata or retains pools", func(t *testing.T) {
		for _, cause := range []string{"missing marker", "listener occupied"} {
			t.Run(cause, func(t *testing.T) {
				c := runtimeConfig(t, f, namespace(t), cause != "missing marker")
				ca := testpki.New(t)
				address := "127.0.0.1:0"
				if cause == "listener occupied" {
					occupied, err := net.Listen("tcp", address)
					must(t, err)
					defer func() { must(t, occupied.Close()) }()
					address = occupied.Addr().String()
				}
				before := rateSnapshot(t, f, c.Namespace)
				connections := redisConnections(t, f)
				p := entryStart(t, binary, entryConfig(t, c, ca, time.Second), address)
				p.wait(t, false)
				if _, ok := p.event("API starting"); ok {
					t.Fatal("failed startup advertised a listener")
				}
				if _, ok := p.event("API stopped gracefully"); ok {
					t.Fatal("failed startup claimed success")
				}
				unchangedRate(t, f, c.Namespace, before)
				noRuntimeConnections(t, f, c.Database.ApplicationName)
				sameRedisConnections(t, f, connections)
			})
		}
	})
	t.Run("startup signal cancels real PostgreSQL lock wait without listening", func(t *testing.T) { testEntryStartupSignal(t, f, binary) })
	t.Run("active request signal shares bounded drain", func(t *testing.T) { testEntryDrain(t, f, binary) })
	t.Run("real mTLS proxy entry authenticates certificate and forwarding", func(t *testing.T) { testEntryProxy(t, f, binary) })
	t.Run("authenticated password change HTTP", func(t *testing.T) { testEntryPasswordChange(t, f, binary) })
	t.Run("database archive restore fences old cookies", func(t *testing.T) { testEntryDatabaseRestore(t, f, binary) })
}
