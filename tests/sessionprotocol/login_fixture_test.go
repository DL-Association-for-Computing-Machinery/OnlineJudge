//go:build integration

package sessionprotocol_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const loginOrigin = "https://127.0.0.1"

type loginEndpoint struct{ client *http.Client }
type loginResponse struct {
	status  int
	headers http.Header
	cookies []*http.Cookie
	body    []byte
	err     error
}

func loginKey(t *testing.T) [32]byte {
	t.Helper()
	var k [32]byte
	_, err := rand.Read(k[:])
	must(t, err)
	return k
}
func loginManager(t *testing.T, s *protocol.Store, window time.Duration, key [32]byte) *protocol.Login {
	t.Helper()
	l, err := protocol.NewLogin(s, protocol.LoginConfig{Origin: loginOrigin, Window: window, CurrentKey: "fixture", Keys: map[string][32]byte{"fixture": key}})
	must(t, err)
	return l
}
func loginServer(t *testing.T, l *protocol.Login) *loginEndpoint {
	return loginServerBudget(t, l, time.Second)
}

func loginServerBudget(t *testing.T, l *protocol.Login, budget time.Duration) *loginEndpoint {
	t.Helper()
	h, err := transport.NewLogin(l, transport.Config{Origin: loginOrigin, Timeout: budget})
	must(t, err)
	s := httptest.NewTLSServer(h)
	t.Cleanup(s.Close)
	client := s.Client()
	tr := client.Transport.(*http.Transport).Clone()
	// Keep the public URL, TLS verification and Cookie jar on one true origin,
	// while routing each API client to its own disposable loopback TLS replica.
	tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", s.Listener.Addr().String())
	}
	client.Transport = tr
	client.Timeout = 3 * time.Second
	t.Cleanup(tr.CloseIdleConnections)
	return &loginEndpoint{client: client}
}
func loginBodyFor(t *testing.T, p identity.Personal) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"email": p.Email.Original(), "password": "synthetic protocol password"})
	must(t, err)
	return string(b)
}
func loginCall(e *loginEndpoint, path, body string, cookies []*http.Cookie, mutate func(*http.Request)) loginResponse {
	r, err := http.NewRequest("POST", loginOrigin+path, strings.NewReader(body))
	if err != nil {
		return loginResponse{err: err}
	}
	r.Header.Set("Origin", loginOrigin)
	r.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	if mutate != nil {
		mutate(r)
	}
	res, err := e.client.Do(r)
	if err != nil {
		return loginResponse{err: err}
	}
	b, err := io.ReadAll(res.Body)
	closeErr := res.Body.Close()
	if err == nil {
		err = closeErr
	}
	return loginResponse{res.StatusCode, res.Header, res.Cookies(), b, err}
}
func loginStatus(t *testing.T, r loginResponse, status int) {
	t.Helper()
	must(t, r.err)
	if r.status != status {
		t.Fatalf("unexpected login HTTP status: got %d want %d", r.status, status)
	}
	if r.headers.Get("Cache-Control") != "no-store" || r.headers.Get("Access-Control-Allow-Origin") != "" || strings.Contains(string(r.body), "synthetic protocol password") {
		t.Fatal("unsafe login response")
	}
	if status >= 400 && len(r.cookies) != 0 {
		t.Fatal("failed login changed browser cookies")
	}
}
func responseCookie(t *testing.T, r loginResponse, name string) *http.Cookie {
	t.Helper()
	var result *http.Cookie
	for _, c := range r.cookies {
		if c.Name == name {
			if result != nil {
				t.Fatal("duplicate response cookie")
			}
			result = c
		}
	}
	if result == nil {
		t.Fatal("required response cookie absent")
	}
	if !result.Secure || !result.HttpOnly || result.SameSite != http.SameSiteLaxMode || result.Path != "/" || result.Domain != "" {
		t.Fatal("unsafe host cookie scope")
	}
	return result
}
func beginLogin(t *testing.T, e *loginEndpoint, p identity.Personal) *http.Cookie {
	t.Helper()
	r := loginCall(e, "/api/v1/session/login/begin", loginBodyFor(t, p), nil, nil)
	loginStatus(t, r, 202)
	if len(r.cookies) != 1 || len(r.body) != 0 {
		t.Fatal("prepare published session or response data")
	}
	c := responseCookie(t, r, transport.AttemptCookieName)
	if c.MaxAge <= 0 || c.Expires.IsZero() {
		t.Fatal("pending cookie has no bounded lifetime")
	}
	return c
}
func completedCredential(t *testing.T, r loginResponse) protocol.Credential {
	t.Helper()
	loginStatus(t, r, 204)
	if len(r.cookies) != 2 || len(r.body) != 0 {
		t.Fatal("completion cookie contract changed")
	}
	c := responseCookie(t, r, transport.CookieName)
	if c.MaxAge != 0 || !c.Expires.IsZero() {
		t.Fatal("ordinary login cookie became persistent")
	}
	if responseCookie(t, r, transport.AttemptCookieName).MaxAge != -1 {
		t.Fatal("completion retained pending cookie")
	}
	credential, err := protocol.ParseCredential(c.Value)
	must(t, err)
	return credential
}
func namespaceRecords(t *testing.T, f *fixture, ns string) []string {
	t.Helper()
	keys, err := f.control.Keys(f.ctx, "oj-session:"+ns+":token:*").Result()
	must(t, err)
	return keys
}
