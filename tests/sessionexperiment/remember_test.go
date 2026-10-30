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

func TestRememberRequiresExplicitLoginChoice(t *testing.T) {
	for _, choice := range []string{"", "?remember=false", "?remember=true"} {
		t.Run(choice, func(t *testing.T) {
			f := setupAuth(t)
			remembered := choice == "?remember=true"
			idle, lifetime := 2*time.Hour, 12*time.Hour
			if remembered {
				idle, lifetime = 24*time.Hour, 7*24*time.Hour
			}
			before := redisTime(t, f)
			cookie := expect(t, f, 0, "POST", "/login-fixture"+choice, nil, 201).Cookies()[0]
			after := redisTime(t, f)
			state := readLifetime(t, f, cookie)
			if state.deadline.Before(before.Add(lifetime-time.Second)) || state.deadline.After(after.Add(lifetime+time.Second)) ||
				state.expiry.Before(before.Add(idle-time.Second)) || state.expiry.After(after.Add(idle+time.Second)) {
				t.Fatal("login did not use chosen idle and absolute lifetimes")
			}
			if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode ||
				(cookie.MaxAge > 0) != remembered || (!cookie.Expires.IsZero()) != remembered {
				t.Fatal("login cookie did not reflect explicit persistence choice")
			}
			if remembered && (cookie.Expires.Before(state.expiry) || cookie.Expires.After(state.expiry.Add(time.Second))) {
				t.Fatal("persistent cookie does not match server idle expiry")
			}
			// A query on activity cannot change the stored login policy. Neither
			// can a repeated login reconfigure an already-issued credential.
			for replica := range 2 {
				snapshot := memorySnapshot(t, f)
				rejectWithoutCookie(t, f, replica, "POST", "/login-fixture?remember=true", cookie, 409)
				assertMemoryUnchanged(t, f, snapshot)
				expect(t, f, replica, "GET", "/me", cookie, 200)
				activityChoice := "true"
				if remembered {
					activityChoice = "false"
				}
				r := expect(t, f, replica, "POST", "/activity-fixture?remember="+activityChoice, cookie, 201)
				if len(r.Cookies()) != 1 || (r.Cookies()[0].MaxAge > 0) != remembered {
					t.Fatal("activity query changed persistence choice")
				}
				next := readLifetime(t, f, cookie)
				if !next.deadline.Equal(state.deadline) || next.expiry.Before(state.expiry) || next.expiry.After(redisTime(t, f).Add(idle+time.Second)) {
					t.Fatal("activity changed absolute deadline or selected wrong idle policy")
				}
			}
		})
	}
}

func TestRememberPollingDoesNotRenew(t *testing.T) {
	f := setupAuthWithRemember(t, rememberPolicy{6 * time.Second, 2 * time.Second})
	testPollingLifetime(t, f, "/login-fixture?remember=true")
}

func TestRememberActivityRespectsAbsoluteLifetime(t *testing.T) {
	f := setupAuthWithRemember(t, rememberPolicy{6 * time.Second, 2 * time.Second})
	testActivityLifetime(t, f, "/login-fixture?remember=true")
}

func TestRememberExpiryRejectsLoadedActivity(t *testing.T) {
	testLoadedLifetimeExpiry(t, true)
}

func TestRememberAdmissionLostAcknowledgement(t *testing.T) {
	testAdmissionLostAcknowledgement(t, "/login-fixture?remember=true")
}

func TestRememberAbandonBeforeDelayedSave(t *testing.T) {
	testAdmissionAbandonBeforeDelayedSave(t, "/login-fixture?remember=true")
}

func TestRememberStoreLifetimeBounds(t *testing.T) {
	f := setup(t)
	for _, remembered := range []bool{false, true} {
		lifetime := 12 * time.Hour
		if remembered {
			lifetime = 7 * 24 * time.Hour
		}
		data, err := (scs.GobCodec{}).Encode(time.Now().Add(lifetime+time.Hour), map[string]interface{}{
			"subject": "fixture-user", "remember_login": remembered,
		})
		if err != nil {
			t.Fatal(err)
		}
		before := memorySnapshot(t, f)
		if err := f.stores[0].CommitCtx(context.Background(), "overlong-fixture", data, time.Now().Add(time.Hour)); !errors.Is(err, errStale) {
			t.Fatalf("overlong session accepted: %v", err)
		}
		assertMemoryUnchanged(t, f, before)
	}
}

func TestMixedRememberSessionsShareThreeSlots(t *testing.T) {
	f := setupAuth(t)
	ordinary := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	remembered := expect(t, f, 1, "POST", "/login-fixture?remember=true", nil, 201).Cookies()[0]
	start := make(chan struct{})
	results := make(chan error, 24)
	for i := range 24 {
		go func() {
			<-start
			path := "/login-fixture"
			if i%2 == 0 {
				path += "?remember=true"
			}
			r, err := request(f, (i/2)%2, "POST", path, nil)
			if err != nil {
				results <- err
				return
			}
			if r.StatusCode == 201 && len(r.Cookies()) == 1 {
				if (r.Cookies()[0].MaxAge > 0) != (i%2 == 0) {
					results <- errors.New("concurrent login leaked another request's cookie policy")
					return
				}
				data, found, err := f.stores[0].FindCtx(context.Background(), r.Cookies()[0].Value)
				if err != nil || !found {
					results <- fmt.Errorf("concurrent login record found=%v err=%v", found, err)
					return
				}
				deadline, _, err := (scs.GobCodec{}).Decode(data)
				lifetime := 12 * time.Hour
				if i%2 == 0 {
					lifetime = 7 * 24 * time.Hour
				}
				if err != nil || time.Until(deadline) < lifetime-time.Minute || time.Until(deadline) > lifetime+time.Second {
					results <- errors.New("concurrent login leaked another request's absolute lifetime")
					return
				}
				results <- nil
				return
			}
			if r.StatusCode == 409 && len(r.Cookies()) == 0 {
				results <- errLimit
				return
			}
			results <- fmt.Errorf("mixed admission status=%d cookies=%d", r.StatusCode, len(r.Cookies()))
		}()
	}
	close(start)
	admitted, rejected := 0, 0
	for range 24 {
		switch err := <-results; {
		case err == nil:
			admitted++
		case errors.Is(err, errLimit):
			rejected++
		default:
			t.Error(err)
		}
	}
	if admitted != 1 || rejected != 23 {
		t.Fatalf("mixed slots admitted=%d rejected=%d", admitted, rejected)
	}
	for replica := range 2 {
		for _, cookie := range []*http.Cookie{ordinary, remembered} {
			expect(t, f, replica, "GET", "/me", cookie, 200)
		}
	}
	expect(t, f, 0, "POST", "/logout", remembered, 204)
	for replica := range 2 {
		rejectWithoutCookie(t, f, replica, "GET", "/me", remembered, 401)
	}
	expect(t, f, 1, "POST", "/login-fixture?remember=true", nil, 201)
	rejectWithoutCookie(t, f, 0, "POST", "/login-fixture", nil, 409)
}
