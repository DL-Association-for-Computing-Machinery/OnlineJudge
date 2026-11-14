//go:build integration

package sessionprotocol_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const managementPath = "/api/v1/platform-admin/session"

type managementDeadlines struct {
	SubjectID string    `json:"subject_id"`
	Idle      time.Time `json:"idle_expires_at"`
	Absolute  time.Time `json:"absolute_deadline"`
}

func testManagement(t *testing.T, f *fixture) {
	ns := namespace(t)
	p := f.subject(t, false)
	_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1`, p.ID)
	must(t, err)
	roles, err := identitypg.NewPlatformAdmins(f.pools[0])
	must(t, err)
	config := runtimeConfig(t, f, ns, true)
	disabled := loginHandlerServer(t, openRuntime(t, config))
	assertManagementRead(t, disabled, managementPath, nil, 404)
	config.ManagementSessions = true
	a := loginHandlerServer(t, openRuntime(t, config))
	b := loginHandlerServer(t, openRuntime(t, config))
	login := func(body string, status int) *http.Cookie {
		t.Helper()
		result := loginCall(a, managementPath+"/login", body, nil, nil)
		loginStatus(t, result, status)
		if status != 204 {
			return nil
		}
		if len(result.cookies) != 1 || result.cookies[0].Name != transport.ManagementCookieName {
			t.Fatal("management login did not issue only the management cookie")
		}
		return result.cookies[0]
	}
	body := loginBodyFor(t, p)
	login(body, 401)
	change := identity.PlatformAdminChange{SubjectID: p.ID, Enabled: true, Operator: "fixture", Reason: "test scope"}
	_, err = roles.Change(f.ctx, change)
	must(t, err)
	login(strings.TrimSuffix(body, "}")+`,"remember":false}`, 400)
	ordinary := f.store(t, ns, 0, protocol.OrdinaryPolicy())
	_, ordinaryToken := f.create(t, ordinary, p)
	assertManagementRead(t, b, managementPath, &http.Cookie{Name: transport.CookieName, Value: ordinaryToken.Value()}, 401)
	assertManagementRead(t, b, managementPath, &http.Cookie{Name: transport.ManagementCookieName, Value: ordinaryToken.Value()}, 401)
	credential := login(body, 204)
	if !credential.Secure || !credential.HttpOnly || credential.Domain != "" || credential.Path != "/" ||
		credential.MaxAge != 0 || !credential.Expires.IsZero() || credential.SameSite != http.SameSiteStrictMode {
		t.Fatal("management issued the wrong cookie policy")
	}
	loginStatus(t, loginCall(b, managementPath+"/login", body, []*http.Cookie{credential}, nil), 409)
	assertManagementRead(t, b, "/api/v1/session", credential, 401)
	assertManagementRead(t, b, "/api/v1/session", &http.Cookie{Name: transport.CookieName, Value: credential.Value}, 401)
	initial := assertManagementRead(t, b, managementPath, credential, 200)
	if initial.SubjectID != p.ID || initial.Absolute.Sub(initial.Idle) != 7*time.Hour+30*time.Minute {
		t.Fatal("initial management deadlines or identity incorrect")
	}
	key := "oj-management:" + ns + ":token:" + hash(credential.Value)
	before, err := f.control.Get(f.ctx, key).Result()
	must(t, err)
	for range 3 {
		if next := assertManagementRead(t, b, managementPath, credential, 200); next != initial {
			t.Fatal("management polling changed deadlines")
		}
	}
	denied := loginCall(b, managementPath+"/activity", "", []*http.Cookie{credential}, func(r *http.Request) { r.Header.Del("Origin") })
	loginStatus(t, denied, 403)
	after, err := f.control.Get(f.ctx, key).Result()
	must(t, err)
	if before != after || strings.Contains(after, credential.Value) {
		t.Fatal("readonly calls changed the record or exposed the token")
	}
	// Simulate a session late in its absolute lifetime using Redis time. The
	// active window now ends before the cap, which the next touch must retain.
	now, err := f.control.Time(f.ctx).Result()
	must(t, err)
	fields := strings.Split(before, ":")
	capTime := now.Add(time.Minute).UnixMilli()
	fields[6] = strconv.FormatInt(capTime-(8*time.Hour).Milliseconds(), 10)
	fields[7] = strconv.FormatInt(capTime, 10)
	fields[8] = strconv.FormatInt(now.Add(30*time.Second).UnixMilli(), 10)
	must(t, f.control.Do(f.ctx, "SET", key, strings.Join(fields, ":"), "PXAT", now.Add(30*time.Second).UnixMilli()).Err())
	touch := loginCall(b, managementPath+"/activity", "", []*http.Cookie{credential}, nil)
	loginStatus(t, touch, 200)
	var touched managementDeadlines
	must(t, json.Unmarshal(touch.body, &touched))
	if touched.Idle.UnixMilli() != capTime || touched.Absolute.UnixMilli() != capTime || len(touch.cookies) != 0 {
		t.Fatal("management touch extended the absolute deadline or changed the cookie")
	}
	result := loginCall(b, managementPath+"/logout", "", []*http.Cookie{credential}, nil)
	loginStatus(t, result, 204)
	if len(result.cookies) != 1 || result.cookies[0].Name != transport.ManagementCookieName || result.cookies[0].MaxAge != -1 {
		t.Fatal("management logout did not clear only the management cookie")
	}
	assertManagementRead(t, a, managementPath, credential, 401)
	result = loginCall(a, managementPath+"/login", body, []*http.Cookie{credential}, nil)
	loginStatus(t, result, 204)
	if len(result.cookies) != 1 || result.cookies[0].Name != transport.ManagementCookieName || result.cookies[0].Value == credential.Value {
		t.Fatal("logged out browser failed to obtain a fresh management credential")
	}
	credential = result.cookies[0]
	change.ExpectedRevision, change.Enabled = 1, false
	_, err = roles.Change(f.ctx, change)
	must(t, err)
	assertManagementRead(t, b, managementPath, credential, 401)
	change.ExpectedRevision, change.Enabled = 2, true
	_, err = roles.Change(f.ctx, change)
	must(t, err)
	assertManagementRead(t, a, managementPath, credential, 401)
	assertManagementRead(t, b, "/api/v1/session", &http.Cookie{Name: transport.CookieName, Value: ordinaryToken.Value()}, 200)
	credential = login(body, 204)
	f.open(t)
	assertManagementRead(t, b, managementPath, credential, 401)
}

func testManagementLostIssueAndLateTouch(t *testing.T, f *fixture) {
	ns := namespace(t)
	p := f.subject(t, false)
	_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_personal_emails SET email_verified=true WHERE subject_id=$1`, p.ID)
	must(t, err)
	roles, err := identitypg.NewPlatformAdmins(f.pools[0])
	must(t, err)
	_, err = roles.Change(f.ctx, identity.PlatformAdminChange{SubjectID: p.ID, Enabled: true, Operator: "fixture", Reason: "race"})
	must(t, err)
	var orphan string
	store := f.faultStore(t, ns, &hook{op: "management-issue", after: func(_ context.Context, cmd redis.Cmder) error {
		orphan, _ = cmd.Args()[3].(string)
		return errors.New("synthetic management issue response loss")
	}}, time.Second)
	fault, err := protocol.NewManagement(store, roles)
	must(t, err)
	secret := []byte("synthetic protocol password")
	credential, err := fault.Login(f.ctx, p.Email, secret)
	wantError(t, err, protocol.ErrStore)
	if credential.Value() != "" || orphan == "" {
		t.Fatal("unknown issuance published a credential or did not reach Redis")
	}
	ttl, err := f.control.PTTL(f.ctx, orphan).Result()
	must(t, err)
	if ttl <= 0 || ttl > 30*time.Minute {
		t.Fatal("undisclosed record has no finite idle expiry")
	}
	peer, err := protocol.NewManagement(f.store(t, ns, 1, protocol.OrdinaryPolicy()), roles)
	must(t, err)
	credential, err = peer.Login(f.ctx, p.Email, secret)
	must(t, err)
	stop := make(chan struct{})
	entered := make(chan struct{})
	store = f.faultStore(t, ns, &hook{op: "management-touch", before: func(ctx context.Context, _ redis.Cmder) error {
		close(entered)
		select {
		case <-stop:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}, time.Second)
	fault, err = protocol.NewManagement(store, roles)
	must(t, err)
	ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := fault.Touch(ctx, credential); result <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("management touch did not reach the barrier")
	}
	err = peer.Revoke(ctx, credential)
	close(stop)
	must(t, err)
	wantError(t, <-result, protocol.ErrStale)
	_, err = peer.Read(ctx, credential)
	wantError(t, err, protocol.ErrStale)
}

func assertManagementRead(t *testing.T, endpoint *loginEndpoint, path string, cookie *http.Cookie, status int) managementDeadlines {
	t.Helper()
	r, err := http.NewRequest(http.MethodGet, loginOrigin+path, nil)
	must(t, err)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	res, err := endpoint.client.Do(r)
	must(t, err)
	defer func() { must(t, res.Body.Close()) }()
	body, err := io.ReadAll(res.Body)
	must(t, err)
	loginStatus(t, loginResponse{res.StatusCode, res.Header, res.Cookies(), body, nil}, status)
	if len(res.Cookies()) != 0 {
		t.Fatal("readonly response issued cookies")
	}
	var result managementDeadlines
	if status == 200 {
		must(t, json.Unmarshal(body, &result))
		if strings.Contains(string(body), "role_revision") || strings.Contains(string(body), "auth_generation") || strings.Contains(string(body), "recovery") {
			t.Fatal("management read exposed internal authorization state")
		}
	}
	return result
}
