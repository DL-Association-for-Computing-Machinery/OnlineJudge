//go:build integration

package sessionprotocol_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func testEntryPasswordChange(t *testing.T, f *fixture, binary string) {
	t.Run("two real API replicas revoke old sessions and pending logins", func(t *testing.T) {
		c := runtimeConfig(t, f, namespace(t), true)
		rulesFile := enableEntryPasswordChange(t, f, &c, changeLimits(), true)
		ca := testpki.New(t)
		connections := redisConnections(t, f)
		a := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second, rulesFile), "127.0.0.1:0")
		x := entryClient(t, a, ca)
		other := c
		other.Database.ApplicationName += "_b"
		b := entryStart(t, binary, entryConfig(t, other, ca, 3*time.Second, rulesFile), "127.0.0.1:0")
		y := entryClient(t, b, ca)
		p := f.subject(t, true)
		first := entryCredential(t, x, p, "synthetic protocol password")
		second := entryCredential(t, y, p, "synthetic protocol password")
		pending := beginLogin(t, x, p)
		before, err := f.identities[0].State(f.ctx, p.ID)
		must(t, err)
		response := loginCall(y, "/api/v1/account/password", changeBody(t, "synthetic protocol password", "synthetic replacement alpha"), credentialCookies(first), nil)
		requireChangeClearedCookies(t, response)
		after, err := f.identities[1].State(f.ctx, p.ID)
		must(t, err)
		if after.CredentialVersion != before.CredentialVersion+1 || after.AuthGeneration != before.AuthGeneration+1 {
			t.Fatal("HTTP change did not advance both authority revisions")
		}
		for _, credential := range []protocol.Credential{first, second} {
			cookies := []*http.Cookie{{Name: "__Host-oj_session", Value: credential.Value()}}
			for _, endpoint := range []*loginEndpoint{x, y} {
				loginStatus(t, loginCall(endpoint, "/api/v1/session", "", cookies, func(r *http.Request) { r.Method = "GET" }), 401)
			}
		}
		if exists, err := f.control.Exists(f.ctx, tokenKey(c.Namespace, first), tokenKey(c.Namespace, second)).Result(); err != nil || exists != 2 {
			t.Fatal("change success depended on deleting prior Redis records")
		}
		requireChangeFailure(t, loginCall(y, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 401, "LOGIN_REJECTED")
		loginStatus(t, loginCall(x, "/api/v1/session/login/begin", loginBodyFor(t, p), nil, nil), 401)
		fresh := entryCredential(t, y, p, "synthetic replacement alpha")
		loginStatus(t, loginCall(x, "/api/v1/session", "", credentialCookies(fresh), func(r *http.Request) { r.Method = "GET" }), 200)
		for _, process := range []*entryProcess{a, b} {
			process.signal(t)
			process.wait(t, true)
		}
		noRuntimeConnections(t, f, c.Database.ApplicationName)
		noRuntimeConnections(t, f, other.Database.ApplicationName)
		sameRedisConnections(t, f, connections)
	})
	t.Run("transport and password rejection never mutate credentials", func(t *testing.T) {
		c := runtimeConfig(t, f, namespace(t), true)
		file := enableEntryPasswordChange(t, f, &c, changeLimits(), true)
		ca := testpki.New(t)
		p := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second, file), "127.0.0.1:0")
		e := entryClient(t, p, ca)
		subject := f.subject(t, false)
		credential := entryCredential(t, e, subject, "synthetic protocol password")
		before, err := f.identities[0].Get(f.ctx, subject.ID)
		must(t, err)
		body := changeBody(t, "synthetic protocol password", "synthetic replacement alpha")
		for _, tc := range []struct {
			current, replacement string
			status               int
			code                 string
		}{
			{"synthetic protocol password", "short", 400, "INVALID_NEW_PASSWORD"},
			{"synthetic protocol password", "synthetic blocked password", 400, "WEAK_PASSWORD"},
			{"synthetic protocol password", "synthetic protocol password", 400, "PASSWORD_UNCHANGED"},
			{"incorrect current password", "synthetic replacement alpha", 401, "PASSWORD_CHANGE_REJECTED"},
			{strings.Repeat("x", 129), "synthetic replacement alpha", 400, "INVALID_PASSWORD_INPUT"},
		} {
			requireChangeFailure(t, loginCall(e, "/api/v1/account/password", changeBody(t, tc.current, tc.replacement), credentialCookies(credential), nil), tc.status, tc.code)
		}
		rates := changeSnapshot(t, f, c.Namespace)
		loginStatus(t, loginCall(e, "/api/v1/account/password", body, credentialCookies(credential), func(r *http.Request) { r.Method = "GET" }), 405)
		loginStatus(t, loginCall(e, "/api/v1/account/password", body, credentialCookies(credential), func(r *http.Request) { r.Header.Set("Origin", "https://evil.invalid") }), 403)
		requireChangeFailure(t, loginCall(e, "/api/v1/account/password", body, nil, nil), 401, "PASSWORD_CHANGE_REJECTED")
		requireChangeFailure(t, loginCall(e, "/api/v1/account/password", `{"current_password":"synthetic protocol password","new_password":"synthetic replacement alpha","subject_id":"attacker-selected"}`, credentialCookies(credential), nil), 400, "INVALID_PASSWORD_CHANGE")
		unchangedChange(t, f, c.Namespace, rates)
		after, err := f.identities[1].Get(f.ctx, subject.ID)
		must(t, err)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("rejected HTTP change mutated credential material")
		}
		p.signal(t)
		p.wait(t, true)
	})
	t.Run("optional capability refuses unavailable or mismatched rules", func(t *testing.T) {
		for _, kind := range []string{"missing marker", "different rule fingerprint", "missing rule file"} {
			t.Run(kind, func(t *testing.T) {
				c := runtimeConfig(t, f, namespace(t), true)
				file := enableEntryPasswordChange(t, f, &c, changeLimits(), kind != "missing marker")
				switch kind {
				case "different rule fingerprint":
					c.PasswordChange.Acceptance = changeRules(t, 16, "synthetic blocked password")
				case "missing rule file":
					file += ".absent"
				}
				ca := testpki.New(t)
				before := changeSnapshot(t, f, c.Namespace)
				connections := redisConnections(t, f)
				p := entryStart(t, binary, entryConfig(t, c, ca, time.Second, file), "127.0.0.1:0")
				p.wait(t, false)
				if _, ok := p.event("API starting"); ok {
					t.Fatal("unavailable password rules enabled a listener")
				}
				unchangedChange(t, f, c.Namespace, before)
				noRuntimeConnections(t, f, c.Database.ApplicationName)
				sameRedisConnections(t, f, connections)
			})
		}
	})
	t.Run("pinned proxies share real source quotas for changes", func(t *testing.T) { testEntryPasswordChangeProxy(t, f, binary) })
}
