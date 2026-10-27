package workerclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func TestReportAcknowledgementAndRetry(t *testing.T) {
	ca := testpki.New(t)
	serverCert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	cert, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/node-a/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	token := strings.Repeat("a", 64)
	result := submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("b", 64)}
	var mode, calls atomic.Int32
	var first string
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/v1/worker/report" || r.Method != "POST" || len(r.TLS.VerifiedChains) == 0 {
			t.Error("incorrect transport")
		}
		data, _ := io.ReadAll(r.Body)
		if first == "" {
			first = string(data)
		}
		if first != string(data) {
			t.Error("retry changed payload")
		}
		w.Header().Set("Content-Type", "application/problem+json")
		switch mode.Load() {
		case 0: // Simulate accepted request with a lost response.
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
		case 1:
			w.WriteHeader(204)
		case 2:
			w.WriteHeader(409)
			fmt.Fprint(w, `{"code":"LEASE_INVALID"}`)
		case 3:
			w.WriteHeader(409)
			fmt.Fprint(w, `{"code":"RESULT_CONFLICT"}`)
		case 4:
			w.WriteHeader(403)
		case 5:
			w.WriteHeader(503)
			fmt.Fprint(w, token)
		case 6:
			w.Header().Set("Location", "/unexpected")
			w.WriteHeader(307)
		case 7:
			w.WriteHeader(409)
			fmt.Fprint(w, strings.Repeat("x", 1025))
		case 8:
			w.WriteHeader(409)
			fmt.Fprint(w, `{"code":"unknown"}`)
		case 9:
			w.WriteHeader(200)
		case 10:
			<-r.Context().Done()
		}
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots}
	server.StartTLS()
	defer server.Close()
	reporter, err := NewReporter(server.URL, ca.Roots, cert)
	if err != nil {
		t.Fatal(err)
	}
	defer reporter.Close()
	wants := []error{ErrReportUnknown, nil, submission.ErrLease, submission.ErrResultConflict, submission.ErrNode, ErrReportUnknown, ErrReportUnknown, ErrReportUnknown, ErrReportUnknown, ErrReportUnknown, ErrReportUnknown}
	for i, want := range wants {
		mode.Store(int32(i))
		before := calls.Load()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if i == 10 {
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
		}
		err := reporter.Report(ctx, token, result)
		cancel()
		if !errors.Is(err, want) || calls.Load() != before+1 {
			t.Fatalf("mode=%d error=%v calls=%d", i, err, calls.Load()-before)
		}
		if err != nil && strings.Contains(err.Error(), token) {
			t.Fatal("credential disclosed")
		}
	}
	before := calls.Load()
	if reporter.Report(context.Background(), "invalid", result) != submission.ErrInvalid {
		t.Fatal("invalid token")
	}
	result.Verdict = "unknown"
	if reporter.Report(context.Background(), token, result) != submission.ErrInvalid || calls.Load() != before {
		t.Fatal("invalid result sent")
	}
}
