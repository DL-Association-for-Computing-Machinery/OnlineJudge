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

func testRotationEntry(t *testing.T, f *fixture, binary string) {
	cfg := runtimeConfig(t, f, namespace(t), false)
	cfg.SessionRotation = true
	// Three successful completions and up to three rotation requests share
	// this budget. An already-revoked loser might not reach admission.
	cfg.LoginLimits.RecoveryGlobal.Count = 6
	cfg.LoginLimits.RecoverySource.Count = 6
	secrets, err := protocol.LoadLoginSecrets(cfg.SecretsFile)
	must(t, err)
	must(t, protocol.InitializeLoginLimits(f.ctx,
		f.store(t, cfg.Namespace, 0, protocol.OrdinaryPolicy()), secrets, cfg.LoginLimits))
	ca := testpki.New(t)
	a := entryStart(t, binary, entryConfig(t, cfg, ca, 3*time.Second), "127.0.0.1:0")
	cfg.Database.ApplicationName += "_b"
	b := entryStart(t, binary, entryConfig(t, cfg, ca, 3*time.Second), "127.0.0.1:0")
	x, y := entryClient(t, a, ca), entryClient(t, b, ca)
	p := f.subject(t, false)
	store := f.store(t, cfg.Namespace, 0, protocol.OrdinaryPolicy())
	ordinary := entryCredential(t, x, p, "synthetic protocol password")
	check := func(old protocol.Credential, persistent, race bool, slots int) protocol.Credential {
		t.Helper()
		before, err := store.Touch(f.ctx, old)
		must(t, err)
		index := "oj-session:" + cfg.Namespace + ":subject:" + hash(p.ID+":1:"+before.RecoveryID)
		ttl, err := f.control.Do(f.ctx, "PEXPIRETIME", index).Int64()
		must(t, err)
		loginStatus(t, loginCall(y, transport.RotateSessionPath, "", credentialCookies(old), func(r *http.Request) {
			r.Header.Set("Origin", "https://wrong.invalid")
		}), 403)
		var result loginResponse
		if race {
			out := make(chan loginResponse, 2)
			start := make(chan struct{})
			for _, api := range []*loginEndpoint{x, y} {
				go func() {
					<-start
					out <- loginCall(api, transport.RotateSessionPath, "", credentialCookies(old), nil)
				}()
			}
			close(start)
			first, second := <-out, <-out
			if first.status != 204 {
				first, second = second, first
			}
			result = first
			loginStatus(t, second, 401)
		} else {
			result = loginCall(y, transport.RotateSessionPath, "", credentialCookies(old), nil)
		}
		loginStatus(t, result, 204)
		cookie := responseCookie(t, result, transport.CookieName)
		if len(result.cookies) != 1 || len(result.body) != 0 || !cookie.HttpOnly || !cookie.Secure ||
			cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge != 0 ||
			(persistent && !cookie.Expires.Equal(before.AbsoluteDeadline.Truncate(time.Second))) || (!persistent && !cookie.Expires.IsZero()) {
			t.Fatal("unexpected cookie after rotation")
		}
		fresh, err := protocol.ParseCredential(cookie.Value)
		must(t, err)
		after, err := store.Read(f.ctx, fresh)
		must(t, err)
		if fresh == old || before != after {
			t.Fatal("rotation altered identity or deadlines")
		}
		ttlAfter, err := f.control.Do(f.ctx, "PEXPIRETIME", index).Int64()
		must(t, err)
		count, err := f.control.ZCard(f.ctx, index).Result()
		must(t, err)
		if int(count) != slots || ttl != ttlAfter {
			t.Fatal("rotation changed slot count or index expiry")
		}
		oldTTL, err := f.control.Do(f.ctx, "PEXPIRETIME", tokenKey(cfg.Namespace, old)).Int64()
		must(t, err)
		if oldTTL != before.AbsoluteDeadline.UnixMilli() {
			t.Fatal("rotation lost the original tombstone expiry")
		}
		loginStatus(t, loginCall(x, "/api/v1/session/activity", "", credentialCookies(old), nil), 401)
		loginStatus(t, loginCall(y, transport.RotateSessionPath, "", credentialCookies(old), nil), 401)
		return fresh
	}
	check(ordinary, false, false, 1)
	_ = entryCredential(t, x, p, "synthetic protocol password")
	body := strings.TrimSuffix(entryLoginBody(t, p, "synthetic protocol password"), "}") + `,"remember":true}`
	pending := loginCall(y, "/api/v1/session/login/begin", body, nil, nil)
	loginStatus(t, pending, 202)
	result := loginCall(x, "/api/v1/session/login/complete", "", pending.cookies, nil)
	loginStatus(t, result, 204)
	remembered, err := protocol.ParseCredential(responseCookie(t, result, transport.CookieName).Value)
	must(t, err)
	fresh := check(remembered, true, true, 3)
	for i := 0; i < 2; i++ {
		before := len(namespaceRecords(t, f, cfg.Namespace))
		rotate := loginCall(x, transport.RotateSessionPath, "", credentialCookies(fresh), nil)
		if rotate.status == 429 {
			loginStatus(t, rotate, 429)
			_, err := store.Read(f.ctx, fresh)
			must(t, err)
			if len(namespaceRecords(t, f, cfg.Namespace)) != before {
				t.Fatal("rotation refused by quota changed session records")
			}
			break
		}
		loginStatus(t, rotate, 204)
		fresh, err = protocol.ParseCredential(responseCookie(t, rotate, transport.CookieName).Value)
		must(t, err)
		if i == 1 {
			t.Fatal("rotation bypassed the recovery budget")
		}
	}
	for _, proc := range []*entryProcess{a, b} {
		proc.signal(t)
		proc.wait(t, true)
	}
}
