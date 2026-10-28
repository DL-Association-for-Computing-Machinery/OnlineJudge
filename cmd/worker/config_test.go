package main

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func TestConfig(t *testing.T) {
	ca := testpki.New(t)
	_, cert, key := ca.Issue(t, "spiffe://onlinejudge/worker/test/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	base := map[string]string{"OJ_SANDBOX_EXPERIMENT": "1", "OJ_WORKER_CONTROLLER_URL": "https://127.0.0.1:8443",
		"OJ_WORKER_CA_FILE": ca.CAFile, "OJ_WORKER_CERT_FILE": cert, "OJ_WORKER_KEY_FILE": key}
	for _, tc := range []struct {
		name, key, value string
		ok               bool
	}{
		{"valid", "", "", true},
		{"primary-slot", "OJ_WORKER_M0_SLOT", "primary", true},
		{"secondary-slot", "OJ_WORKER_M0_SLOT", "secondary", true},
		{"slot-alias", "OJ_WORKER_M0_SLOT", "oj-m0-sandbox", false},
		{"slot-path", "OJ_WORKER_M0_SLOT", "../secret", false},
		{"opt-in", "OJ_SANDBOX_EXPERIMENT", "", false},
		{"plaintext", "OJ_WORKER_CONTROLLER_URL", "http://127.0.0.1", false},
		{"external", "OJ_WORKER_CONTROLLER_URL", "https://192.0.2.1", false},
		{"dns", "OJ_WORKER_CONTROLLER_URL", "https://localhost", false},
		{"userinfo", "OJ_WORKER_CONTROLLER_URL", "https://secret@127.0.0.1", false},
		{"query", "OJ_WORKER_CONTROLLER_URL", "https://127.0.0.1?secret", false},
		{"path", "OJ_WORKER_CONTROLLER_URL", "https://127.0.0.1/secret", false},
		{"key-missing", "OJ_WORKER_KEY_FILE", "/secret-missing", false},
		{"ca-missing", "OJ_WORKER_CA_FILE", "/secret-missing", false},
		{"bad-ca", "OJ_WORKER_CA_FILE", key, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadConfig(func(k string) string {
				if k == tc.key {
					return tc.value
				}
				return base[k]
			})
			if (err == nil) != tc.ok {
				t.Fatalf("ok=%v: %v", tc.ok, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("configuration leaked")
			}
		})
	}
	if err := os.Chmod(key, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(func(k string) string { return base[k] }); err == nil {
		t.Fatal("public key file accepted")
	}
	if err := os.Chmod(key, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "key-link")
	if err := os.Symlink(key, link); err != nil {
		t.Fatal(err)
	}
	base["OJ_WORKER_KEY_FILE"] = link
	if _, err := loadConfig(func(k string) string { return base[k] }); err == nil {
		t.Fatal("key symlink accepted")
	}
}
