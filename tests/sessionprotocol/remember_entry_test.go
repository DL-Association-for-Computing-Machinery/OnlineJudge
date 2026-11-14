//go:build integration

package sessionprotocol_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testRememberedEntry(t *testing.T, f *fixture, binary string) {
	c := runtimeConfig(t, f, namespace(t), true)
	ca := testpki.New(t)
	a := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	c.Database.ApplicationName += "_b"
	b := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	x, y := entryClient(t, a, ca), entryClient(t, b, ca)
	p := f.subject(t, false)
	const secret = "synthetic protocol password"
	const begin, complete = "/api/v1/session/login/begin", "/api/v1/session/login/complete"
	ordinaryBody := entryLoginBody(t, p, secret)
	rememberBody := strings.TrimSuffix(ordinaryBody, "}") + `,"remember":true}`
	for _, invalid := range []string{`,"remember":null}`, `,"remember":"true"}`, `,"remember":true,"re\u006dember":false}`} {
		loginStatus(t, loginCall(x, begin, strings.TrimSuffix(ordinaryBody, "}")+invalid, nil, nil), 400)
	}
	ordinary := entryCredential(t, x, p, secret)
	pending := loginCall(x, begin, rememberBody, nil, nil)
	loginStatus(t, pending, 202)
	var remembered protocol.Credential
	var expires time.Time
	for _, api := range []*loginEndpoint{y, x} {
		res := loginCall(api, complete, "", pending.cookies, nil)
		loginStatus(t, res, 204)
		cookie := responseCookie(t, res, transport.CookieName)
		credential, err := protocol.ParseCredential(cookie.Value)
		must(t, err)
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" ||
			cookie.Domain != "" || cookie.MaxAge != 0 || cookie.Expires.IsZero() {
			t.Fatal("unexpected remembered cookie attributes")
		}
		if remembered != (protocol.Credential{}) && (credential != remembered || !cookie.Expires.Equal(expires)) {
			t.Fatal("completion retry changed remembered cookie or its absolute deadline")
		}
		remembered, expires = credential, cookie.Expires
	}
	s := f.store(t, c.Namespace, 1, protocol.OrdinaryPolicy())
	o, err := s.Read(f.ctx, ordinary)
	must(t, err)
	r, err := s.Read(f.ctx, remembered)
	must(t, err)
	if r.IdleExpiresAt.Sub(r.CreatedAt) != 24*time.Hour || r.AbsoluteDeadline.Sub(r.CreatedAt) != 7*24*time.Hour ||
		!expires.Equal(r.AbsoluteDeadline.Truncate(time.Second)) {
		t.Fatal("remembered profile or cookie expiry does not match frozen policy")
	}
	for _, pair := range []struct {
		credential protocol.Credential
		initial    protocol.View
		idle       time.Duration
	}{{ordinary, o, 2 * time.Hour}, {remembered, r, 24 * time.Hour}} {
		f.waitTime(t, pair.initial.CreatedAt.Add(10*time.Millisecond))
		before, err := f.control.Time(f.ctx).Result()
		must(t, err)
		res := loginCall(y, "/api/v1/session/activity", `{"remember":true}`, credentialCookies(pair.credential), nil)
		loginStatus(t, res, 200)
		v, err := s.Read(f.ctx, pair.credential)
		must(t, err)
		after, err := f.control.Time(f.ctx).Result()
		must(t, err)
		if len(res.cookies) != 0 || !v.IdleExpiresAt.After(pair.initial.IdleExpiresAt) ||
			v.IdleExpiresAt.Before(before.Truncate(time.Millisecond).Add(pair.idle)) ||
			v.IdleExpiresAt.After(after.Add(pair.idle)) || v.AbsoluteDeadline != pair.initial.AbsoluteDeadline {
			t.Fatal("touch did not use the original profile and absolute deadline")
		}
	}
	_ = entryCredential(t, y, p, secret)
	fourth := loginCall(x, begin, rememberBody, nil, nil)
	loginStatus(t, fourth, 202)
	loginStatus(t, loginCall(y, complete, "", fourth.cookies, nil), 409)
	loginStatus(t, loginCall(x, "/api/v1/session/logout", "", credentialCookies(remembered), nil), 204)
	_, err = s.Read(f.ctx, remembered)
	wantError(t, err, protocol.ErrStale)
	loginStatus(t, loginCall(y, complete, "", fourth.cookies, nil), 204)
	for _, proc := range []*entryProcess{a, b} {
		proc.signal(t)
		proc.wait(t, true)
	}
}
