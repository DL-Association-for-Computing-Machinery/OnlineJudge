package httptransport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const origin = "https://oj.example.invalid"
const token = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type fakeStore struct {
	calls     []string
	err       error
	operation func(context.Context)
}

func (s *fakeStore) call(ctx context.Context, c redisstore.Credential, op string) error {
	s.calls = append(s.calls, op)
	if c.Value() != token {
		return errors.New("unexpected credential")
	}
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("missing call budget")
	}
	if s.operation != nil {
		s.operation(ctx)
	}
	return s.err
}
func (s *fakeStore) Read(ctx context.Context, c redisstore.Credential) (redisstore.View, error) {
	return redisstore.View{SubjectID: strings.Repeat("a", 32), AuthGeneration: 9, RecoveryID: "private recovery value",
		CreatedAt: time.Unix(1000, 0).UTC(), IdleExpiresAt: time.Unix(1100, 0).UTC(), AbsoluteDeadline: time.Unix(1200, 0).UTC()}, s.call(ctx, c, "read")
}
func (s *fakeStore) Touch(ctx context.Context, c redisstore.Credential) (redisstore.View, error) {
	return redisstore.View{}, s.call(ctx, c, "touch")
}
func (s *fakeStore) Revoke(ctx context.Context, c redisstore.Credential) error {
	return s.call(ctx, c, "revoke")
}
func newHandler(t *testing.T, s *fakeStore) http.Handler {
	t.Helper()
	h, err := transport.New(s, transport.Config{Origin: origin, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func request(method, path string) *http.Request {
	r := httptest.NewRequest(method, origin+path, nil)
	r.AddCookie(&http.Cookie{Name: transport.CookieName, Value: token})
	if method == http.MethodPost {
		r.Header.Set("Origin", origin)
	}
	return r
}
func response(t *testing.T, h http.Handler, r *http.Request, status int, clear bool) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("status/headers: %d %v", w.Code, w.Header())
	}
	if got := len(w.Header().Values("Set-Cookie")); (clear && got != 1) || (!clear && got != 0) {
		t.Fatal("unexpected cookie publication/clearing")
	}
	if strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "private recovery") {
		t.Fatal("response disclosed private state")
	}
	return w
}

func TestLifecycle(t *testing.T) {
	s := &fakeStore{}
	h := newHandler(t, s)
	w := response(t, h, request("GET", "/api/v1/session"), 200, false)
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got["subject_id"] != strings.Repeat("a", 32) || got["created_at"] != "1970-01-01T00:16:40Z" {
		t.Fatal("unexpected public identity response")
	}
	response(t, h, request("POST", "/api/v1/session/activity"), 200, false)
	w = response(t, h, request("POST", "/api/v1/session/logout"), 204, true)
	res := w.Result()
	defer func() {
		if err := res.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	cookies := res.Cookies()
	c := cookies[0]
	if c.Name != transport.CookieName || c.Value != "" || c.Path != "/" || c.Domain != "" ||
		!c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.MaxAge != -1 || !c.Expires.Before(time.Now()) {
		t.Fatal("logout cookie does not preserve secure host scope")
	}
	if strings.Join(s.calls, ",") != "read,touch,revoke" || w.Body.Len() != 0 {
		t.Fatal("unexpected lifecycle operations")
	}
}

func TestPreflightRejectsWithoutStorage(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*http.Request)
		status int
	}{
		{"no TLS", func(r *http.Request) { r.TLS = nil; r.Header.Set("X-Forwarded-Proto", "https") }, 403},
		{"wrong Host", func(r *http.Request) { r.Host = "evil.invalid"; r.Header.Set("X-Forwarded-Host", "oj.example.invalid") }, 403},
		{"unknown path", func(r *http.Request) { r.URL.Path = "/other" }, 404},
		{"unsafe GET", func(r *http.Request) { r.Method = "GET" }, 405},
		{"unsafe HEAD", func(r *http.Request) { r.Method = "HEAD" }, 405},
		{"preflight", func(r *http.Request) { r.Method = "OPTIONS" }, 405},
		{"missing proof", func(r *http.Request) { r.Header.Del("Origin") }, 403},
		{"empty Origin", func(r *http.Request) { r.Header.Set("Origin", "") }, 403},
		{"null Origin", func(r *http.Request) { r.Header.Set("Origin", "null") }, 403},
		{"cross Origin", func(r *http.Request) { r.Header.Set("Origin", "https://evil.invalid") }, 403},
		{"HTTP Origin", func(r *http.Request) { r.Header.Set("Origin", "http://oj.example.invalid") }, 403},
		{"duplicate Origin", func(r *http.Request) { r.Header.Add("Origin", origin) }, 403},
		{"empty Origin values", func(r *http.Request) { r.Header["Origin"] = nil }, 403},
		{"comma Origin", func(r *http.Request) { r.Header.Set("Origin", origin+", "+origin) }, 403},
		{"Origin slash", func(r *http.Request) { r.Header.Set("Origin", origin+"/") }, 403},
		{"contradictory site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, 403},
		{"same-site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") }, 403},
		{"none site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "none") }, 403},
		{"unknown site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "unknown") }, 403},
		{"empty site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "") }, 403},
		{"duplicate site", func(r *http.Request) { r.Header["Sec-Fetch-Site"] = []string{"same-origin", "same-origin"} }, 403},
		{"missing cookie", func(r *http.Request) { r.Header.Del("Cookie") }, 401},
		{"empty cookie", func(r *http.Request) { r.Header.Set("Cookie", transport.CookieName+"=") }, 401},
		{"bad token", func(r *http.Request) { r.Header.Set("Cookie", transport.CookieName+"=invalid") }, 401},
		{"noncanonical token", func(r *http.Request) { r.Header.Set("Cookie", transport.CookieName+"="+strings.Repeat("A", 42)+"B") }, 401},
		{"duplicate cookie", func(r *http.Request) { r.Header.Add("Cookie", transport.CookieName+"="+token) }, 401},
		{"duplicate in line", func(r *http.Request) {
			r.Header.Set("Cookie", transport.CookieName+"="+token+"; "+transport.CookieName+"=bad")
		}, 401},
		{"malformed unrelated cookie", func(r *http.Request) { r.Header.Add("Cookie", "broken") }, 401},
		{"malformed quoted cookie", func(r *http.Request) { r.Header.Add("Cookie", transport.CookieName+"=\"broken") }, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &fakeStore{}
			h := newHandler(t, s)
			r := request("POST", "/api/v1/session/activity")
			tc.mutate(r)
			response(t, h, r, tc.status, false)
			if len(s.calls) != 0 {
				t.Fatal("rejected request reached storage")
			}
		})
	}
}

func TestPositiveSourceEvidence(t *testing.T) {
	for _, mode := range []string{"origin", "site", "both"} {
		t.Run(mode, func(t *testing.T) {
			s := &fakeStore{}
			h := newHandler(t, s)
			r := request("POST", "/api/v1/session/activity")
			if mode == "site" {
				r.Header.Del("Origin")
			}
			if mode != "origin" {
				r.Header.Set("Sec-Fetch-Site", "same-origin")
			}
			r.Header.Add("Cookie", "other=value")
			response(t, h, r, 200, false)
			if len(s.calls) != 1 || s.calls[0] != "touch" {
				t.Fatal("source proof was not accepted")
			}
		})
	}
}

func TestErrorsNeverClearOrPublishCookies(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"stale", redisstore.ErrStale, 401}, {"absent", redisstore.ErrNotFound, 401},
		{"account changed", identity.ErrConflict, 401}, {"account absent", identity.ErrNotFound, 401},
		{"recovering", identity.ErrRecovering, 503}, {"store", redisstore.ErrStore, 503},
		{"identity store", identity.ErrStore, 503}, {"guarded stale", errors.Join(identity.ErrGuardedOperation, redisstore.ErrStale), 401},
		{"guarded absent", errors.Join(identity.ErrGuardedOperation, redisstore.ErrNotFound), 401},
		{"guarded identity conflict", errors.Join(identity.ErrGuardedOperation, identity.ErrConflict), 503},
		{"guarded identity absent", errors.Join(identity.ErrGuardedOperation, identity.ErrNotFound), 503},
		{"guarded", identity.ErrGuardedOperation, 503},
		{"commit conflict", errors.Join(identity.ErrStore, identity.ErrConflict), 503},
		{"unknown", errors.New("private recovery value"), 503}, {"cancelled", context.Canceled, 503},
		{"deadline", context.DeadlineExceeded, 503}, {"corrupt", redisstore.ErrConflict, 503},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range []string{"/api/v1/session", "/api/v1/session/activity", "/api/v1/session/logout"} {
				s := &fakeStore{err: tc.err}
				method := "POST"
				if path == "/api/v1/session" {
					method = "GET"
				}
				w := response(t, newHandler(t, s), request(method, path), tc.status, false)
				if w.Header().Get("Content-Type") != "application/problem+json" || len(s.calls) != 1 {
					t.Fatal("incorrect failure boundary")
				}
			}
		})
	}
}

func TestUncertainErrorsOverrideProtocolRefusal(t *testing.T) {
	for _, failure := range []error{redisstore.ErrStore, identity.ErrStore, identity.ErrRecovering, redisstore.ErrUnknown, context.Canceled, context.DeadlineExceeded} {
		for _, path := range []string{"/api/v1/session", "/api/v1/session/activity", "/api/v1/session/logout"} {
			s := &fakeStore{err: errors.Join(identity.ErrGuardedOperation, redisstore.ErrStale, failure)}
			method := "POST"
			if path == "/api/v1/session" {
				method = "GET"
			}
			response(t, newHandler(t, s), request(method, path), 503, false)
		}
	}
}

func TestCooperativeTimeout(t *testing.T) {
	s := &fakeStore{operation: func(ctx context.Context) { <-ctx.Done() }}
	h, err := transport.New(s, transport.Config{Origin: origin, Timeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	response(t, h, request("POST", "/api/v1/session/logout"), 503, false)
	if len(s.calls) != 1 {
		t.Fatal("timed operation did not execute")
	}
}

func TestCancellationAfterSuccessfulEffectIsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &fakeStore{operation: func(context.Context) { cancel() }}
	r := request("POST", "/api/v1/session/logout").WithContext(ctx)
	response(t, newHandler(t, s), r, 503, false)
	if len(s.calls) != 1 {
		t.Fatal("effect did not execute")
	}
}

func TestConfiguration(t *testing.T) {
	for _, value := range []string{"", "http://oj.example.invalid", origin + "/", origin + "/path", origin + "?", origin + "#", origin + "?q=1", origin + "#fragment", "https://user@oj.example.invalid", "https://"} {
		if h, err := transport.New(&fakeStore{}, transport.Config{Origin: value, Timeout: time.Second}); err == nil || h != nil {
			t.Fatal("accepted invalid origin")
		}
	}
	for _, budget := range []time.Duration{0, -time.Second, 6 * time.Second} {
		if _, err := transport.New(&fakeStore{}, transport.Config{Origin: origin, Timeout: budget}); err == nil {
			t.Fatal("accepted unbounded budget")
		}
	}
	if _, err := transport.New(nil, transport.Config{Origin: origin, Timeout: time.Second}); err == nil {
		t.Fatal("accepted absent Store")
	}
}
