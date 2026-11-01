//go:build integration

package sessionprotocol_test

import (
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testEntryPasswordChangeProxy(t *testing.T, f *fixture, binary string) {
	c := runtimeConfig(t, f, namespace(t), true)
	limits := changeLimits()
	limits.Source.Count = 1
	file := enableEntryPasswordChange(t, f, &c, limits, true)
	ca := testpki.New(t)
	certificate, _, _ := ca.Issue(t, "spiffe://onlinejudge/proxy/password-change", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	c.Mode = sessionruntime.ProxyMTLS
	c.Proxy.CertificateSHA256 = [][32]byte{sha256.Sum256(certificate.Certificate[0])}
	connections := redisConnections(t, f)
	a := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second, file), "127.0.0.1:0")
	other := c
	other.Database.ApplicationName += "_b"
	b := entryStart(t, binary, entryConfig(t, other, ca, 3*time.Second, file), "127.0.0.1:0")
	left, right := entryProxy(t, a, ca, certificate), entryProxy(t, b, ca, certificate)
	x, y := proxyClient(t, left, "127.0.0.2", nil), proxyClient(t, right, "127.0.0.2", nil)
	z := proxyClient(t, right, "127.0.0.3", nil)
	w := proxyClient(t, right, "127.0.0.4", nil)
	subject := f.subject(t, false)
	credential := entryCredential(t, x, subject, "synthetic protocol password")
	cookies := credentialCookies(credential)
	requireChangeFailure(t, loginCall(x, "/api/v1/account/password", changeBody(t, "incorrect current password", "synthetic replacement alpha"), cookies, nil), 401, "PASSWORD_CHANGE_REJECTED")
	before := changeSnapshot(t, f, c.Namespace)
	spoof := func(r *http.Request) {
		r.Header.Set("Forwarded", "for=198.51.100.10;proto=http;host=evil.invalid")
		r.Header.Set("X-Forwarded-For", "198.51.100.10, 198.51.100.11")
		r.Header.Set("X-Forwarded-Host", "evil.invalid")
		r.Header.Set("X-Forwarded-Proto", "http")
		r.Header.Set("X-Real-IP", "198.51.100.10")
		r.Header.Set("Connection", "X-Forwarded-For, X-Forwarded-Host, X-Forwarded-Proto")
	}
	requireChangeFailure(t, loginCall(y, "/api/v1/account/password", changeBody(t, "synthetic protocol password", "synthetic replacement alpha"), cookies, spoof), 429, "PASSWORD_CHANGE_RATE_LIMITED")
	unchangedChange(t, f, c.Namespace, before)
	requireChangeFailure(t, loginCall(z, "/api/v1/account/password", changeBody(t, "incorrect current password", "synthetic replacement alpha"), cookies, spoof), 401, "PASSWORD_CHANGE_REJECTED")
	state, err := f.identities[0].State(f.ctx, subject.ID)
	must(t, err)
	if state.Revision != subject.Revision {
		t.Fatal("denied proxy change reached credential mutation")
	}
	requireChangeClearedCookies(t, loginCall(w, "/api/v1/account/password", changeBody(t, "synthetic protocol password", "synthetic replacement alpha"), cookies, spoof))
	loginStatus(t, loginCall(x, "/api/v1/session", "", cookies, func(r *http.Request) { r.Method = "GET" }), 401)
	for _, p := range []*entryProcess{a, b} {
		p.signal(t)
		p.wait(t, true)
	}
	noRuntimeConnections(t, f, c.Database.ApplicationName)
	noRuntimeConnections(t, f, other.Database.ApplicationName)
	sameRedisConnections(t, f, connections)
}

func testPasswordChangeOwner(t *testing.T, f *fixture) {
	ns := namespace(t)
	secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
	store := f.store(t, ns, 0, protocol.OrdinaryPolicy())
	a := limitedManager(t, f, store, secrets, rateLimits(), true)
	b := limitedManager(t, f, store, secrets, rateLimits(), false)
	rules := changeRules(t, 15, "synthetic blocked password")
	must(t, protocol.InitializePasswordChangeLimits(f.ctx, store, secrets, changeLimits(), rules))
	capability, err := protocol.NewPasswordChange(f.ctx, a, secrets, changeLimits(), rules)
	must(t, err)
	wrongBudget, err := password.NewAcceptance(15, 256, [][32]byte{sha256.Sum256([]byte("synthetic blocked password"))})
	must(t, err)
	for _, invalid := range []*password.Acceptance{nil, new(password.Acceptance), changeRules(t, 14, "synthetic blocked password"), wrongBudget} {
		if c, err := protocol.NewPasswordChange(f.ctx, a, secrets, changeLimits(), invalid); c != nil || !errors.Is(err, protocol.ErrInvalid) {
			t.Fatal("invalid network password policy enabled capability")
		}
	}
	if capability.ForLogin(b) || !capability.ForLogin(a) {
		t.Fatal("password capability lost Login association")
	}
	config := transport.Config{Origin: loginOrigin, Timeout: time.Second}
	proxy := transport.ProxyConfig{CertificateSHA256: [][32]byte{{1}}}
	for _, invalid := range []*protocol.PasswordChange{nil, {}} {
		if h, err := transport.NewLimitedLogin(a, config, invalid); h != nil || !errors.Is(err, protocol.ErrInvalid) {
			t.Fatal("valid login accepted nil or zero change capability")
		}
		if h, err := transport.NewProxiedLimitedLogin(a, config, proxy, invalid); h != nil || !errors.Is(err, protocol.ErrInvalid) {
			t.Fatal("valid proxy login accepted nil or zero change capability")
		}
	}
	if h, err := transport.NewLimitedLogin(a, config, capability, capability); h != nil || !errors.Is(err, protocol.ErrInvalid) {
		t.Fatal("multiple change capabilities accepted")
	}
	if _, err := transport.NewLimitedLogin(b, transport.Config{Origin: loginOrigin, Timeout: time.Second}, capability); !errors.Is(err, protocol.ErrInvalid) {
		t.Fatal("transport accepted another Login's capability")
	}
	if _, err := transport.NewLimitedLogin(a, transport.Config{Origin: loginOrigin, Timeout: time.Second}, capability); err != nil {
		t.Fatal("transport refused correctly bound capability")
	}
}
