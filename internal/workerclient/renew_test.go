package workerclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func TestRenewalMTLSAndResponses(t *testing.T) {
	ca := testpki.New(t)
	serverCert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	cert, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/node-a/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	token := strings.Repeat("a", 64)
	var mode atomic.Int32
	var calls atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/redirect-must-not-be-followed" {
			t.Error("renewal credential forwarded to redirect")
			w.WriteHeader(403)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/api/v1/worker/renew" || len(r.TLS.VerifiedChains) == 0 {
			t.Error("wrong protocol")
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1024))
		if string(body) != `{"lease_token":"`+token+`"}` {
			t.Error("wrong renewal body")
		}
		w.Header().Set("Content-Type", "application/json")
		switch mode.Load() {
		case 1:
			w.WriteHeader(403)
		case 2:
			w.WriteHeader(409)
		case 3:
			w.WriteHeader(503)
			fmt.Fprint(w, token)
		case 4:
			fmt.Fprint(w, `{}`)
		case 5:
			fmt.Fprint(w, `null`)
		case 6:
			fmt.Fprint(w, strings.Repeat("x", 1025))
		case 7:
			w.Header().Set("Location", "/redirect-must-not-be-followed")
			w.WriteHeader(307)
		case 8:
			<-r.Context().Done()
		case 9:
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, `{"lease_until":"2026-10-27T00:00:00Z"}`)
		case 10:
			fmt.Fprint(w, `{"lease_until":"invalid"}`)
		default:
			fmt.Fprintf(w, `{"lease_until":%q}`, time.Now().Add(30*time.Second).UTC().Format(time.RFC3339Nano))
		}
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots}
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	r, err := NewRenewer(server.URL, ca.Roots, cert)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for i := range int32(11) {
		mode.Store(i)
		before := calls.Load()
		budget := 2 * time.Second
		if i == 8 {
			budget = 100 * time.Millisecond
		}
		ctx, cancel := context.WithTimeout(context.Background(), budget)
		err := r.Renew(ctx, token)
		cancel()
		if (err == nil) != (i == 0) {
			t.Fatalf("mode=%d err=%v", i, err)
		}
		if i == 1 && !errors.Is(err, submission.ErrNode) {
			t.Fatal(err)
		}
		if i == 2 && !errors.Is(err, submission.ErrLease) {
			t.Fatal(err)
		}
		if err != nil && strings.Contains(err.Error(), token) {
			t.Fatal("credential leaked")
		}
		if calls.Load() != before+1 {
			t.Fatal("renewal retried or redirected")
		}
	}
	before := calls.Load()
	if r.Renew(context.Background(), "invalid") == nil || calls.Load() != before {
		t.Fatal("invalid token sent")
	}
	wrong := testpki.New(t)
	untrusted, err := NewRenewer(server.URL, wrong.Roots, cert)
	if err != nil {
		t.Fatal(err)
	}
	defer untrusted.Close()
	if untrusted.Renew(context.Background(), token) == nil || calls.Load() != before {
		t.Fatal("untrusted server accepted")
	}
	for _, endpoint := range []string{"http://localhost", "https://user@localhost", "https://localhost/path", "https://localhost?query", "https://localhost#fragment"} {
		if _, err := NewRenewer(endpoint, ca.Roots, cert); err == nil {
			t.Fatalf("unsafe endpoint accepted: %s", endpoint)
		}
	}
}
