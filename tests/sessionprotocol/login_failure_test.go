//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type commandCounter struct{ calls atomic.Int64 }

func (c *commandCounter) DialHook(next redis.DialHook) redis.DialHook { return next }
func (c *commandCounter) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (c *commandCounter) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error { c.calls.Add(1); return next(ctx, cmd) }
}

func testLoginPreflight(t *testing.T, f *fixture) {
	ns := namespace(t)
	p := f.subject(t, false)
	s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
	x := loginServer(t, loginManager(t, s, time.Minute, loginKey(t)))
	counter := &commandCounter{}
	f.clients[0].AddHook(counter)
	for _, tc := range []struct {
		name, path, body string
		status           int
		cookies          []*http.Cookie
		mutate           func(*http.Request)
	}{
		{"source", "begin", loginBodyFor(t, p), 403, nil, func(r *http.Request) { r.Header.Set("Origin", "https://evil.invalid") }},
		{"no source", "begin", loginBodyFor(t, p), 403, nil, func(r *http.Request) { r.Header.Del("Origin") }},
		{"method", "complete", "", 405, nil, func(r *http.Request) { r.Method = "GET" }},
		{"duplicate field", "begin", `{"email":"a@example.invalid","password":"a","password":"b"}`, 400, nil, nil},
		{"media", "begin", loginBodyFor(t, p), 415, nil, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }},
		{"missing attempt before old session read", "complete", "", 400, []*http.Cookie{{Name: transport.CookieName, Value: strings.Repeat("A", 43)}}, nil},
		{"malformed attempt before old session read", "complete", "", 400, []*http.Cookie{{Name: transport.AttemptCookieName, Value: "bad"}, {Name: transport.CookieName, Value: strings.Repeat("A", 43)}}, nil},
		{"duplicate attempt", "complete", "", 400, []*http.Cookie{{Name: transport.AttemptCookieName, Value: "a"}, {Name: transport.AttemptCookieName, Value: "b"}}, nil},
		{"duplicate session", "begin", loginBodyFor(t, p), 400, []*http.Cookie{{Name: transport.CookieName, Value: "a"}, {Name: transport.CookieName, Value: "b"}}, nil},
		{"pending prevents prepare", "begin", loginBodyFor(t, p), 409, []*http.Cookie{{Name: transport.AttemptCookieName, Value: "pending"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := counter.calls.Load()
			r := loginCall(x, "/api/v1/session/login/"+tc.path, tc.body, tc.cookies, tc.mutate)
			loginStatus(t, r, tc.status)
			if counter.calls.Load() != before || len(namespaceRecords(t, f, ns)) != 0 {
				t.Fatal("preflight rejection reached Redis")
			}
		})
	}
	for _, secret := range []string{"incorrect password", strings.Repeat("x", 129)} {
		body := strings.Replace(loginBodyFor(t, p), "synthetic protocol password", secret, 1)
		status := 401
		if len(secret) > 128 {
			status = 400
		}
		loginStatus(t, loginCall(x, "/api/v1/session/login/begin", body, nil, nil), status)
		if len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("failed credential validation created a session")
		}
	}
}

func testLoginUnknown(t *testing.T, f *fixture) {
	for _, op := range []string{"retry", "check"} {
		t.Run(op+" succeeds before HTTP budget expires", func(t *testing.T) {
			ns := namespace(t)
			p := f.subject(t, false)
			key := loginKey(t)
			normal := f.store(t, ns, 1, protocol.OrdinaryPolicy())
			peer := loginServer(t, loginManager(t, normal, time.Minute, key))
			pending := beginLogin(t, peer, p)
			h := &hook{op: op, after: func(ctx context.Context, _ redis.Cmder) error { <-ctx.Done(); return ctx.Err() }}
			fault := f.faultStore(t, ns, h, time.Second)
			x := loginServerBudget(t, loginManager(t, fault, time.Minute, key), 150*time.Millisecond)
			loginStatus(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 503)
			if !h.used.Load() {
				t.Fatal("timeout did not follow a real protocol operation")
			}
			keys := namespaceRecords(t, f, ns)
			if len(keys) != 1 {
				t.Fatal("timeout lost original admitted record")
			}
			before, err := f.control.HGetAll(f.ctx, keys[0]).Result()
			must(t, err)
			ttl, err := f.control.PExpireTime(f.ctx, keys[0]).Result()
			must(t, err)
			credential := completedCredential(t, loginCall(peer, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
			after, err := f.control.HGetAll(f.ctx, keys[0]).Result()
			must(t, err)
			afterTTL, err := f.control.PExpireTime(f.ctx, keys[0]).Result()
			must(t, err)
			if tokenKey(ns, credential) != keys[0] || !reflect.DeepEqual(before, after) || ttl != afterTTL {
				t.Fatal("timeout recovery changed token or original deadlines")
			}
		})
	}
	for _, op := range []string{"retry", "check"} {
		t.Run(op+" lost receipt", func(t *testing.T) {
			ns := namespace(t)
			p := f.subject(t, false)
			key := loginKey(t)
			h := &hook{op: op, after: func(context.Context, redis.Cmder) error { return errors.New("synthetic login receipt loss") }}
			fault := f.faultStore(t, ns, h, time.Second)
			normal := f.store(t, ns, 1, protocol.OrdinaryPolicy())
			x := loginServer(t, loginManager(t, fault, time.Minute, key))
			pending := beginLogin(t, x, p)
			loginStatus(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 503)
			if !h.used.Load() {
				t.Fatal("receipt loss did not follow actual command")
			}
			keys := namespaceRecords(t, f, ns)
			if len(keys) != 1 {
				t.Fatal("unknown write did not preserve one candidate")
			}
			before, err := f.control.HGetAll(f.ctx, keys[0]).Result()
			must(t, err)
			// Reconstruct the manager and transport, without request-local state.
			y := loginServer(t, loginManager(t, normal, time.Minute, key))
			c := completedCredential(t, loginCall(y, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
			if tokenKey(ns, c) != keys[0] {
				t.Fatal("recovery issued a replacement credential")
			}
			c2 := completedCredential(t, loginCall(y, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
			if c2 != c {
				t.Fatal("dropped completion response created a new credential")
			}
			after, err := f.control.HGetAll(f.ctx, keys[0]).Result()
			must(t, err)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("receipt recovery changed original session metadata")
			}
		})
	}
	t.Run("prepare commit failure publishes no continuation", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		h := &hook{op: "time", after: func(context.Context, redis.Cmder) error {
			b := oneBackend(t, f.ctx, f.pools[1], 0)
			var killed bool
			must(t, f.pools[1].QueryRow(f.ctx, `SELECT pg_terminate_backend($1)`, b.pid).Scan(&killed))
			if !killed {
				t.Fatal("preparation authority backend not terminated")
			}
			gone(t, f.ctx, f.pools[1], b)
			return nil
		}}
		fault := f.faultStore(t, ns, h, time.Second)
		l := loginManager(t, fault, time.Minute, loginKey(t))
		c, err := l.Prepare(f.ctx, p.Email, []byte("synthetic protocol password"))
		wantError(t, err, identity.ErrStore)
		wantError(t, err, identity.ErrGuardedOperation)
		if c != (protocol.Continuation{}) || len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("failed preparation published recovery data or wrote a session")
		}
	})
	t.Run("full quota retains candidate until one slot is freed", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		var old protocol.Credential
		for range 3 {
			_, old = f.create(t, s, p)
		}
		l := loginManager(t, s, time.Minute, loginKey(t))
		c, err := l.Prepare(f.ctx, p.Email, []byte("synthetic protocol password"))
		must(t, err)
		x := loginServer(t, l)
		pending := &http.Cookie{Name: transport.AttemptCookieName, Value: c.Value()}
		loginStatus(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 409)
		must(t, s.Revoke(f.ctx, old))
		fresh := completedCredential(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
		v, err := s.Read(f.ctx, fresh)
		must(t, err)
		if v.CreatedAt != c.Deadline().Add(-time.Minute) || v.AbsoluteDeadline != v.CreatedAt.Add(12*time.Hour) {
			t.Fatal("quota recovery replaced frozen candidate/deadlines")
		}
	})
}
