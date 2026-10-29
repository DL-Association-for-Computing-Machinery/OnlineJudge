package workerapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

type spyStore struct {
	calls atomic.Int32
	err   error
}

func (s *spyStore) Claim(_ context.Context, n submission.NodeIdentity) (submission.Lease, error) {
	s.calls.Add(1)
	if n.ID != "node-a" || n.CredentialVersion != 1 {
		return submission.Lease{}, submission.ErrNode
	}
	return submission.Lease{}, s.err
}
func (s *spyStore) Renew(context.Context, submission.NodeIdentity, string) (time.Time, error) {
	s.calls.Add(1)
	return time.Now(), s.err
}
func (s *spyStore) Report(context.Context, submission.NodeIdentity, string, submission.Result) error {
	s.calls.Add(1)
	return s.err
}

func TestWorkerBoundary(t *testing.T) {
	ca := testpki.New(t)
	_, certFile, keyFile := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	cfg, err := LoadTLS(ca.CAFile, certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinVersion != tls.VersionTLS13 || cfg.ClientAuth != tls.RequireAndVerifyClientCert || !cfg.SessionTicketsDisabled {
		t.Fatal("unsafe TLS configuration")
	}
	store := &spyStore{err: submission.ErrNoTask}
	server := httptest.NewUnstartedServer(NewHandler(store, cfg.ClientCAs))
	server.TLS = cfg
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	valid, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/node-a/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
	client := testClient(t, ca.Roots, valid)
	request := func(method, path, body, contentType string) int {
		t.Helper()
		req, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		// Cleanup does not change the completed operation or its primary error.
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(resp.Body)
		if resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable response")
		}
		if strings.Contains(string(data), "private-db-error") {
			t.Fatal("internal error leaked")
		}
		return resp.StatusCode
	}
	if got := request("POST", "/api/v1/worker/claim", "{}", "application/json"); got != 204 || store.calls.Load() != 1 {
		t.Fatalf("claim status=%d calls=%d", got, store.calls.Load())
	}
	for _, tc := range []struct {
		name, method, path, body, media string
		status                          int
	}{
		{"forged node", "POST", "claim", `{"node_id":"node-b"}`, "application/json", 400},
		{"null", "POST", "claim", `null`, "application/json", 400},
		{"array", "POST", "claim", `[]`, "application/json", 400},
		{"trailing", "POST", "claim", `{} {}`, "application/json", 400},
		{"empty", "POST", "claim", ``, "application/json", 400},
		{"large", "POST", "claim", `{}` + strings.Repeat(" ", 1024), "application/json", 413},
		{"content type", "POST", "claim", `{}`, "text/plain", 415},
		{"method", "GET", "claim", `{}`, "application/json", 405},
		{"route", "POST", "materials", `{}`, "application/json", 404},
		{"invalid lease", "POST", "renew", `{"lease_token":"x"}`, "application/json", 400},
		{"invalid verdict", "POST", "report", `{"lease_token":"` + strings.Repeat("a", 64) + `","verdict":"OK","evidence_sha256":"` + strings.Repeat("b", 64) + `"}`, "application/json", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := request(tc.method, "/api/v1/worker/"+tc.path, tc.body, tc.media); got != tc.status {
				t.Fatalf("status=%d want=%d", got, tc.status)
			}
		})
	}
	if store.calls.Load() != 1 {
		t.Fatal("invalid request reached store")
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{submission.ErrNode, 403}, {submission.ErrLease, 409}, {submission.ErrResultConflict, 409}, {errors.New("private-db-error"), 503}} {
		store.err = tc.err
		if got := request("POST", "/api/v1/worker/claim", "{}", "application/json"); got != tc.status {
			t.Fatalf("status=%d want=%d", got, tc.status)
		}
	}
	before := store.calls.Load()
	otherCA := testpki.New(t)
	foreign, _, _ := otherCA.Issue(t, "spiffe://onlinejudge/worker/node-a/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
	expired, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/node-a/1", x509.ExtKeyUsageClientAuth, time.Now().Add(-time.Minute))
	wrongUse, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/node-a/1", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Minute))
	for name, pair := range map[string]*tls.Certificate{"missing": nil, "foreign": &foreign, "expired": &expired, "wrong usage": &wrongUse} {
		t.Run(name, func(t *testing.T) {
			var certs []tls.Certificate
			if pair != nil {
				certs = []tls.Certificate{*pair}
			}
			c := testClient(t, ca.Roots, certs...)
			resp, err := c.Post(server.URL+"/api/v1/worker/claim", "application/json", strings.NewReader("{}"))
			if err == nil {
				_ = resp.Body.Close() // Cleanup; the operation outcome is checked separately.
				t.Fatal("invalid certificate passed handshake")
			}
		})
	}
	if store.calls.Load() != before {
		t.Fatal("failed TLS reached store")
	}
	for _, identity := range []string{"", "spiffe://onlinejudge/worker/node-a/0", "spiffe://onlinejudge/worker/node-a/01", "spiffe://onlinejudge/worker/node-a/9223372036854775808", "spiffe://other/worker/node-a/1"} {
		pair, _, _ := ca.Issue(t, identity, x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
		c := testClient(t, ca.Roots, pair)
		resp, err := c.Post(server.URL+"/api/v1/worker/claim", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close() // Cleanup; the operation outcome is checked separately.
		if resp.StatusCode != 401 {
			t.Fatalf("invalid identity status=%d", resp.StatusCode)
		}
	}
	if store.calls.Load() != before {
		t.Fatal("invalid identity reached store")
	}
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/worker/claim", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://example.invalid")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close() // Cleanup; the operation outcome is checked separately.
	if resp.StatusCode != 403 || store.calls.Load() != before {
		t.Fatal("browser request accepted")
	}
	if err := os.Chmod(keyFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTLS(ca.CAFile, certFile, keyFile); err == nil {
		t.Fatal("public key file accepted")
	}
}

func TestRechecksCertificateOnExistingConnection(t *testing.T) {
	ca := testpki.New(t)
	until := time.Now().Add(time.Minute)
	pair, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/node-a/1", x509.ExtKeyUsageClientAuth, until)
	_, cert, key := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	cfg, err := LoadTLS(ca.CAFile, cert, key)
	if err != nil {
		t.Fatal(err)
	}
	var clock atomic.Int64
	clock.Store(time.Now().Unix())
	store := &spyStore{err: submission.ErrNoTask}
	server := httptest.NewUnstartedServer(newHandler(store, ca.Roots, func() time.Time { return time.Unix(clock.Load(), 0) }))
	server.TLS = cfg
	server.StartTLS()
	defer server.Close()
	client := testClient(t, ca.Roots, pair)
	for _, want := range []int{204, 401} {
		reused := false
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/worker/claim", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}))
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close() // Cleanup; the operation outcome is checked separately.
		if resp.StatusCode != want {
			t.Fatalf("status=%d want=%d", resp.StatusCode, want)
		}
		if want == 401 && !reused {
			t.Fatal("expiry was not tested on the existing connection")
		}
		clock.Store(until.Add(time.Second).Unix())
	}
	if store.calls.Load() != 1 {
		t.Fatal("expired request reached store")
	}
	plain := httptest.NewRequest("POST", "/api/v1/worker/claim", strings.NewReader("{}"))
	plain.Header.Set("X-SSL-Client-Verify", "SUCCESS")
	w := httptest.NewRecorder()
	NewHandler(store, ca.Roots).ServeHTTP(w, plain)
	if w.Code != 401 {
		t.Fatal("forwarded identity accepted")
	}
}

func testClient(t *testing.T, roots *x509.CertPool, certs ...tls.Certificate) *http.Client {
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: certs}}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr, Timeout: 3 * time.Second}
}
