package sessionexperiment

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

func TestSessionPayloadCannotCrossScopes(t *testing.T) {
	f := setupAuth(t)
	admin := adminView(f)
	personal := expect(t, f, 0, "POST", "/login-fixture?remember=true", nil, 201).Cookies()[0]
	management := expect(t, admin, 1, "POST", "/login-fixture", nil, 201).Cookies()[0]
	ctx := context.Background()
	personalData, found, err := f.stores[0].FindCtx(ctx, personal.Value)
	if err != nil || !found {
		t.Fatal("missing personal payload")
	}
	adminData, found, err := admin.stores[0].FindCtx(ctx, management.Value)
	if err != nil || !found {
		t.Fatal("missing management payload")
	}
	for _, swapped := range []struct {
		view   *fixture
		cookie *http.Cookie
		data   []byte
	}{
		{f, personal, adminData}, {admin, management, personalData},
	} {
		store := swapped.view.stores[0]
		if err := store.client.HSet(ctx, store.key(swapped.cookie.Value), "data", swapped.data).Err(); err != nil {
			t.Fatal(err)
		}
		before := memorySnapshot(t, f)
		for replica := range 2 {
			rejectWithoutCookie(t, swapped.view, replica, "GET", "/me", swapped.cookie, 401)
			rejectWithoutCookie(t, swapped.view, replica, "POST", "/activity-fixture", swapped.cookie, 401)
		}
		assertMemoryUnchanged(t, f, before)
	}
}

func TestSharedGenerationsRevokePersonalAndAdmin(t *testing.T) {
	for _, change := range []string{"account", "recovery"} {
		t.Run(change, func(t *testing.T) {
			f := setupAuth(t)
			admin := adminView(f)
			cookies := []*http.Cookie{
				expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0],
				expect(t, f, 1, "POST", "/login-fixture?remember=true", nil, 201).Cookies()[0],
				expect(t, admin, 0, "POST", "/login-fixture", nil, 201).Cookies()[0],
			}
			if change == "account" {
				invalidate(t, f.stores[1].auth, true)
			} else {
				generation := beginRecovery(t, f.stores[1].auth)
				assertRecoveryClosed(t, f, cookies[0])
				assertRecoveryClosed(t, admin, cookies[2])
				finishRecovery(t, f.stores[0].auth, generation)
			}
			before := memorySnapshot(t, f)
			for i, cookie := range cookies {
				view := f
				if i == 2 {
					view = admin
				}
				for replica := range 2 {
					rejectWithoutCookie(t, view, replica, "GET", "/me", cookie, 401)
					rejectWithoutCookie(t, view, replica, "POST", "/activity-fixture", cookie, 401)
				}
			}
			assertMemoryUnchanged(t, f, before)
			assertThreeFreshSlots(t, f)
			assertThreeFreshSlots(t, admin)
		})
	}
}

func TestConcurrentPersonalAndAdminPoliciesStayIsolated(t *testing.T) {
	f := setupAuth(t)
	admin := adminView(f)
	cookies := []*http.Cookie{
		expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0],
		expect(t, f, 1, "POST", "/login-fixture?remember=true", nil, 201).Cookies()[0],
		expect(t, admin, 0, "POST", "/login-fixture", nil, 201).Cookies()[0],
	}
	deadlines := make([]time.Time, 3)
	for i, cookie := range cookies {
		view := f
		if i == 2 {
			view = admin
		}
		data, found, err := view.stores[0].FindCtx(context.Background(), cookie.Value)
		if err != nil || !found {
			t.Fatal("missing session")
		}
		deadlines[i], _, err = (scs.GobCodec{}).Decode(data)
		if err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	finished := make(chan error, 24)
	for i := range 24 {
		go func() {
			<-start
			index := i % 3
			view := f
			if index == 2 {
				view = admin
			}
			r, err := request(view, (i/3)%2, "POST", "/activity-fixture?remember=true", cookies[index])
			if err == nil && (r.StatusCode != 201 || len(r.Cookies()) != 1) {
				err = fmt.Errorf("mixed activity status=%d cookies=%d", r.StatusCode, len(r.Cookies()))
			}
			if err == nil {
				cookie := r.Cookies()[0]
				if cookie.Name != cookies[index].Name || cookie.Path != cookies[index].Path || (cookie.MaxAge > 0) != (index == 1) {
					err = fmt.Errorf("mixed activity changed scope/persistence for profile %d", index)
				}
			}
			finished <- err
		}()
	}
	close(start)
	for range 24 {
		if err := <-finished; err != nil {
			t.Error(err)
		}
	}
	for i, cookie := range cookies {
		view := f
		if i == 2 {
			view = admin
		}
		data, found, err := view.stores[0].FindCtx(context.Background(), cookie.Value)
		if err != nil || !found {
			t.Fatal("lost mixed session")
		}
		deadline, _, err := (scs.GobCodec{}).Decode(data)
		if err != nil || !deadline.Equal(deadlines[i]) {
			t.Fatal("mixed activity reset absolute deadline")
		}
	}
}

func TestAdminRedisOOMIsExplicitAndLeavesStateUnchanged(t *testing.T) {
	f := adminView(setupAuth(t))
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	before := memorySnapshot(t, f)
	restore := limitRedisMemory(t, f)
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 200)
		rejectWithoutCookie(t, f, replica, "POST", "/login-fixture?remember=true", nil, 503)
		rejectWithoutCookie(t, f, replica, "POST", "/activity-fixture", cookie, 503)
	}
	assertMemoryUnchanged(t, f, before)
	restore()
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 200)
	}
}
