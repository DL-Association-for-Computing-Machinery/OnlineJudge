package localdev

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestMaterialsPrivateAndNonOverwriting(t *testing.T) {
	dir := t.TempDir()
	if err := generate(dir); err != nil {
		t.Fatal("generation failed")
	}
	before, err := materialHashes(dir)
	if err != nil {
		t.Fatal("materials unreadable")
	}
	pair, err := certificate(dir)
	if err != nil {
		t.Fatal("certificate cannot verify localhost")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || len(leaf.AuthorityKeyId) == 0 {
		t.Fatal("strict TLS clients need the authority key identifier")
	}
	if _, err = redisstore.LoadLoginSecrets(filepath.Join(dir, "login-keys.json")); err != nil {
		t.Fatal("generated secrets rejected by real loader")
	}
	if _, err = readAccount(dir); err != nil {
		t.Fatal("account unreadable")
	}
	for _, name := range materialNames {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("material is not owner-private")
		}
	}
	if generate(dir) == nil {
		t.Fatal("existing materials overwritten")
	}
	after, err := materialHashes(dir)
	if err != nil {
		t.Fatal("materials changed")
	}
	for name, digest := range before {
		if after[name] != digest {
			t.Fatal("failed regeneration changed a material")
		}
	}
}

func TestPartialStateAndSymlinksFailClosed(t *testing.T) {
	dir := t.TempDir()
	if writeNew(dir, "interrupted", []byte("incomplete")) != nil {
		t.Fatal("setup")
	}
	if Initialize(context.Background(), dir) == nil {
		t.Fatal("partial install admitted")
	}
	if _, err := os.Stat(filepath.Join(dir, "interrupted")); err != nil {
		t.Fatal("partial data removed")
	}
	link := filepath.Join(t.TempDir(), "state")
	if os.Symlink(dir, link) != nil {
		t.Fatal("symlink setup")
	}
	if Initialize(context.Background(), link) == nil {
		t.Fatal("symlink state admitted")
	}
}

func TestMaterialPermissionsAndInvalidTLSFailClosed(t *testing.T) {
	dir := t.TempDir()
	if generate(dir) != nil {
		t.Fatal("setup")
	}
	path := filepath.Join(dir, "account.json")
	if os.Chmod(path, 0644) != nil {
		t.Fatal("chmod")
	}
	if _, err := readAccount(dir); err == nil {
		t.Fatal("public credentials accepted")
	}
	if os.WriteFile(filepath.Join(dir, "server.key"), []byte("not a private key"), 0600) != nil {
		t.Fatal("key setup")
	}
	if _, err := certificate(dir); err == nil {
		t.Fatal("bad key accepted")
	}
}

func TestQuickEntryDelegatesOnlyRealBeginWithOriginalTLS(t *testing.T) {
	a := account{Email: "developer@example.invalid", Password: "synthetic server-only credential"}
	var calls atomic.Int32
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.TLS == nil || !r.TLS.HandshakeComplete || r.Host != "localhost:8443" || r.RemoteAddr == "" {
			t.Error("lost real transport identity")
		}
		if r.URL.Path != "/api/v1/session/login/begin" || r.Method != "POST" || r.Header.Get("Origin") != Origin || r.Header.Get("Cookie") != "pending=synthetic" {
			t.Error("lost ordinary protocol evidence")
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["email"] != a.Email || body["password"] != a.Password {
			t.Error("wrong private input")
		}
		w.Header().Add("Set-Cookie", "pending=continued; Secure; HttpOnly; Path=/")
		w.WriteHeader(202)
	})
	server := httptest.NewTLSServer(handler(api, a, t.TempDir()))
	defer server.Close()
	req, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/__local/login", nil)
	if err != nil {
		t.Fatal("request")
	}
	req.Host = "localhost:8443"
	req.Header.Set("Origin", Origin)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Cookie", "pending=synthetic")
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal("real TLS call failed")
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != 202 || len(body) != 0 || len(res.Cookies()) != 1 || calls.Load() != 1 {
		t.Fatal("quick entry did more than standard begin or lost continuation")
	}
}

func TestQuickEntryRejectsUntrustedTransportAndInputs(t *testing.T) {
	var calls atomic.Int32
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(202) })
	h := handler(api, account{}, t.TempDir())
	plain := httptest.NewRecorder()
	h.ServeHTTP(plain, httptest.NewRequest("POST", Origin+"/__local/login", nil))
	if plain.Code != 403 {
		t.Fatal("plaintext admitted")
	}
	server := httptest.NewTLSServer(h)
	defer server.Close()
	for _, test := range []struct {
		method, host, origin, site, body string
		status                           int
	}{
		{"GET", "localhost:8443", Origin, "same-origin", "", 405},
		{"POST", "evil.invalid", Origin, "same-origin", "", 403},
		{"POST", "localhost:8443", "https://evil.invalid", "cross-site", "", 403},
		{"POST", "localhost:8443", "", "", "", 403},
		{"POST", "localhost:8443", Origin, "cross-site", "", 403},
		{"POST", "localhost:8443", Origin, "same-origin", "x", 400},
	} {
		req, err := http.NewRequestWithContext(t.Context(), test.method, server.URL+"/__local/login", strings.NewReader(test.body))
		if err != nil {
			t.Fatal("request")
		}
		req.Host = test.host
		if test.origin != "" {
			req.Header.Set("Origin", test.origin)
		}
		if test.site != "" {
			req.Header.Set("Sec-Fetch-Site", test.site)
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal("call")
		}
		_ = res.Body.Close()
		if res.StatusCode != test.status {
			t.Errorf("guard %s: got %d want %d", test.method, res.StatusCode, test.status)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("guarded request reached credentials/KDF")
	}
}
