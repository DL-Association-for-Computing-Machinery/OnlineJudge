package httptransport

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func TestProxyTrust(t *testing.T) {
	ca := testpki.New(t)
	pair, _, _ := ca.Issue(t, "", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	otherPair, _, _ := ca.Issue(t, "", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	other, err := x509.ParseCertificate(otherPair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	h := &handler{host: "oj.example.invalid", proxy: &proxyTrust{certificates: map[[32]byte]struct{}{sha256.Sum256(leaf.Raw): {}}}}
	request := func() *http.Request {
		r := httptest.NewRequest("GET", "https://oj.example.invalid/api/v1/session", nil)
		// Synthetic TLS state is solely a unit boundary fixture. Integration tests
		// separately require actual TLS handshakes and certificate verification.
		r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
		r.Header.Set("X-Forwarded-Host", h.host)
		r.Header.Set("X-Forwarded-Proto", "https")
		r.Header.Set("X-Forwarded-For", "192.0.2.1")
		return r
	}
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
	}{
		{"plaintext", func(r *http.Request) { r.TLS = nil }},
		{"no certificate", func(r *http.Request) { r.TLS.PeerCertificates = nil }},
		{"nil certificate", func(r *http.Request) { r.TLS.PeerCertificates[0] = nil }},
		{"unverified", func(r *http.Request) { r.TLS.VerifiedChains = nil }},
		{"empty chain", func(r *http.Request) { r.TLS.VerifiedChains = [][]*x509.Certificate{{}} }},
		{"nil chain leaf", func(r *http.Request) { r.TLS.VerifiedChains = [][]*x509.Certificate{{nil}} }},
		{"different verified leaf", func(r *http.Request) { r.TLS.VerifiedChains = [][]*x509.Certificate{{other}} }},
		{"verified unpinned", func(r *http.Request) {
			r.TLS.PeerCertificates[0] = other
			r.TLS.VerifiedChains = [][]*x509.Certificate{{other}}
		}},
		{"host", func(r *http.Request) { r.Host = "evil.invalid" }},
		{"forwarded host", func(r *http.Request) { r.Header.Set("X-Forwarded-Host", "evil.invalid") }},
		{"duplicate host", func(r *http.Request) { r.Header.Add("X-Forwarded-Host", h.host) }},
		{"missing host", func(r *http.Request) { r.Header.Del("X-Forwarded-Host") }},
		{"http", func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "http") }},
		{"duplicate scheme", func(r *http.Request) { r.Header.Add("X-Forwarded-Proto", "https") }},
		{"missing scheme", func(r *http.Request) { r.Header.Del("X-Forwarded-Proto") }},
		{"forwarded dialect", func(r *http.Request) { r.Header.Set("Forwarded", "") }},
		{"real IP dialect", func(r *http.Request) { r.Header.Set("X-Real-IP", "") }},
		{"missing source", func(r *http.Request) { r.Header.Del("X-Forwarded-For") }},
		{"duplicate source", func(r *http.Request) { r.Header.Add("X-Forwarded-For", "192.0.2.1") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := request()
			tc.mutate(r)
			if source, ok := h.requestSource(r); ok || source.IsValid() {
				t.Fatal("untrusted proxy metadata accepted")
			}
			// No store is installed: reaching any lifecycle store operation panics.
			for _, path := range []string{"/api/v1/session", "/api/v1/session/activity", "/api/v1/session/logout", "/api/v1/session/login/begin", "/api/v1/session/login/complete", "/api/v1/session/login/abandon"} {
				r.URL.Path = path
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != 403 || len(w.Header().Values("Set-Cookie")) != 0 || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("proxy rejection reached routing or changed cookies")
				}
			}
		})
	}
	for _, raw := range []string{"", "192.0.2.1, 192.0.2.2", "192.0.2.1:80", "[::1]", "fe80::1%eth0", "unknown", " 192.0.2.1", "192.0.2.1 ", "0.0.0.0", "::", "::ffff:0.0.0.0", "224.0.0.1", "::ffff:224.0.0.1", "ff02::1"} {
		t.Run("bad IP "+raw, func(t *testing.T) {
			r := request()
			r.Header.Set("X-Forwarded-For", raw)
			if _, ok := h.requestSource(r); ok {
				t.Fatal("invalid proxy source accepted")
			}
		})
	}
	for _, raw := range []string{"192.0.2.1", "::ffff:192.0.2.1", "2001:db8::1"} {
		t.Run("valid IP "+raw, func(t *testing.T) {
			r := request()
			r.Header.Set("X-Forwarded-For", raw)
			before := r.RemoteAddr
			source, ok := h.requestSource(r)
			if !ok || source != netip.MustParseAddr(raw).Unmap() || r.RemoteAddr != before || r.TLS.PeerCertificates[0] != leaf {
				t.Fatal("validated source or original socket/TLS changed")
			}
		})
	}
}

func TestProxyConstructorRejectsMissingManager(t *testing.T) {
	if h, err := NewProxiedLimitedLogin(nil, Config{}, ProxyConfig{CertificateSHA256: [][32]byte{{1}}}); h != nil || err == nil {
		t.Fatal("missing limited manager accepted")
	}
}
