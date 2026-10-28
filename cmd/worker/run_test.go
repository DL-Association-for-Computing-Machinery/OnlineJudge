package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

type testHost struct {
	bad         bool
	stopErr     error
	stops       int
	independent bool
}

func (h *testHost) Verify(context.Context, experiment.ExecutionProfile) error { return nil }
func (h *testHost) Stop(ctx context.Context) error {
	h.stops++
	h.independent = ctx.Err() == nil
	return h.stopErr
}
func (h *testHost) RoundTrip(r *http.Request) (*http.Response, error) {
	body := `{"buildVersion":"v1.13.3","os":"linux","platform":"arm64"}`
	if r.URL.Path == "/config" {
		body = `{"runnerConfig":{"cgroupType":2,"seccomp":true,"cgroupControllers":["cpu","memory","pids"]}}`
	}
	if h.bad {
		body = `{}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestSlotPreflightAndShutdown(t *testing.T) {
	for _, scenario := range []string{"idle-cancel", "claim-failure", "protocol-failure", "open-failure", "stop-failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ca := testpki.New(t)
			serverCert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
			cert, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/test/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
			var claims atomic.Int32
			s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				claims.Add(1)
				if r.URL.Path != "/api/v1/worker/claim" {
					t.Error("unexpected request")
				}
				if scenario == "claim-failure" {
					w.WriteHeader(403)
					return
				}
				w.WriteHeader(204)
				cancel()
			}))
			s.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots}
			s.StartTLS()
			defer s.Close()
			path := filepath.Join(t.TempDir(), "slot")
			lock, err := acquireLock(path)
			if err != nil {
				t.Fatal(err)
			}
			h := &testHost{bad: scenario == "protocol-failure"}
			if scenario == "stop-failure" {
				h.stopErr = errors.New("unconfirmed")
			}
			err = runSlot(ctx, config{endpoint: s.URL, roots: ca.Roots, cert: cert}, slog.New(slog.NewTextHandler(io.Discard, nil)), lock, func(context.Context) (sandboxHost, error) {
				if scenario == "open-failure" {
					return nil, errors.New("private error")
				}
				return h, nil
			})
			lock.Close()
			if (err == nil) != (scenario == "idle-cancel") {
				t.Fatalf("unexpected exit: %v", err)
			}
			preflight := scenario == "open-failure" || scenario == "protocol-failure"
			if preflight && claims.Load() != 0 {
				t.Fatal("claimed before preflight")
			}
			if !preflight && claims.Load() != 1 {
				t.Fatalf("claims=%d", claims.Load())
			}
			if scenario != "open-failure" && (h.stops != 1 || !h.independent) {
				t.Fatalf("stop count=%d independent=%v", h.stops, h.independent)
			}
			lock, err = acquireLock(path)
			if lock != nil {
				lock.Close()
			}
			if (err != nil) != (scenario == "stop-failure") {
				t.Fatalf("quarantine: %v", err)
			}
		})
	}
}
