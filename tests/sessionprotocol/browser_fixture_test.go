//go:build integration && browser

package sessionprotocol_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

// TestBrowserFixture is a manually selected fixture, not a regular integration
// gate. Run only this test against the disposable stores required by setup.
func TestBrowserFixture(t *testing.T) {
	origin := browserOrigin(t)
	assets := browserAssets(t)
	f := setup(t)
	c := runtimeConfig(t, f, namespace(t), false)
	c.HTTP.Origin = origin.String()
	loginLoad := os.Getenv("OJ_BROWSER_LOGIN_LOAD") == "1"
	if loginLoad {
		c.PasswordPolicy.Current.MemoryKiB = 19 * 1024
		c.PasswordPolicy.Current.Parallelism = 1
		c.PasswordLimits.MaxMemoryKiB = 19 * 1024
	}
	bucket := protocol.LoginBucket{Count: 200, Window: time.Hour}
	c.LoginLimits = protocol.LoginLimits{BeginGlobal: bucket, BeginSource: bucket, BeginEmail: bucket, RecoveryGlobal: bucket, RecoverySource: bucket}
	secrets, err := protocol.LoadLoginSecrets(c.SecretsFile)
	must(t, err)
	must(t, protocol.InitializeLoginLimits(f.ctx, f.store(t, c.Namespace, 0, protocol.OrdinaryPolicy()), secrets, c.LoginLimits))
	rules := enableEntryPasswordChange(t, f, &c, protocol.PasswordChangeLimits{Global: bucket, Source: bucket, Subject: bucket}, true)
	subject := f.subject(t, false)
	browserDrafts(t, f, &c, subject.ID)
	reviewMode := os.Getenv("OJ_BROWSER_REVIEW_MATERIALS") == "1"
	if reviewMode {
		if !c.PrivateDrafts.ReferenceRequests {
			t.Fatal("browser review requires the synthetic reference fixture")
		}
		c.PrivateDrafts.ReviewIntake = true
		c.PrivateDrafts.ReviewAccess = true
		c.PrivateDrafts.StatementConfirmations = true
	}
	otherAuthor := f.subject(t, false)
	access, err := identitypg.NewProblemAccess(f.pools[0])
	must(t, err)
	_, err = access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: otherAuthor.ID, Enabled: true, ExpectedRevision: 0, Operator: "browser-fixture", Reason: "synthetic second author"})
	must(t, err)
	ca := testpki.New(t)
	proxyCert, _, _ := ca.Issue(t, "spiffe://onlinejudge/proxy/browser-fixture", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	c.Mode = sessionruntime.ProxyMTLS
	c.Proxy.CertificateSHA256 = [][32]byte{sha256.Sum256(proxyCert.Certificate[0])}
	connections := redisConnections(t, f)
	writeTimeout := max(5*time.Second, c.PrivateDrafts.Timeout+time.Second)
	process := entryStart(t, entryBinary(t), entryConfigWithWriteTimeout(t, c, ca, 3*time.Second, writeTimeout, rules), "127.0.0.1:0")
	started := process.waitEvent(t, "API starting")
	if started.Scope != "ordinary-session-http" {
		t.Fatal("browser fixture started the wrong API scope")
	}
	target, err := url.Parse("https://" + started.Address)
	must(t, err)
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: ca.Roots, Certificates: []tls.Certificate{proxyCert}}}
	t.Cleanup(transport.CloseIdleConnections)
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		ErrorLog:  log.New(io.Discard, "", 0),
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "fixture upstream unavailable", http.StatusServiceUnavailable)
		},
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(target)
			p.Out.Host = origin.Host
			p.Out.Header.Del("X-Real-IP")
			p.SetXForwarded()
		},
	}
	done := make(chan struct{})
	var stop sync.Once
	stats := &browserStats{api: make(map[string]uint64)}
	static := browserStatic(assets)
	edgeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.TLS == nil || r.Host != origin.Host {
			http.Error(w, "fixture origin rejected", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/__fixture/stats":
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			stats.write(w)
		case "/__fixture/redirect_sink":
			stats.sink()
			w.WriteHeader(http.StatusNoContent)
		case "/__fixture/stop":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			stop.Do(func() { close(done) })
		default:
			if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
				stats.request(r.Method, r.URL.Path)
				proxy.ServeHTTP(w, r)
				return
			}
			static.ServeHTTP(w, r)
		}
	})
	edge := browserServer(t, "0.0.0.0:9443", edgeHandler)
	edgeCert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	edge.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{edgeCert}}
	edge.StartTLS()
	plain := browserServer(t, "0.0.0.0:9444", static)
	plain.Start()
	info := map[string]string{"url": origin.String(), "email": subject.Email.Original(), "subject_id": subject.ID, "other_email": otherAuthor.Email.Original(), "other_subject_id": otherAuthor.ID}
	if loginLoad {
		hasher, err := password.New(c.PasswordPolicy, c.PasswordLimits)
		must(t, err)
		writer, err := identitypg.New(f.pools[0], hasher)
		must(t, err)
		accounts := make([][2]string, 100)
		for i := range accounts {
			email, err := identity.ParseEmail(fmt.Sprintf("load-%d@example.invalid", i+1))
			must(t, err)
			record, err := hasher.Create([]byte("synthetic protocol password"))
			must(t, err)
			account, err := writer.InitializePersonal(f.ctx, fmt.Sprintf("%032x", 1000+i), email, true, record)
			must(t, err)
			accounts[i] = [2]string{account.Email.Original(), account.ID}
		}
		raw, err := json.Marshal(accounts)
		must(t, err)
		info["login_accounts"] = string(raw)
	}
	if reviewMode {
		info["review_id"], info["review_draft_id"] = seedBrowserReview(t, f, origin, ca.Roots, subject, otherAuthor, access)
		info["review_reference_key"] = entryTerminalReferenceKey
	}
	if os.Getenv("OJ_BROWSER_COLD_TLS") == "1" {
		publicCA, err := os.ReadFile(ca.CAFile)
		must(t, err)
		info["ca_pem"] = string(publicCA)
	}
	ready, err := json.Marshal(info)
	must(t, err)
	_, err = fmt.Printf("BROWSER_READY %s\n", ready)
	must(t, err)
	timer := time.NewTimer(15 * time.Minute)
	defer timer.Stop()
	expired := false
	select {
	case <-done:
	case <-timer.C:
		expired = true
	case <-t.Context().Done():
		expired = true
	}
	for _, server := range []*httptest.Server{edge, plain} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := server.Config.Shutdown(ctx)
		cancel()
		if err != nil {
			_ = server.Config.Close()
			t.Error("browser fixture HTTP server did not drain")
		}
		server.Close()
	}
	transport.CloseIdleConnections()
	process.signal(t)
	process.wait(t, true)
	if _, ok := process.event("API stopped gracefully"); !ok {
		t.Error("browser fixture API did not confirm resource drain")
	}
	noRuntimeConnections(t, f, c.Database.ApplicationName)
	sameRedisConnections(t, f, connections)
	if expired {
		t.Fatal("browser fixture stopped without explicit completion")
	}
}

func browserDrafts(t *testing.T, f *fixture, c *sessionruntime.Config, subjectID string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "materials")
	must(t, os.Mkdir(root, 0700))
	if os.Getenv("OJ_BROWSER_LC_DRAFTS") == "1" {
		const lockPath = "/workspace/config/problem-sources/library-checker.json"
		raw, err := os.ReadFile(lockPath)
		must(t, err)
		var lock struct{ Problems []struct{ Path string } }
		must(t, json.Unmarshal(raw, &lock))
		if len(lock.Problems) != 10 {
			t.Fatal("browser fixture requires ten pinned Library Checker sources")
		}
		sources := make([]string, 0, len(lock.Problems))
		for _, p := range lock.Problems {
			sources = append(sources, "library-checker:"+filepath.Base(p.Path))
		}
		c.PrivateDrafts = &sessionruntime.PrivateDraftsConfig{MaterialRoot: root, BundleDirectory: "/lc-bundle",
			SourceLockFile: lockPath, SourceIDs: sources, Timeout: 45 * time.Second, MaxInFlight: 1}
	} else {
		browserSyntheticDraft(t, f, c, root)
	}
	access, err := identitypg.NewProblemAccess(f.pools[0])
	must(t, err)
	_, err = access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: subjectID, Enabled: true, ExpectedRevision: 0, Operator: "browser-fixture", Reason: "synthetic preview author"})
	must(t, err)
}

func browserSyntheticDraft(t *testing.T, f *fixture, c *sessionruntime.Config, root string) {
	t.Helper()
	bundle := referenceEntryBundle(t)
	statement := []byte("# Browser statement\n\nInline $x^2$ and display:\n\n$$\n\\sum_{i=1}^n i\n$$\n\n| Input | Output |\n| --- | --- |\n| 1 2 | 3 |\n\n" +
		"```math\na+b\n```\n\nInvalid $\\badcommand$ then valid $y+1$.\n\n![授权的本题示意图](figures/diagram.png)\n\n![private source image](https://invalid.example.test/blocked.png)\n\n<script>window.__unsafeStatement=true</script>\n\n[unsafe](javascript:alert%281%29) [reference](https://example.test/reference)\n")
	bundle.Files["statements/aplusb.md"] = statement
	s := bundle.Document["statement"].(map[string]any)
	s["size"], s["sha256"] = len(statement), problemfixture.Hash(statement)
	bundle.AddStatementImage(t, "figures/diagram.png", problemfixture.PNG(t, 640, 160))
	bundle.Write(t)
	lock := filepath.Join(t.TempDir(), "trusted-lock.json")
	must(t, os.WriteFile(lock, bundle.Lock, 0600))
	c.PrivateDrafts = &sessionruntime.PrivateDraftsConfig{MaterialRoot: root, BundleDirectory: bundle.Directory, SourceLockFile: lock,
		SourceIDs: []string{"library-checker:aplusb"}, Timeout: 3 * time.Second, MaxInFlight: 1, ReferenceRequests: true}
	must(t, referencepg.Configure(f.ctx, f.pools[0], problemreference.Limits{Global: 4, Owner: 1}))
}

func browserOrigin(t *testing.T) *url.URL {
	t.Helper()
	raw := os.Getenv("OJ_BROWSER_ORIGIN")
	origin, err := url.Parse(raw)
	if err != nil || origin.Scheme != "https" || origin.Hostname() != "127.0.0.1" || origin.User != nil ||
		origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || origin.Opaque != "" {
		t.Fatal("browser fixture requires an explicit loopback HTTPS origin")
	}
	port, err := strconv.ParseUint(origin.Port(), 10, 16)
	if err != nil || port == 0 || raw != "https://127.0.0.1:"+strconv.FormatUint(port, 10) {
		t.Fatal("browser fixture requires a canonical published loopback port")
	}
	return origin
}

func browserAssets(t *testing.T) string {
	t.Helper()
	path := os.Getenv("OJ_BROWSER_UI_DIR")
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		t.Fatal("browser fixture requires an explicit built assets directory")
	}
	directory, err := os.Stat(path)
	if err != nil || !directory.IsDir() {
		t.Fatal("browser fixture assets directory unavailable")
	}
	index, err := os.Lstat(filepath.Join(path, "index.html"))
	if err != nil || !index.Mode().IsRegular() || index.Size() == 0 {
		t.Fatal("browser fixture requires a regular built index.html")
	}
	return path
}

func browserStatic(directory string) http.Handler {
	files := http.FileServer(http.Dir(directory))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/__fixture/") {
			http.NotFound(w, r)
			return
		}
		// Mirror the production SPA fallback for the explicit UI routes only.
		if r.URL.Path == "/login" || r.URL.Path == "/account" || r.URL.Path == "/submissions" || r.URL.Path == "/author/drafts" || strings.HasPrefix(r.URL.Path, "/author/drafts/") || strings.HasPrefix(r.URL.Path, "/preview/problems/") ||
			r.URL.Path == "/review/materials" || strings.HasPrefix(r.URL.Path, "/review/materials/") {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			r.URL.RawPath = ""
		}
		files.ServeHTTP(w, r)
	})
}

func browserServer(t *testing.T, address string, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	t.Cleanup(server.Close)
	must(t, server.Listener.Close())
	listener, err := net.Listen("tcp", address)
	must(t, err)
	server.Listener = listener
	server.Config.ReadHeaderTimeout = 2 * time.Second
	server.Config.ReadTimeout = 5 * time.Second
	server.Config.WriteTimeout = 5 * time.Second
	server.Config.IdleTimeout = time.Minute
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	return server
}

type browserStats struct {
	mu           sync.Mutex
	api          map[string]uint64
	redirectSink uint64
}

func (s *browserStats) request(method, path string) {
	// Only fixed product paths become keys, so a malformed URL cannot put a
	// credential in diagnostics. Query strings, headers and bodies are not read.
	switch path {
	case "/api/v1/session", "/api/v1/session/activity", "/api/v1/session/logout", "/api/v1/session/login/begin", "/api/v1/session/login/complete", "/api/v1/session/login/abandon", "/api/v1/account/password":
	default:
		path = "/api/*"
	}
	if method != "GET" && method != "POST" && method != "HEAD" && method != "OPTIONS" {
		method = "OTHER"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.api[method+" "+path]++
}

func (s *browserStats) sink() { s.mu.Lock(); defer s.mu.Unlock(); s.redirectSink++ }

func (s *browserStats) write(w http.ResponseWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{"api_requests": s.api, "redirect_sink": s.redirectSink})
}
