//go:build integration

package sessionprotocol_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func TestTrialEdge(t *testing.T) {
	if os.Getenv("OJ_TRIAL_EDGE_FIXTURE") != "1" {
		t.Skip("requires the isolated Nginx trial fixture")
	}
	f := setup(t)
	c := runtimeConfig(t, f, namespace(t), true)
	const origin = "https://oj.example.invalid"
	c.HTTP.Origin = origin
	c.Mode = sessionruntime.ProxyMTLS
	pair, err := tls.LoadX509KeyPair("/trial-tls/proxy-chain.pem", "/trial-tls/proxy-key.pem")
	must(t, err)
	c.Proxy.CertificateSHA256 = [][32]byte{sha256.Sum256(pair.Certificate[0])}
	cfg := entryConfig(t, c, testpki.New(t), 3*time.Second)
	data, err := os.ReadFile(cfg)
	must(t, err)
	var doc map[string]any
	must(t, json.Unmarshal(data, &doc))
	material := make(map[string]string)
	for field, name := range map[string]string{"certificate_file": "backend-chain.pem", "key_file": "backend-key.pem", "ca_file": "api-ca.pem"} {
		raw, err := os.ReadFile(filepath.Join("/trial-tls", name))
		must(t, err)
		path := filepath.Join(filepath.Dir(cfg), name)
		must(t, os.WriteFile(path, raw, 0600))
		material[field] = path
	}
	doc["tls"] = material
	data, err = json.Marshal(doc)
	must(t, err)
	must(t, os.WriteFile(cfg, data, 0600))
	p := entryStart(t, entryBinary(t), cfg, "127.0.0.1:9443")
	p.waitEvent(t, "API starting")

	roots := x509.NewCertPool()
	publicCA, err := os.ReadFile("/trial-tls/public-ca.pem")
	must(t, err)
	if !roots.AppendCertsFromPEM(publicCA) {
		t.Fatal("invalid trial edge fixture CA")
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", "127.0.0.1:8443")
		},
	}
	t.Cleanup(tr.CloseIdleConnections)
	jar, err := cookiejar.New(nil)
	must(t, err)
	client := &loginEndpoint{client: &http.Client{Transport: tr, Jar: jar, Timeout: 4 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	call := func(path, body string, get bool, change func(*http.Request)) loginResponse {
		return loginCall(client, path, body, nil, func(r *http.Request) {
			r.URL.Host, r.Host = "oj.example.invalid", "oj.example.invalid"
			r.Header.Set("Origin", origin)
			if get {
				r.Method = http.MethodGet
			}
			if change != nil {
				change(r)
			}
		})
	}
	ready := call("/api/v1/readyz", "", true, nil)
	loginStatus(t, ready, 200)
	if len(ready.cookies) != 0 {
		t.Fatal("readiness changed cookies")
	}
	page := call("/login", "", true, nil)
	if page.err != nil || page.status != 200 || page.headers.Get("Cache-Control") != "no-cache" ||
		!strings.Contains(string(page.body), `id="root"`) {
		t.Fatal("trial edge did not serve the application entry")
	}
	loginStatus(t, call("/api/v1/session", "", true, nil), 401)
	subject := f.subject(t, false)
	begin := call("/api/v1/session/login/begin", loginBodyFor(t, subject), false, func(r *http.Request) {
		r.Header.Add("X-Forwarded-For", "192.0.2.1")
		r.Header.Add("X-Forwarded-For", "192.0.2.2")
		r.Header.Set("X-Forwarded-Host", "wrong.invalid")
		r.Header.Set("X-Forwarded-Proto", "http")
		r.Header.Set("X-Real-IP", "192.0.2.3")
		r.Header.Set("Forwarded", "for=192.0.2.4")
	})
	loginStatus(t, begin, 202)
	complete := call("/api/v1/session/login/complete", "", false, nil)
	credential := completedCredential(t, complete)
	read := call("/api/v1/session", "", true, nil)
	loginStatus(t, read, 200)
	var identity struct {
		SubjectID string `json:"subject_id"`
	}
	must(t, json.Unmarshal(read.body, &identity))
	if identity.SubjectID != subject.ID {
		t.Fatal("trial edge returned the wrong account")
	}
	marker := "/trial-maintenance/paused"
	must(t, os.WriteFile(marker, nil, 0644))
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodOptions} {
		blocked := call("/api/v1/session/logout", "", false, func(r *http.Request) { r.Method = method })
		loginStatus(t, blocked, 503)
		if string(blocked.body) != `{"code":"TRIAL_MAINTENANCE"}` ||
			blocked.headers.Get("Cache-Control") != "no-store" || len(blocked.cookies) != 0 {
			t.Fatal("maintenance rejection changed cookies or returned a cacheable response")
		}
	}
	loginStatus(t, call("/api/v1/readyz", "", true, nil), 200)
	loginStatus(t, call("/api/v1/readyz", "", true, func(r *http.Request) {
		r.Method = http.MethodHead
	}), 405) // Nginx forwards HEAD; the API allows only GET here.
	loginStatus(t, call("/api/v1/session", "", true, nil), 200)
	must(t, os.Remove(marker))
	logout := call("/api/v1/session/logout", "", false, nil)
	loginStatus(t, logout, 204)
	if responseCookie(t, logout, transport.CookieName).MaxAge != -1 ||
		len(jar.Cookies(&url.URL{Scheme: "https", Host: "oj.example.invalid", Path: "/"})) != 0 {
		t.Fatal("trial edge logout did not clear the session cookie")
	}
	loginStatus(t, call("/api/v1/session", "", true, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: transport.CookieName, Value: credential.Value()})
	}), 401)
	wrongHost := call("/login", "", true, func(r *http.Request) { r.Host = "wrong.invalid" })
	if wrongHost.err != nil || wrongHost.status != 421 {
		t.Fatal("trial edge accepted a different public host")
	}
	p.signal(t)
	p.wait(t, true)
	noRuntimeConnections(t, f, c.Database.ApplicationName)
}
