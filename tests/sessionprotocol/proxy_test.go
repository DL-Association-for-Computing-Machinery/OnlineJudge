//go:build integration

package sessionprotocol_test

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testProxy(t *testing.T, f *fixture) {
	ca := testpki.New(t)
	cert, _, _ := ca.Issue(t, "spiffe://onlinejudge/proxy/fixture", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	other, _, _ := ca.Issue(t, "spiffe://onlinejudge/proxy/other", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	var probes [2]*loginAuthorityProbe
	newHandlers := func(t *testing.T, ns string, limits protocol.LoginLimits) [2]http.Handler {
		t.Helper()
		secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
		var handlers [2]http.Handler
		for i := range 2 {
			probes[i] = &loginAuthorityProbe{Authority: f.identities[i]}
			store, err := protocol.New(f.ctx, f.clients[i], probes[i], f.budget, ns, protocol.OrdinaryPolicy())
			must(t, err)
			l := limitedManager(t, f, store, secrets, limits, i == 0)
			pins := [][32]byte{sha256.Sum256(cert.Certificate[0])}
			h, err := transport.NewProxiedLimitedLogin(l, transport.Config{Origin: loginOrigin, Timeout: time.Second}, transport.ProxyConfig{CertificateSHA256: pins})
			must(t, err)
			pins[0] = sha256.Sum256(other.Certificate[0]) // Construction must freeze trust.
			handlers[i] = h
		}
		return handlers
	}
	t.Run("proxy constructor validates trust with a valid manager", func(t *testing.T) {
		s := f.store(t, namespace(t), 0, protocol.OrdinaryPolicy())
		l := limitedManager(t, f, s, loadedLoginSecrets(t, loginKey(t), loginKey(t)), rateLimits(), true)
		config := transport.Config{Origin: loginOrigin, Timeout: time.Second}
		pin := sha256.Sum256(cert.Certificate[0])
		var tooMany [][32]byte
		for i := range 17 {
			tooMany = append(tooMany, sha256.Sum256([]byte{byte(i)}))
		}
		for _, tc := range []struct {
			name string
			pins [][32]byte
		}{
			{"empty", nil}, {"zero", [][32]byte{{}}},
			{"duplicate", [][32]byte{pin, pin}}, {"too many", tooMany},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if h, err := transport.NewProxiedLimitedLogin(l, config, transport.ProxyConfig{CertificateSHA256: tc.pins}); err == nil || h != nil {
					t.Fatal("invalid proxy pins accepted with valid manager")
				}
			})
		}
		var maxPins [][32]byte
		for i := range 16 {
			maxPins = append(maxPins, sha256.Sum256([]byte{byte(i)}))
		}
		_, err := transport.NewProxiedLimitedLogin(l, config, transport.ProxyConfig{CertificateSHA256: maxPins})
		must(t, err)
		unlimited := loginManager(t, s, time.Minute, loginKey(t))
		if h, err := transport.NewProxiedLimitedLogin(unlimited, config, transport.ProxyConfig{CertificateSHA256: [][32]byte{pin}}); err == nil || h != nil {
			t.Fatal("proxy constructor accepted manager without shared admission")
		}
	})
	t.Run("real proxy mTLS and cross replica cookie lifecycle", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		h := newHandlers(t, ns, rateLimits())
		a, b := proxyServer(t, h[0], ca, cert), proxyServer(t, h[1], ca, cert)
		x, y := proxyClient(t, a.edge, "127.0.0.2", nil), proxyClient(t, b.edge, "127.0.0.2", nil)
		jar, err := cookiejar.New(nil)
		must(t, err)
		x.client.Jar, y.client.Jar = jar, jar
		beginLogin(t, x, p)
		if len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("proxied prepare wrote a session")
		}
		credential := completedCredential(t, loginCall(y, "/api/v1/session/login/complete", "", nil, nil))
		for _, endpoint := range []*loginEndpoint{x, y} {
			loginStatus(t, loginCall(endpoint, "/api/v1/session", "", nil, func(r *http.Request) { r.Method = "GET" }), 200)
		}
		loginStatus(t, loginCall(x, "/api/v1/session/activity", "", nil, nil), 200)
		loginStatus(t, loginCall(y, "/api/v1/session/logout", "", nil, func(r *http.Request) { r.Header.Set("Origin", "https://evil.invalid") }), 403)
		loginStatus(t, loginCall(y, "/api/v1/session/logout", "", nil, nil), 204)
		loginStatus(t, loginCall(x, "/api/v1/session", "", []*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}}, func(r *http.Request) { r.Method = "GET" }), 401)
		u, err := url.Parse(loginOrigin)
		must(t, err)
		if len(jar.Cookies(u)) != 0 {
			t.Fatal("proxied logout left credentials in shared cookie jar")
		}
	})
	t.Run("actual source resists spoofed forwarding across replicas", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, true)
		limits := rateLimits()
		limits.BeginSource.Count = 1
		h := newHandlers(t, ns, limits)
		a, b := proxyServer(t, h[0], ca, cert), proxyServer(t, h[1], ca, cert)
		x, y := proxyClient(t, a.edge, "127.0.0.2", nil), proxyClient(t, b.edge, "127.0.0.2", nil)
		z := proxyClient(t, b.edge, "127.0.0.3", nil)
		wrong := strings.Replace(loginBodyFor(t, p), "synthetic protocol password", "incorrect password", 1)
		spoof := func(r *http.Request) {
			r.Header.Set("Forwarded", "for=198.51.100.1;proto=http;host=evil.invalid")
			r.Header.Set("X-Forwarded-For", "198.51.100.1, 198.51.100.2")
			r.Header.Set("X-Forwarded-Proto", "http")
			r.Header.Set("X-Forwarded-Host", "evil.invalid")
			r.Header.Set("X-Real-IP", "198.51.100.1")
			r.Header.Set("Connection", "X-Forwarded-For, X-Forwarded-Host, X-Forwarded-Proto")
		}
		loginStatus(t, loginCall(x, "/api/v1/session/login/begin", wrong, nil, spoof), 401)
		before := rateSnapshot(t, f, ns)
		loginStatus(t, loginCall(y, "/api/v1/session/login/begin", loginBodyFor(t, p), nil, spoof), 429)
		unchangedRate(t, f, ns, before)
		if probes[1].entries.Load() != 0 {
			t.Fatal("throttled proxy request reached credential gate")
		}
		loginStatus(t, loginCall(z, "/api/v1/session/login/begin", wrong, nil, spoof), 401)
		var version int64
		must(t, f.pools[0].QueryRow(f.ctx, `SELECT credential_version FROM identity_credentials WHERE id=$1`, p.ID).Scan(&version))
		if version != 1 || len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("throttled proxied password reached upgrade or session creation")
		}
	})
	t.Run("completion and abandonment share the authenticated source budget", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		limits := rateLimits()
		limits.RecoverySource.Count = 1
		h := newHandlers(t, ns, limits)
		a, b := proxyServer(t, h[0], ca, cert), proxyServer(t, h[1], ca, cert)
		x, y := proxyClient(t, a.edge, "127.0.0.2", nil), proxyClient(t, b.edge, "127.0.0.2", nil)
		z := proxyClient(t, a.edge, "127.0.0.3", nil)
		pending := beginLogin(t, x, p)
		credential := completedCredential(t, loginCall(y, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
		before := rateSnapshot(t, f, ns)
		loginStatus(t, loginCall(x, "/api/v1/session/login/abandon", "", []*http.Cookie{pending}, nil), 429)
		unchangedRate(t, f, ns, before)
		live := &http.Cookie{Name: transport.CookieName, Value: credential.Value()}
		loginStatus(t, loginCall(y, "/api/v1/session", "", []*http.Cookie{live}, func(r *http.Request) { r.Method = "GET" }), 200)
		loginStatus(t, loginCall(z, "/api/v1/session/login/abandon", "", []*http.Cookie{pending}, nil), 204)
		loginStatus(t, loginCall(y, "/api/v1/session", "", []*http.Cookie{live}, func(r *http.Request) { r.Method = "GET" }), 401)
	})
	t.Run("real verified but unpinned proxy and missing mTLS fail closed", func(t *testing.T) {
		ns := namespace(t)
		h := newHandlers(t, ns, rateLimits())
		p := f.subject(t, false)
		backend := proxyServer(t, h[0], ca, cert).backend
		before := rateSnapshot(t, f, ns)
		for _, pair := range []*tls.Certificate{&other, nil} {
			x := proxyClient(t, backend, "127.0.0.2", pair)
			res := loginCall(x, "/api/v1/session/login/begin", loginBodyFor(t, p), nil, func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "127.0.0.2")
				r.Header.Set("X-Forwarded-Host", "127.0.0.1")
				r.Header.Set("X-Forwarded-Proto", "https")
			})
			if pair == nil {
				if res.err == nil {
					t.Fatal("backend accepted TLS connection without client certificate")
				}
			} else {
				loginStatus(t, res, 403)
			}
		}
		unchangedRate(t, f, ns, before)
		if len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("untrusted proxy wrote sessions")
		}
	})
}
