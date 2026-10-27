package main

import (
	"context"
	"crypto/x509"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func TestConfigurationFailsClosed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, addr := range []string{"", "0.0.0.0:8443", "localhost:8443", "[::]:8443"} {
		t.Setenv("OJ_WORKER_API_ADDR", addr)
		if err := run(context.Background(), logger); err == nil {
			t.Fatalf("address %q accepted", addr)
		}
	}
	t.Setenv("OJ_WORKER_API_ADDR", "127.0.0.1:8443")
	t.Setenv("OJ_WORKER_KEY_FILE", "")
	if err := run(context.Background(), logger); err == nil {
		t.Fatal("missing TLS accepted")
	}
	ca := testpki.New(t)
	_, cert, key := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	t.Setenv("OJ_WORKER_CA_FILE", ca.CAFile)
	t.Setenv("OJ_WORKER_CERT_FILE", cert)
	t.Setenv("OJ_WORKER_KEY_FILE", key)
	t.Setenv("OJ_DATABASE_URL", "")
	if err := run(context.Background(), logger); err == nil {
		t.Fatal("missing database accepted")
	}
}
