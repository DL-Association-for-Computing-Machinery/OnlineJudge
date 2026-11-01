//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testEntryStartupSignal(t *testing.T, f *fixture, binary string) {
	c := runtimeConfig(t, f, namespace(t), true)
	c.Database.Timeout = 5 * time.Second
	ca := testpki.New(t)
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
	before := rateSnapshot(t, f, c.Namespace)
	connections := redisConnections(t, f)
	p := entryStart(t, binary, entryConfig(t, c, ca, time.Second), "127.0.0.1:0")
	waiter := oneBackend(t, f.ctx, f.pools[0], blocker)
	p.signal(t)
	p.wait(t, true)
	if _, ok := p.event("API starting"); ok {
		t.Fatal("canceled startup opened an HTTP listener")
	}
	if _, ok := p.event("API startup canceled"); !ok {
		t.Fatal("startup signal did not reach cancellation path")
	}
	if _, ok := p.event("API stopped gracefully"); ok {
		t.Fatal("canceled startup claimed served drain")
	}
	gone(t, f.ctx, f.pools[0], waiter)
	noRuntimeConnections(t, f, c.Database.ApplicationName)
	sameRedisConnections(t, f, connections)
	unchangedRate(t, f, c.Namespace, before)
}

// Process exit stops the driver before it can finish asynchronous cancellation.
// A PG lock waiter need not notice socket EOF until its configured statement/
// lock timeout. Keep the blocker held and verify that exact remote backend ends
// within its own declared budget, rather than treating exit as business drain.
func entryExpiredBackendGone(t *testing.T, f *fixture, waiter backend, observed time.Time, budget time.Duration) {
	t.Helper()
	ctx, cancel := context.WithDeadline(t.Context(), observed.Add(budget+time.Second))
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	logged := false
	for {
		var count int
		var state, wait string
		if err := f.pools[0].QueryRow(ctx, `SELECT count(*),coalesce(max(state),''),coalesce(max(wait_event_type),'') FROM pg_stat_activity WHERE pid=$1 AND backend_start=$2`, waiter.pid, waiter.started).Scan(&count, &state, &wait); err != nil {
			t.Fatal("remote backend exceeded its configured database budget")
		}
		if !logged && count != 0 {
			t.Logf("after failed API exit: remote state=%s wait=%s", state, wait)
			logged = true
		}
		if count == 0 {
			t.Logf("remote backend gone after %s from observed lock wait", time.Since(observed).Round(time.Millisecond))
			return
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			t.Fatal("exact remote backend retained beyond database budget")
		}
	}
}

func testEntryDrain(t *testing.T, f *fixture, binary string) {
	for _, complete := range []bool{true, false} {
		name := "deadline is failure"
		if complete {
			name = "released waiter drains successfully"
		}
		t.Run(name, func(t *testing.T) {
			c := runtimeConfig(t, f, namespace(t), true)
			c.Database.Timeout, c.HTTP.Timeout = 5*time.Second, 5*time.Second
			budget := 50 * time.Millisecond
			if complete {
				budget = 3 * time.Second
			}
			ca := testpki.New(t)
			connections := redisConnections(t, f)
			p := entryStart(t, binary, entryConfig(t, c, ca, budget), "127.0.0.1:0")
			e := entryClient(t, p, ca)
			subject := f.subject(t, false)
			pending := beginLogin(t, e, subject)
			credential := completedCredential(t, loginCall(e, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
			held, err := f.pools[1].Begin(f.ctx)
			must(t, err)
			defer func() {
				if err := held.Rollback(f.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Error(err)
				}
			}()
			var blocker int32
			must(t, held.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&blocker))
			_, err = held.Exec(f.ctx, `SELECT id FROM identity_credentials WHERE id=$1 FOR UPDATE`, subject.ID)
			must(t, err)
			response := make(chan loginResponse, 1)
			go func() {
				response <- loginCall(e, "/api/v1/session", "", []*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}},
					func(r *http.Request) { r.Method = "GET" })
			}()
			waiter := oneBackend(t, f.ctx, f.pools[0], blocker)
			observed := time.Now()
			p.signal(t)
			stopping := p.waitEvent(t, "API stopping")
			if stopping.Scope != "ordinary-session-http" {
				t.Fatal("shutdown lost session scope")
			}
			// Shutdown must close listening before waiting for existing synchronous
			// authority calls; an in-flight request cannot justify accepting more.
			event, _ := p.event("API starting")
			entryListenerClosed(t, event.Address)
			if complete {
				select {
				case <-p.done:
					t.Fatal("graceful shutdown exited with active authority waiter")
				default:
				}
				if runtimeConnections(t, f, c.Database.ApplicationName) == 0 || redisConnections(t, f) <= connections {
					t.Fatal("draining API closed resources while authority request remained active")
				}
				if _, ok := p.event("API stopped gracefully"); ok {
					t.Fatal("drain success preceded active authority completion")
				}
				must(t, held.Rollback(f.ctx))
				loginStatus(t, awaitEntryResponse(t, response), 200)
				p.wait(t, true)
				if _, ok := p.event("API stopped gracefully"); !ok {
					t.Fatal("confirmed request/resource drain not logged")
				}
			} else {
				p.wait(t, false)
				if _, ok := p.event("API stopped gracefully"); ok {
					t.Fatal("shutdown timeout reported graceful success")
				}
				result := awaitEntryResponse(t, response)
				if result.err == nil && result.status == 200 {
					t.Fatal("blocked authority was authenticated during forced exit")
				}
			}
			if complete {
				gone(t, f.ctx, f.pools[0], waiter)
			} else {
				entryExpiredBackendGone(t, f, waiter, observed, c.Database.Timeout)
			}
			noRuntimeConnections(t, f, c.Database.ApplicationName)
			sameRedisConnections(t, f, connections)
		})
	}
}

func entryListenerClosed(t *testing.T, address string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 20*time.Millisecond)
		if err != nil {
			return
		}
		_ = connection.Close()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("stopping API kept accepting TCP connections")
}

func awaitEntryResponse(t *testing.T, response <-chan loginResponse) loginResponse {
	t.Helper()
	select {
	case result := <-response:
		return result
	case <-time.After(6 * time.Second):
		t.Fatal("active API request did not finish")
	}
	return loginResponse{}
}
