//go:build integration

package sessionprotocol_test

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testManagementDeadlinesAndDisable(t *testing.T, f *fixture) {
	ns := namespace(t)
	actor, target := f.subject(t, false), f.subject(t, false)
	_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id IN ($1,$2)`, actor.ID, target.ID)
	must(t, err)
	roles, err := identitypg.NewPlatformAdmins(f.pools[0])
	must(t, err)
	_, err = roles.Change(f.ctx, identity.PlatformAdminChange{SubjectID: actor.ID, Enabled: true, Operator: "fixture", Reason: "test deadlines"})
	must(t, err)
	c := runtimeConfig(t, f, ns, true)
	c.ManagementSessions, c.ManagementRoleChanges = true, true
	a := loginHandlerServer(t, openRuntime(t, c))
	b := loginHandlerServer(t, openRuntime(t, c))
	body := fmt.Sprintf(`{"subject_id":%q,"expected_revision":0,"enabled":true,"reason":"must not commit","password":"synthetic protocol password"}`, target.ID)
	deny := func(t *testing.T, cookie *http.Cookie) {
		t.Helper()
		for _, endpoint := range []*loginEndpoint{a, b} {
			assertManagementRead(t, endpoint, managementPath, cookie, 401)
			loginStatus(t, loginCall(endpoint, managementPath+"/activity", "", []*http.Cookie{cookie}, nil), 401)
			loginStatus(t, loginCall(endpoint, transport.ManagementRolePath, body, []*http.Cookie{cookie}, nil), 401)
		}
		_, err := roles.Inspect(f.ctx, target.ID)
		wantError(t, err, identity.ErrNotFound)
	}
	for _, absolute := range []bool{false, true} {
		name := "idle expiry"
		if absolute {
			name = "absolute expiry"
		}
		t.Run(name, func(t *testing.T) {
			cookie := managementRestoreLogin(t, a, actor, 204)
			key := "oj-management:" + ns + ":token:" + hash(cookie.Value)
			current, err := f.control.Get(f.ctx, key).Result()
			must(t, err)
			now, err := f.control.Time(f.ctx).Result()
			must(t, err)
			deadline := now.Add(1500 * time.Millisecond).Truncate(time.Millisecond)
			created := deadline.Add(-30 * time.Minute)
			if absolute {
				created = deadline.Add(-8 * time.Hour)
			}
			// Shift the test record close to its deadline while keeping the
			// real policy, Redis clock, TTL and authority checks in use.
			fields := strings.Split(current, ":")
			if len(fields) != 9 {
				t.Fatal("unexpected management record shape")
			}
			fields[6] = strconv.FormatInt(created.UnixMilli(), 10)
			fields[7] = strconv.FormatInt(created.Add(8*time.Hour).UnixMilli(), 10)
			fields[8] = strconv.FormatInt(deadline.UnixMilli(), 10)
			must(t, f.control.Do(f.ctx, "SET", key, strings.Join(fields, ":"), "PXAT", deadline.UnixMilli()).Err())
			view := assertManagementRead(t, b, managementPath, cookie, 200)
			if !view.Idle.Equal(deadline) || view.Absolute.Equal(deadline) != absolute {
				t.Fatal("fixture does not represent the selected deadline")
			}
			if absolute {
				r := loginCall(b, managementPath+"/activity", "", []*http.Cookie{cookie}, nil)
				loginStatus(t, r, 200)
			}
			f.waitTime(t, deadline)
			deny(t, cookie)
			exists, err := f.control.Exists(f.ctx, key).Result()
			must(t, err)
			if exists != 0 {
				t.Fatal("expired management credential was retained or recreated")
			}
		})
	}
	t.Run("disabled administrator", func(t *testing.T) {
		cookie := managementRestoreLogin(t, a, actor, 204)
		_, ordinary := f.create(t, f.store(t, ns, 0, protocol.OrdinaryPolicy()), actor)
		assertManagementRead(t, b, managementPath, cookie, 200)
		_, err := f.identities[1].DisableCAS(f.ctx, actor.ID, actor.Revision)
		must(t, err)
		deny(t, cookie)
		for _, endpoint := range []*loginEndpoint{a, b} {
			assertManagementRead(t, endpoint, "/api/v1/session", &http.Cookie{Name: transport.CookieName, Value: ordinary.Value()}, 401)
			managementRestoreLogin(t, endpoint, actor, 401)
		}
		key := "oj-management:" + ns + ":token:" + hash(cookie.Value)
		ttl, err := f.control.PTTL(f.ctx, key).Result()
		must(t, err)
		if ttl <= 0 {
			t.Fatal("management cookie was already expired before the account fence was verified")
		}
	})
}
