//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testManagementRoleChanges(t *testing.T, f *fixture) {
	ns := namespace(t)
	a, b := f.subject(t, false), f.subject(t, false)
	_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id IN ($1,$2)`, a.ID, b.ID)
	must(t, err)
	roles, err := identitypg.NewPlatformAdmins(f.pools[0])
	must(t, err)
	_, err = roles.Change(f.ctx, identity.PlatformAdminChange{SubjectID: a.ID, Enabled: true, Operator: "fixture", Reason: "bootstrap"})
	must(t, err)
	config := runtimeConfig(t, f, ns, true)
	config.ManagementSessions = true
	readOnly := loginHandlerServer(t, openRuntime(t, config))
	config.ManagementRoleChanges = true
	left := loginHandlerServer(t, openRuntime(t, config))
	right := loginHandlerServer(t, openRuntime(t, config))
	login := func(e *loginEndpoint, p identity.Personal) *http.Cookie {
		t.Helper()
		result := loginCall(e, managementPath+"/login", loginBodyFor(t, p), nil, nil)
		loginStatus(t, result, 204)
		if len(result.cookies) != 1 || result.cookies[0].Name != transport.ManagementCookieName {
			t.Fatal("management login missing cookie")
		}
		return result.cookies[0]
	}
	secret := "synthetic protocol password"
	ca := login(left, a)
	body := func(id string, rev int, enabled bool, pass string) string {
		return fmt.Sprintf(`{"subject_id":%q,"expected_revision":%d,"enabled":%t,"reason":"audited test","password":%q}`, id, rev, enabled, pass)
	}
	call := func(e *loginEndpoint, c *http.Cookie, input string, status int) loginResponse {
		t.Helper()
		r := loginCall(e, transport.ManagementRolePath, input, []*http.Cookie{c}, nil)
		loginStatus(t, r, status)
		if len(r.cookies) != 0 || strings.Contains(string(r.body), "audited test") {
			t.Fatal("sensitive action leaked data or issued a cookie")
		}
		return r
	}
	call(readOnly, ca, body(b.ID, 0, true, secret), 404)
	_, ordinary := f.create(t, f.store(t, ns, 0, protocol.OrdinaryPolicy()), a)
	call(right, &http.Cookie{Name: transport.CookieName, Value: ordinary.Value()}, body(b.ID, 0, true, secret), 401)
	call(right, ca, body(b.ID, 0, true, "wrong password"), 401)
	call(right, ca, strings.TrimSuffix(body(b.ID, 0, true, secret), "}")+`,"operator":"forged"}`, 400)
	call(right, ca, body(a.ID, 1, false, secret), 400)
	csrf := loginCall(right, transport.ManagementRolePath, body(b.ID, 0, true, secret), []*http.Cookie{ca}, func(r *http.Request) { r.Header.Del("Origin") })
	loginStatus(t, csrf, 403)
	_, err = roles.Inspect(f.ctx, b.ID)
	wantError(t, err, identity.ErrNotFound)
	before := assertManagementRead(t, left, managementPath, ca, 200)
	result := call(right, ca, body(b.ID, 0, true, secret), 200)
	var grant identity.PlatformAdminRole
	must(t, json.Unmarshal(result.body, &grant))
	if grant.SubjectID != b.ID || grant.Revision != 1 || !grant.Enabled || before != assertManagementRead(t, right, managementPath, ca, 200) {
		t.Fatal("role write returned a wrong receipt or touched the management session")
	}
	call(right, ca, body(b.ID, 0, true, secret), 409)
	cb := login(right, b)
	out := make(chan loginResponse, 2)
	start := make(chan struct{})
	go func() {
		<-start
		out <- loginCall(left, transport.ManagementRolePath, body(b.ID, 1, false, secret), []*http.Cookie{ca}, nil)
	}()
	go func() {
		<-start
		out <- loginCall(right, transport.ManagementRolePath, body(a.ID, 1, false, secret), []*http.Cookie{cb}, nil)
	}()
	close(start)
	first, second := <-out, <-out
	if first.status != 200 {
		first, second = second, first
	}
	loginStatus(t, first, 200)
	loginStatus(t, second, 401)
	var count int
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT count(*) FROM identity_platform_admin_history WHERE
		(subject_id=$1 AND maintenance_operator=$2 OR subject_id=$3 AND maintenance_operator=$4) AND reason=$5`,
		b.ID, identity.PlatformAdminOperatorPrefix+a.ID, a.ID, identity.PlatformAdminOperatorPrefix+b.ID, "audited test").Scan(&count))
	if count != 2 {
		t.Fatal("authorization failures or concurrent loser left audit writes")
	}
}
