//go:build integration

package sessionprotocol_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

type proxyEndpoint struct {
	backend *httptest.Server
	edge    *httptest.Server
}

func proxyServer(t *testing.T, h http.Handler, ca *testpki.Authority, cert tls.Certificate) proxyEndpoint {
	t.Helper()
	backend := httptest.NewUnstartedServer(h)
	backend.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ClientCAs: ca.Roots, ClientAuth: tls.RequireAndVerifyClientCert}
	backend.StartTLS()
	t.Cleanup(backend.Close)
	tr := backend.Client().Transport.(*http.Transport).Clone()
	tr.TLSClientConfig.Certificates = []tls.Certificate{cert}
	t.Cleanup(tr.CloseIdleConnections)
	target, err := url.Parse(backend.URL)
	must(t, err)
	proxy := &httputil.ReverseProxy{Transport: tr, Rewrite: func(p *httputil.ProxyRequest) {
		p.SetURL(target)
		p.Out.Host = p.In.Host
		// Rewrite runs after hop-by-hop and standard forwarded-header removal.
		// Never copy inbound XFF: SetXForwarded would append untrusted values.
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
	return proxyEndpoint{backend, edge}
}

func proxyClient(t *testing.T, server *httptest.Server, source string, cert *tls.Certificate) *loginEndpoint {
	t.Helper()
	// Server.Client returns the same pointer on every call. Copy it so distinct
	// source transports and client-certificate choices cannot overwrite each other.
	client := *server.Client()
	tr := client.Transport.(*http.Transport).Clone()
	if cert != nil {
		tr.TLSClientConfig.Certificates = []tls.Certificate{*cert}
	}
	tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		d := &net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(source)}}
		return d.DialContext(ctx, "tcp", server.Listener.Addr().String())
	}
	client.Transport = tr
	client.Timeout = 3 * time.Second
	t.Cleanup(tr.CloseIdleConnections)
	return &loginEndpoint{&client}
}
