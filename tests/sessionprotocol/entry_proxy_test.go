//go:build integration

package sessionprotocol_test

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testEntryProxy(t *testing.T, f *fixture, binary string) {
	c := runtimeConfig(t, f, namespace(t), true)
	ca := testpki.New(t)
	proxyCert, _, _ := ca.Issue(t, "spiffe://onlinejudge/proxy/entry", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	c.Mode = sessionruntime.ProxyMTLS
	c.Proxy.CertificateSHA256 = [][32]byte{sha256.Sum256(proxyCert.Certificate[0])}
	connections := redisConnections(t, f)
	p := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	bare := entryClient(t, p, ca)
	get := func(r *http.Request) { r.Method = "GET" }
	if result := loginCall(bare, "/api/v1/session", "", nil, get); result.err == nil {
		t.Fatal("mTLS API admitted a connection without client certificate")
	}
	trustedUnpinned, _, _ := ca.Issue(t, "spiffe://onlinejudge/proxy/not-pinned", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	unpinned := entryClient(t, p, ca)
	unpinned.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{trustedUnpinned}
	forward := func(r *http.Request) {
		r.Method = "GET"
		r.Header.Set("X-Forwarded-For", "192.0.2.7")
		r.Header.Set("X-Forwarded-Host", "127.0.0.1")
		r.Header.Set("X-Forwarded-Proto", "https")
	}
	loginStatus(t, loginCall(unpinned, "/api/v1/session", "", nil, forward), 403)
	pinned := entryClient(t, p, ca)
	pinned.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{proxyCert}
	loginStatus(t, loginCall(pinned, "/api/v1/session", "", nil, get), 403)
	loginStatus(t, loginCall(pinned, "/api/v1/session", "", nil, forward), 401)
	wrongCA := testpki.New(t)
	wrongCert, _, _ := wrongCA.Issue(t, "spiffe://onlinejudge/proxy/wrong-ca", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	untrusted := entryClient(t, p, ca)
	untrusted.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{wrongCert}
	if result := loginCall(untrusted, "/api/v1/session", "", nil, forward); result.err == nil {
		t.Fatal("mTLS API admitted an untrusted client certificate")
	}
	edge := entryProxy(t, p, ca, proxyCert)
	x := proxyClient(t, edge, "127.0.0.2", nil)
	subject := f.subject(t, false)
	pending := beginLogin(t, x, subject)
	credential := completedCredential(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
	cookies := []*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}}
	// The real reverse proxy discards caller forwarding values before deriving
	// them from its authenticated HTTPS request and observed client address.
	loginStatus(t, loginCall(x, "/api/v1/session", "", cookies, func(r *http.Request) {
		r.Method = "GET"
		r.Header.Set("X-Forwarded-For", "untrusted,chain")
		r.Header.Set("X-Forwarded-Host", "untrusted.invalid")
		r.Header.Set("X-Forwarded-Proto", "http")
	}), 200)
	loginStatus(t, loginCall(x, "/api/v1/session/logout", "", cookies, nil), 204)
	p.signal(t)
	p.wait(t, true)
	noRuntimeConnections(t, f, c.Database.ApplicationName)
	sameRedisConnections(t, f, connections)
}

func entryProxy(t *testing.T, p *entryProcess, ca *testpki.Authority, cert tls.Certificate) *httptest.Server {
	t.Helper()
	event := p.waitEvent(t, "API starting")
	target, err := url.Parse("https://" + event.Address)
	must(t, err)
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: ca.Roots, Certificates: []tls.Certificate{cert}}}
	t.Cleanup(tr.CloseIdleConnections)
	proxy := &httputil.ReverseProxy{Transport: tr, Rewrite: func(p *httputil.ProxyRequest) {
		p.SetURL(target)
		p.Out.Host = p.In.Host
		p.Out.Header.Del("X-Real-IP")
		p.SetXForwarded()
	}}
	edge := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.Host != "127.0.0.1" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(edge.Close)
	return edge
}
