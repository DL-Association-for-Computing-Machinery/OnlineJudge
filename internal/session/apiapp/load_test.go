package apiapp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/apiconfig"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func canonical(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFixedCredentialSources(t *testing.T) {
	for _, tc := range []struct {
		mode, value string
		present, ok bool
	}{
		{"none", "", false, true}, {"none", "", true, true}, {"none", "synthetic-secret", true, false},
		{"env", "synthetic-secret", true, true}, {"env", "", false, false}, {"env", "", true, false}, {"OTHER", "synthetic-secret", true, false},
	} {
		value, err := password(tc.mode, "OJ_SESSION_DATABASE_PASSWORD", func(name string) (string, bool) {
			if name != "OJ_SESSION_DATABASE_PASSWORD" {
				t.Error("arbitrary credential name requested")
			}
			return tc.value, tc.present
		})
		if (err == nil) != tc.ok || (err != nil && (!errors.Is(err, ErrConfiguration) || strings.Contains(err.Error(), "synthetic-secret"))) || (err == nil && tc.mode == "env" && value != tc.value) {
			t.Fatal("credential mode did not fail closed")
		}
	}
}

func TestTLSMaterialPolicyAndTrust(t *testing.T) {
	ca := testpki.New(t)
	_, cert, key := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	m := apiconfig.Material{CertificateFile: canonical(t, cert), KeyFile: canonical(t, key)}
	c := apiconfig.Config{TLS: m}
	c.Runtime.Mode = sessionruntime.DirectTLS
	c.Runtime.Database.Security = sessionruntime.PrivateTransport
	c.Runtime.RedisSecurity = sessionruntime.PrivateTransport
	server, err := loadTLS(&c)
	if err != nil || len(server.Certificates) != 1 || server.ClientAuth != tls.NoClientCert || server.MinVersion != tls.VersionTLS12 {
		t.Fatal("direct TLS preparation failed")
	}
	c.Runtime.Mode = sessionruntime.ProxyMTLS
	c.TLS.CAFile = canonical(t, ca.CAFile)
	server, err = loadTLS(&c)
	if err != nil || server.ClientAuth != tls.RequireAndVerifyClientCert || server.ClientCAs == nil {
		t.Fatal("proxy did not require verified client certificates")
	}
	for _, mode := range []os.FileMode{0644, 0700} {
		if err := os.Chmod(key, mode); err != nil {
			t.Fatal(err)
		}
		if _, err := keyPair(m, true); err != ErrTLS {
			t.Fatal("unsafe private TLS key accepted")
		}
	}
	if err := os.Chmod(key, 0600); err != nil {
		t.Fatal(err)
	}
	_, clientCert, clientKey := ca.Issue(t, "", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	clientMaterial := apiconfig.Material{CertificateFile: canonical(t, clientCert), KeyFile: canonical(t, clientKey), CAFile: canonical(t, ca.CAFile), ServerName: "127.0.0.1"}
	client, err := storageTLS(sessionruntime.VerifiedTLS, clientMaterial)
	if err != nil || client.InsecureSkipVerify || client.RootCAs == nil || client.ServerName != "127.0.0.1" || len(client.Certificates) != 1 {
		t.Fatal("storage TLS lost explicit trust")
	}
	if _, err := keyPair(clientMaterial, true); err != ErrTLS {
		t.Fatal("client-only certificate accepted as server")
	}
	_, expiredCert, expiredKey := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(-time.Minute))
	if _, err := keyPair(apiconfig.Material{CertificateFile: canonical(t, expiredCert), KeyFile: canonical(t, expiredKey)}, true); err != ErrTLS {
		t.Fatal("expired server certificate accepted")
	}
	link := filepath.Join(filepath.Dir(key), "linked-key")
	if err := os.Symlink(key, link); err != nil {
		t.Fatal(err)
	}
	m.KeyFile = link
	if _, err := keyPair(m, true); err != ErrTLS {
		t.Fatal("symbolic TLS key accepted")
	}
}

func TestTransportDiagnosticsNeverWriteOriginalInput(t *testing.T) {
	var buffer bytes.Buffer
	w := safeHTTPLog{slog.New(slog.NewJSONHandler(&buffer, nil))}
	marker := []byte("synthetic-secret in transport diagnostics")
	n, err := w.Write(marker)
	if err != nil || n != len(marker) || strings.Contains(buffer.String(), "synthetic-secret") || !strings.Contains(buffer.String(), "session HTTP transport failure") {
		t.Fatal("raw transport diagnostics published")
	}
	for _, address := range []string{"", "localhost", "127.0.0.1:", "127.0.0.1:https", "127.0.0.1:65536"} {
		if validAddress(address) {
			t.Fatal("implicit/invalid listener address accepted")
		}
	}
	for _, address := range []string{"127.0.0.1:0", ":8443", "[::1]:443"} {
		if !validAddress(address) {
			t.Fatal("explicit listener address rejected")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Run(ctx, slog.Default(), "127.0.0.1:0", "/never-read", func(string) (string, bool) { return "", false }); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled startup continued")
	}
}
