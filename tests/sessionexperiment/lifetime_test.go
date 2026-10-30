package sessionexperiment

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

func shortLifetime(lifetime, idle time.Duration) sessionConfigurer {
	return func(manager *scs.SessionManager, _ *guardedStore) {
		manager.Lifetime, manager.IdleTimeout = lifetime, idle
	}
}

// Observe Redis's clock, which the store uses to authorize expiration. Bounds
// keep a stopped server or clock regression from hanging the behavioral suite.
func redisTime(t *testing.T, f *fixture) time.Time {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	now, err := f.stores[0].client.Time(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	return now
}

func awaitRedisTime(t *testing.T, f *fixture, target time.Time) {
	t.Helper()
	until := time.Now().Add(8 * time.Second)
	for redisTime(t, f).Before(target) {
		if time.Now().After(until) {
			t.Fatal("Redis clock did not reach session boundary")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type lifetimeState struct {
	deadline, expiry time.Time
	slot             string
}

func readLifetime(t *testing.T, f *fixture, cookie *http.Cookie) lifetimeState {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	store := f.stores[0]
	key := store.key(cookie.Value)
	fields, err := store.client.HGetAll(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	parse := func(field string) time.Time {
		n, err := strconv.ParseInt(fields[field], 10, 64)
		if err != nil {
			t.Fatalf("missing/invalid %s: %v", field, err)
		}
		return time.UnixMilli(n)
	}
	state := lifetimeState{parse("deadline"), parse("expiry"), store.prefix + "subject:" + fields["subject"]}
	score, err := store.client.ZScore(ctx, state.slot, key).Result()
	if err != nil || int64(score) != state.expiry.UnixMilli() {
		t.Fatalf("slot does not match idle expiry: %v", err)
	}
	physical, err := store.client.PExpireTime(ctx, key).Result()
	if err != nil || physical.Milliseconds() != state.deadline.UnixMilli() {
		t.Fatalf("record does not retain original absolute deadline: %v", err)
	}
	indexExpiry, err := store.client.PExpireTime(ctx, state.slot).Result()
	if err != nil || indexExpiry.Milliseconds() != state.expiry.UnixMilli() {
		t.Fatalf("single-session index does not match idle expiry: %v", err)
	}
	if n, err := store.client.ZCard(ctx, state.slot).Result(); err != nil || n != 1 {
		t.Fatalf("activity occupied multiple slots: count=%d err=%v", n, err)
	}
	if state.expiry.After(state.deadline) {
		t.Fatal("idle expiry exceeds absolute deadline")
	}
	return state
}

func TestPollingDoesNotRenewIdleLifetime(t *testing.T) {
	f := setupAuth(t, shortLifetime(6*time.Second, 2*time.Second))
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	state := readLifetime(t, f, cookie)
	before := memorySnapshot(t, f)
	// Multiple successful background reads cross replicas without any write.
	for i := range 4 {
		awaitRedisTime(t, f, state.expiry.Add(-2*time.Second+time.Duration(i)*300*time.Millisecond))
		r := expect(t, f, i%2, "GET", "/me", cookie, 200)
		if len(r.Cookies()) != 0 {
			t.Fatal("background read emitted a renewal cookie")
		}
		assertMemoryUnchanged(t, f, before)
	}
	awaitRedisTime(t, f, state.expiry)
	beforeDenial := memorySnapshot(t, f)
	if len(beforeDenial) != 1 {
		t.Fatal("idle expiry did not remove slot index while retaining token record")
	}
	for replica := range 2 {
		rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
		rejectWithoutCookie(t, f, replica, "POST", "/activity-fixture", cookie, 401)
	}
	// The idle-expired record is deliberately retained as a tombstone until
	// its original deadline; denial must not rewrite or delete this evidence.
	assertMemoryUnchanged(t, f, beforeDenial)
	for i := range 3 {
		expect(t, f, i%2, "POST", "/login-fixture", nil, 201)
	}
	rejectWithoutCookie(t, f, 1, "POST", "/login-fixture", nil, 409)
	if n, err := f.stores[0].client.ZCard(context.Background(), state.slot).Result(); err != nil || n != 3 {
		t.Fatalf("idle-expired slot was not replaced: count=%d err=%v", n, err)
	}
}

func TestActivityRenewsIdleButNotAbsoluteLifetime(t *testing.T) {
	f := setupAuth(t, shortLifetime(6*time.Second, 2*time.Second))
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	original := readLifetime(t, f, cookie)
	before := memorySnapshot(t, f)
	rejectWithoutCookie(t, f, 1, "GET", "/activity-fixture", cookie, 405)
	assertMemoryUnchanged(t, f, before)
	awaitRedisTime(t, f, original.expiry.Add(-time.Second))
	current := original
	for i := 0; ; i++ {
		r := expect(t, f, i%2, "POST", "/activity-fixture", cookie, 201)
		if len(r.Cookies()) != 1 || r.Cookies()[0].Value != cookie.Value {
			t.Fatal("activity changed credential or omitted renewal cookie")
		}
		next := readLifetime(t, f, cookie)
		if !next.deadline.Equal(original.deadline) || next.expiry.Before(current.expiry) {
			t.Fatal("renewal changed absolute deadline or shortened idle lifetime")
		}
		if i == 0 && !next.expiry.After(original.expiry) {
			t.Fatal("activity did not extend idle lifetime")
		}
		current = next
		if current.expiry.Equal(original.deadline) {
			break
		}
		if i >= 20 {
			t.Fatal("activity never reached absolute lifetime cap")
		}
		awaitRedisTime(t, f, redisTime(t, f).Add(350*time.Millisecond))
	}
	// We have successfully used this cookie beyond its initial idle boundary.
	if !redisTime(t, f).After(original.expiry) {
		t.Fatal("did not exercise renewal beyond original idle lifetime")
	}
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 200)
	}
	awaitRedisTime(t, f, original.deadline)
	for replica := range 2 {
		rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
		rejectWithoutCookie(t, f, replica, "POST", "/activity-fixture", cookie, 401)
	}
	if n, err := f.stores[0].client.Exists(context.Background(), f.stores[0].key(cookie.Value), original.slot).Result(); err != nil || n != 0 {
		t.Fatalf("absolute expiry retained payload/index: count=%d err=%v", n, err)
	}
	for i := range 3 {
		expect(t, f, i%2, "POST", "/login-fixture", nil, 201)
	}
	rejectWithoutCookie(t, f, 0, "POST", "/login-fixture", nil, 409)
}

func TestOrdinaryLifetimeDefaults(t *testing.T) {
	f := setup(t)
	for _, manager := range f.managers {
		if manager.Lifetime != 12*time.Hour || manager.IdleTimeout != 2*time.Hour {
			t.Fatal("shortened test deadlines leaked into ordinary fixture defaults")
		}
	}
}

func TestLifetimeExpiryRejectsLoadedActivity(t *testing.T) {
	for _, boundary := range []string{"idle", "absolute"} {
		t.Run(boundary, func(t *testing.T) {
			lifetime, idle := 6*time.Second, time.Second
			if boundary == "absolute" {
				lifetime, idle = time.Second, 4*time.Second
			}
			f := setupAuth(t, shortLifetime(lifetime, idle))
			cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			state := readLifetime(t, f, cookie)
			finished := make(chan error, 1)
			go func() {
				r, err := request(f, 0, "POST", "/late-save", cookie)
				if err == nil && (r.StatusCode != 401 || len(r.Cookies()) != 0) {
					err = fmt.Errorf("expired loaded activity: status=%d cookies=%d", r.StatusCode, len(r.Cookies()))
				}
				finished <- err
			}()
			select {
			case <-f.loaded:
			case <-time.After(time.Second):
				close(f.resume)
				t.Fatal("activity load barrier")
			}
			awaitRedisTime(t, f, state.expiry)
			beforeDenial := memorySnapshot(t, f)
			wantKeys := 1 // Idle-expired token record remains until absolute expiry.
			if boundary == "absolute" {
				wantKeys = 0
			}
			if len(beforeDenial) != wantKeys {
				t.Fatal("unexpected expired record/index retention")
			}
			close(f.resume)
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			assertMemoryUnchanged(t, f, beforeDenial)
			for replica := range 2 {
				rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
			}
			for i := range 3 {
				expect(t, f, i%2, "POST", "/login-fixture", nil, 201)
			}
			rejectWithoutCookie(t, f, 0, "POST", "/login-fixture", nil, 409)
		})
	}
}
