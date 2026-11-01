//go:build integration

package sessionprotocol_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const httpOrigin = "https://session.example.invalid"

func httpServer(t *testing.T, store transport.Store) *httptest.Server {
	t.Helper()
	h, err := transport.New(store, transport.Config{Origin: httpOrigin, Timeout: time.Second})
	must(t, err)
	s := httptest.NewTLSServer(h)
	t.Cleanup(s.Close)
	s.Client().Timeout = 3 * time.Second
	return s
}

func httpCall(t *testing.T, s *httptest.Server, method, path string, c protocol.Credential, origin string, status int, clear bool) []byte {
	t.Helper()
	r, err := http.NewRequest(method, s.URL+path, nil)
	must(t, err)
	// TLS is verified against the synthetic server certificate. Host selects the
	// adapter's fixed public authority; forwarded headers are never consulted.
	r.Host = "session.example.invalid"
	r.AddCookie(&http.Cookie{Name: transport.CookieName, Value: c.Value()})
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	res, err := s.Client().Do(r)
	must(t, err)
	defer func() { must(t, res.Body.Close()) }()
	body, err := io.ReadAll(res.Body)
	must(t, err)
	if res.StatusCode != status || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("HTTPS response status/headers: %d %v", res.StatusCode, res.Header)
	}
	cookies := res.Cookies()
	if (!clear && len(cookies) != 0) || (clear && (len(cookies) != 1 || cookies[0].Name != transport.CookieName || cookies[0].MaxAge != -1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Path != "/" || cookies[0].Domain != "" || cookies[0].SameSite != http.SameSiteLaxMode)) {
		t.Fatal("HTTPS response published/cleared an unexpected cookie")
	}
	if strings.Contains(string(body), c.Value()) || strings.Contains(string(body), "recovery") || strings.Contains(string(body), "auth_generation") {
		t.Fatal("HTTPS response leaked private state")
	}
	return body
}

func testHTTP(t *testing.T, f *fixture) {
	f.open(t)
	t.Run("two TLS replicas preserve polling state and revoke", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		a := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		b := f.store(t, ns, 1, protocol.OrdinaryPolicy())
		attempt, c := f.create(t, a, p)
		servers := [2]*httptest.Server{httpServer(t, a), httpServer(t, b)}
		key := tokenKey(ns, c)
		before, err := f.control.HGetAll(f.ctx, key).Result()
		must(t, err)
		ttl, err := f.control.PExpireTime(f.ctx, key).Result()
		must(t, err)
		for i := range 8 {
			body := httpCall(t, servers[i%2], "GET", "/api/v1/session", c, "", 200, false)
			var view map[string]any
			must(t, json.Unmarshal(body, &view))
			if len(view) != 4 || view["subject_id"] != p.ID {
				t.Fatal("TLS identity response changed ownership")
			}
		}
		httpCall(t, servers[1], "GET", "/api/v1/session/activity", c, "", 405, false)
		httpCall(t, servers[1], "POST", "/api/v1/session/logout", c, "https://evil.invalid", 403, false)
		httpCall(t, servers[1], "POST", "/api/v1/session/activity", c, "", 403, false)
		after, err := f.control.HGetAll(f.ctx, key).Result()
		must(t, err)
		afterTTL, err := f.control.PExpireTime(f.ctx, key).Result()
		must(t, err)
		if !reflect.DeepEqual(before, after) || ttl != afterTTL {
			t.Fatal("polling/rejected HTTP changed session state")
		}
		view, err := a.Read(f.ctx, c)
		must(t, err)
		f.waitTime(t, view.CreatedAt.Add(25*time.Millisecond))
		httpCall(t, servers[1], "POST", "/api/v1/session/activity", c, httpOrigin, 200, false)
		renewed, err := a.Read(f.ctx, c)
		must(t, err)
		if !renewed.IdleExpiresAt.After(view.IdleExpiresAt) || renewed.AbsoluteDeadline != view.AbsoluteDeadline {
			t.Fatal("explicit activity violated expiry rules")
		}
		httpCall(t, servers[0], "POST", "/api/v1/session/logout", c, httpOrigin, 204, true)
		httpCall(t, servers[1], "GET", "/api/v1/session", c, "", 401, false)
		httpCall(t, servers[1], "POST", "/api/v1/session/activity", c, httpOrigin, 401, false)
		wantError(t, b.Retry(f.ctx, attempt), protocol.ErrStale)
	})
	t.Run("lost revoke receipt never clears cookie", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		normal := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		_, c := f.create(t, normal, p)
		h := &hook{op: "abandon", after: func(context.Context, redis.Cmder) error { return errors.New("synthetic receipt loss") }}
		fault := f.faultStore(t, ns, h, time.Second)
		httpCall(t, httpServer(t, fault), "POST", "/api/v1/session/logout", c, httpOrigin, 503, false)
		if !h.used.Load() {
			t.Fatal("failure did not follow a real revoke")
		}
		_, err := normal.Read(f.ctx, c)
		wantError(t, err, protocol.ErrStale)
		httpCall(t, httpServer(t, normal), "GET", "/api/v1/session", c, "", 401, false)
	})
	t.Run("idle expiration and recovery fail closed", func(t *testing.T) {
		p := f.subject(t, false)
		s := f.store(t, namespace(t), 0, protocol.Policy{Idle: 250 * time.Millisecond, Absolute: time.Second})
		_, c := f.create(t, s, p)
		server := httpServer(t, s)
		v, err := s.Read(f.ctx, c)
		must(t, err)
		httpCall(t, server, "GET", "/api/v1/session", c, "", 200, false)
		f.waitTime(t, v.IdleExpiresAt)
		httpCall(t, server, "POST", "/api/v1/session/activity", c, httpOrigin, 401, false)
		httpCall(t, server, "GET", "/api/v1/session", c, "", 401, false)
		other := f.store(t, namespace(t)+"_other", 1, protocol.OrdinaryPolicy())
		_, live := f.create(t, other, p)
		otherServer := httpServer(t, other)
		generation, err := f.identities[0].BeginRecovery(f.ctx)
		must(t, err)
		httpCall(t, otherServer, "GET", "/api/v1/session", live, "", 503, false)
		httpCall(t, otherServer, "POST", "/api/v1/session/logout", live, httpOrigin, 503, false)
		must(t, f.identities[0].FinishRecovery(f.ctx, generation))
		httpCall(t, otherServer, "GET", "/api/v1/session", live, "", 401, false)
	})
	t.Run("disabled account fails across TLS replicas", func(t *testing.T) {
		p := f.subject(t, false)
		ns := namespace(t)
		a := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		b := f.store(t, ns, 1, protocol.OrdinaryPolicy())
		_, c := f.create(t, a, p)
		servers := [2]*httptest.Server{httpServer(t, a), httpServer(t, b)}
		httpCall(t, servers[0], "GET", "/api/v1/session", c, "", 200, false)
		_, err := f.identities[1].DisableCAS(f.ctx, p.ID, p.Revision)
		must(t, err)
		for _, server := range servers {
			httpCall(t, server, "GET", "/api/v1/session", c, "", 401, false)
			httpCall(t, server, "POST", "/api/v1/session/activity", c, httpOrigin, 401, false)
			httpCall(t, server, "POST", "/api/v1/session/logout", c, httpOrigin, 401, false)
		}
	})
}
