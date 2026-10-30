package sessionexperiment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

// Reuse the same two TLS APIs while inspecting only the management namespace.
func adminView(f *fixture) *fixture {
	view := *f
	view.stores, view.managers = f.adminStores, f.adminManagers
	view.routePrefix = "/admin-fixture"
	return &view
}

func shortAdmin(t *testing.T) *fixture {
	t.Helper()
	return adminView(setupAuthWithProfiles(t, rememberPolicy{7 * 24 * time.Hour, 24 * time.Hour}, rememberPolicy{6 * time.Second, 2 * time.Second}))
}

func TestAdminLifetimeAndCookieDefaults(t *testing.T) {
	for _, choice := range []string{"", "?remember=false", "?remember=true"} {
		t.Run(choice, func(t *testing.T) {
			f := adminView(setupAuth(t))
			before := redisTime(t, f)
			cookie := expect(t, f, 0, "POST", "/login-fixture"+choice, nil, 201).Cookies()[0]
			after := redisTime(t, f)
			state := readLifetime(t, f, cookie)
			if state.deadline.Before(before.Add(8*time.Hour-time.Second)) || state.deadline.After(after.Add(8*time.Hour+time.Second)) ||
				state.expiry.Before(before.Add(30*time.Minute-time.Second)) || state.expiry.After(after.Add(30*time.Minute+time.Second)) {
				t.Fatal("management login inherited personal lifetime")
			}
			if cookie.Name != "admin_session" || cookie.Path != "/admin-fixture" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge != 0 || !cookie.Expires.IsZero() {
				t.Fatal("management cookie scope or persistence")
			}
			for replica := range 2 {
				expect(t, f, replica, "GET", "/me", cookie, 200)
				r := expect(t, f, replica, "POST", "/activity-fixture?remember=true", cookie, 201)
				if len(r.Cookies()) != 1 || r.Cookies()[0].MaxAge != 0 || !r.Cookies()[0].Expires.IsZero() {
					t.Fatal("management activity enabled persistence")
				}
				next := readLifetime(t, f, cookie)
				if !next.deadline.Equal(state.deadline) || next.expiry.After(redisTime(t, f).Add(30*time.Minute+time.Second)) {
					t.Fatal("management activity changed deadline or idle policy")
				}
			}
		})
	}
}

func TestAdminPollingDoesNotRenew(t *testing.T) {
	testPollingLifetime(t, shortAdmin(t), "/login-fixture?remember=true")
}

func TestAdminActivityRespectsAbsoluteLifetime(t *testing.T) {
	testActivityLifetime(t, shortAdmin(t), "/login-fixture?remember=true")
}

func TestAdminExpiryRejectsLoadedActivity(t *testing.T) {
	testScopedLoadedLifetimeExpiry(t, false, true)
}

func TestAdminAndPersonalCredentialsAreIsolated(t *testing.T) {
	f := setupAuth(t)
	admin := adminView(f)
	personal := expect(t, f, 0, "POST", "/login-fixture?remember=true", nil, 201).Cookies()[0]
	management := expect(t, admin, 1, "POST", "/login-fixture", nil, 201).Cookies()[0]
	before := memorySnapshot(t, f)
	for replica := range 2 {
		for _, methodPath := range [][2]string{{"GET", "/me"}, {"POST", "/activity-fixture"}, {"POST", "/logout"}} {
			rejectWithoutCookie(t, admin, replica, methodPath[0], methodPath[1], personal, 401)
			renamed := *personal
			renamed.Name = management.Name
			rejectWithoutCookie(t, admin, replica, methodPath[0], methodPath[1], &renamed, 401)
			rejectWithoutCookie(t, f, replica, methodPath[0], methodPath[1], management, 401)
			renamed = *management
			renamed.Name = personal.Name
			rejectWithoutCookie(t, f, replica, methodPath[0], methodPath[1], &renamed, 401)
		}
		for _, view := range []*fixture{f, admin} {
			req, err := http.NewRequest("GET", view.servers[replica].URL+view.routePrefix+"/me", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(personal)
			req.AddCookie(management)
			r, err := view.servers[replica].Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = r.Body.Close()
			if r.StatusCode != 200 || len(r.Cookies()) != 0 {
				t.Fatal("coexisting cookies selected wrong scope")
			}
		}
	}
	assertMemoryUnchanged(t, f, before)
	r := expect(t, admin, 0, "POST", "/logout", management, 204)
	if len(r.Cookies()) != 1 || r.Cookies()[0].Name != management.Name || r.Cookies()[0].Path != management.Path || r.Cookies()[0].MaxAge >= 0 {
		t.Fatal("management logout did not clear its own cookie path")
	}
	for replica := range 2 {
		rejectWithoutCookie(t, admin, replica, "GET", "/me", management, 401)
		expect(t, f, replica, "GET", "/me", personal, 200)
	}
}

func TestAdminLogoutRejectsLoadedActivity(t *testing.T) {
	f := adminView(setupAuth(t))
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	finished := make(chan error, 1)
	go func() {
		r, err := request(f, 0, "POST", "/late-save", cookie)
		if err == nil && (r.StatusCode != 401 || len(r.Cookies()) != 0) {
			err = fmt.Errorf("loaded admin save status=%d cookies=%d", r.StatusCode, len(r.Cookies()))
		}
		finished <- err
	}()
	select {
	case <-f.loaded:
	case <-time.After(time.Second):
		close(f.resume)
		t.Fatal("admin load barrier")
	}
	expect(t, f, 1, "POST", "/logout", cookie, 204)
	before := memorySnapshot(t, f)
	close(f.resume)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	assertMemoryUnchanged(t, f, before)
}

func TestAdminPayloadScopeAndLifetimeAreGuarded(t *testing.T) {
	for _, fault := range []string{"missing-scope", "wrong-scope", "wrong-scope-type", "remember", "persist", "overlong"} {
		t.Run(fault, func(t *testing.T) {
			f := adminView(setupAuth(t))
			cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			ctx := context.Background()
			data, found, err := f.stores[0].FindCtx(ctx, cookie.Value)
			if err != nil || !found {
				t.Fatal("missing admin payload")
			}
			deadline, values, err := (scs.GobCodec{}).Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "missing-scope":
				delete(values, "session_scope")
			case "wrong-scope":
				values["session_scope"] = "personal"
			case "wrong-scope-type":
				values["session_scope"] = int64(1)
			case "remember":
				values["remember_login"] = true
			case "persist":
				values["__rememberMe"] = true
			case "overlong":
				deadline = time.Now().Add(9 * time.Hour)
			}
			changed, err := (scs.GobCodec{}).Encode(deadline, values)
			if err != nil {
				t.Fatal(err)
			}
			before := memorySnapshot(t, f)
			token := cookie.Value
			if fault == "overlong" {
				// A fresh token proves the 8h cap, independently of the guard
				// against changing an existing session's absolute deadline.
				token = "overlong-admin-fixture"
			}
			if err := f.stores[1].CommitCtx(ctx, token, changed, time.Now().Add(time.Minute)); !errors.Is(err, errStale) {
				t.Fatalf("invalid admin save: %v", err)
			}
			assertMemoryUnchanged(t, f, before)
			// Simulate a misplaced payload, independently of the write guard.
			if err := f.stores[0].client.HSet(ctx, f.stores[0].key(cookie.Value), "data", changed, "deadline", deadline.UnixMilli()).Err(); err != nil {
				t.Fatal(err)
			}
			before = memorySnapshot(t, f)
			for replica := range 2 {
				rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
			}
			assertMemoryUnchanged(t, f, before)
		})
	}
}
