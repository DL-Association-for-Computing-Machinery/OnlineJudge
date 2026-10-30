package sessionexperiment

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type apiSocketIDs [2]map[string]bool

// Match only initialized sockets belonging to these two random API labels.
// In particular, neither the observer nor another fixture is a kill target.
func readAPISocketIDs(t *testing.T, f *fixture, observer *redis.Client) apiSocketIDs {
	t.Helper()
	names := [2]string{f.stores[0].client.Options().ClientName, f.stores[1].client.Options().ClientName}
	if names[0] == "" || names[1] == "" || names[0] == names[1] {
		t.Fatal("API clients need distinct nonempty labels")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	list, err := observer.ClientList(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	ids := apiSocketIDs{make(map[string]bool), make(map[string]bool)}
	for line := range strings.SplitSeq(list, "\n") {
		var name, id string
		for _, field := range strings.Fields(line) {
			if value, ok := strings.CutPrefix(field, "name="); ok {
				name = value
			}
			if value, ok := strings.CutPrefix(field, "id="); ok {
				id = value
			}
		}
		for replica := range 2 {
			if name != names[replica] {
				continue
			}
			if n, err := strconv.ParseInt(id, 10, 64); err != nil || n <= 0 || ids[replica][id] {
				t.Fatal("invalid or duplicate API connection ID")
			}
			ids[replica][id] = true
		}
	}
	return ids
}

func checkReconnectRead(t *testing.T, result poolRequestResult) bool {
	t.Helper()
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.elapsed >= 4*time.Second || len(result.response.Header.Values("Set-Cookie")) != 0 {
		t.Fatal("reconnect read exceeded budget or emitted a Cookie")
	}
	switch result.response.StatusCode {
	case 200:
		return false
	case 503:
		return true
	default:
		t.Fatalf("reconnect read status=%d", result.response.StatusCode)
		return false
	}
}

func checkReconnectedSockets(t *testing.T, current, old apiSocketIDs, full bool) {
	t.Helper()
	for replica, ids := range current {
		if len(ids) > sessionRedisPoolLimit || (full && len(ids) != sessionRedisPoolLimit) {
			t.Fatalf("API %d live connections=%d (full=%v)", replica, len(ids), full)
		}
		for id := range ids {
			if old[replica][id] {
				t.Fatal("disconnected API socket survived or was reused")
			}
		}
	}
}

func disconnectAPISockets(t *testing.T, f *fixture, observer *redis.Client) apiSocketIDs {
	t.Helper()
	old := readAPISocketIDs(t, f, observer)
	checkReconnectedSockets(t, old, apiSocketIDs{}, true)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, ids := range old {
		for id := range ids {
			n, err := observer.ClientKillByFilter(ctx, "ID", id).Result()
			if err != nil || n != 1 {
				t.Fatalf("disconnect exact fixture connection: killed=%d err=%v", n, err)
			}
		}
	}
	current := readAPISocketIDs(t, f, observer)
	if len(current[0])+len(current[1]) != 0 {
		t.Fatal("API connections remained before request release")
	}
	return old
}

func fillAndReleaseAPIPools(t *testing.T, f *fixture, observer *redis.Client, old apiSocketIDs) {
	t.Helper()
	release0 := holdSessionPool(t, f.stores[0].client)
	release1 := holdSessionPool(t, f.stores[1].client)
	checkReconnectedSockets(t, readAPISocketIDs(t, f, observer), old, true)
	release0()
	release1()
}

// Reads can temporarily fail on stale transports with retries disabled. They must
// never become 401 or mutate session state; later reads must recover normally.
func reconnectReadBurst(t *testing.T, f *fixture, observer *redis.Client, old apiSocketIDs, cookies [2]*http.Cookie) {
	t.Helper()
	views := [2]*fixture{f, adminView(f)}
	start := make(chan struct{})
	stop := make(chan struct{})
	var workers sync.WaitGroup
	defer func() {
		close(stop)
		workers.Wait()
	}()
	results := make(chan poolRequestResult, 128)
	for scope, view := range views {
		for replica := range 2 {
			for range 8 {
				workers.Add(1)
				go func() {
					defer workers.Done()
					select {
					case <-start:
					case <-stop:
						return
					}
					for range 4 {
						select {
						case <-stop:
							return
						default:
						}
						results <- timedPoolRequest(view, replica, "GET", "/me", cookies[scope])
					}
				}()
			}
		}
	}
	var peak [2]int
	samples, unavailable := 0, 0
	sample := func() {
		ids := readAPISocketIDs(t, f, observer)
		checkReconnectedSockets(t, ids, old, false)
		for replica := range 2 {
			peak[replica] = max(peak[replica], len(ids[replica]))
		}
		samples++
	}
	sample()
	close(start)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	until := time.NewTimer(5 * time.Second)
	defer until.Stop()
	for remaining := cap(results); remaining > 0; {
		select {
		case result := <-results:
			if checkReconnectRead(t, result) {
				unavailable++
			}
			sample()
			remaining--
		case <-ticker.C:
			sample()
		case <-until.C:
			t.Fatal("reconnection read burst did not finish")
		}
	}
	for scope, view := range views {
		for replica := range 2 {
			if checkReconnectRead(t, timedPoolRequest(view, replica, "GET", "/me", cookies[scope])) {
				t.Fatal("reads remained unavailable after reconnect burst")
			}
		}
	}
	t.Logf("128 reconnect reads, %d unavailable; %d CLIENT LIST samples, named socket peaks=%v", unavailable, samples, peak)
}

func TestSessionReconnectBudgetAcrossAPIs(t *testing.T) {
	f := setupAuth(t)
	admin := adminView(f)
	cookies := [2]*http.Cookie{
		expect(t, f, 0, "POST", "/login-fixture?remember=true", nil, 201).Cookies()[0],
		expect(t, admin, 1, "POST", "/login-fixture", nil, 201).Cookies()[0],
	}
	observer := poolObserver(t, f)
	observed := observedFixture(f, observer)
	runID := redisRunID(t, observer)
	observerID, err := observer.ClientID(context.Background()).Result()
	if err != nil {
		t.Fatal(err)
	}
	before := memorySnapshot(t, observed)
	fillAndReleaseAPIPools(t, f, observer, apiSocketIDs{})
	for range 2 {
		old := disconnectAPISockets(t, f, observer)
		reconnectReadBurst(t, f, observer, old, cookies)
		fillAndReleaseAPIPools(t, f, observer, old)
		assertMemoryUnchanged(t, observed, before)
	}
	if id, err := observer.ClientID(context.Background()).Result(); err != nil || id != observerID || redisRunID(t, observer) != runID {
		t.Fatal("disconnect affected observer or restarted Redis")
	}
	for scope, view := range []*fixture{f, admin} {
		original := readLifetime(t, view, cookies[scope])
		expect(t, view, 0, "POST", "/activity-fixture", cookies[scope], 201)
		if next := readLifetime(t, view, cookies[scope]); next.deadline != original.deadline || next.expiry.Before(original.expiry) {
			t.Fatal("activity after reconnect changed original deadline or shortened idle expiry")
		}
		expect(t, view, 1, "POST", "/logout", cookies[scope], 204)
		for replica := range 2 {
			rejectWithoutCookie(t, view, replica, "GET", "/me", cookies[scope], 401)
		}
		assertThreeFreshSlots(t, view)
	}
}

func TestSessionReconnectCannotReviveLoadedSession(t *testing.T) {
	for _, scope := range []string{"personal", "admin"} {
		t.Run(scope, func(t *testing.T) {
			f := setupAuth(t)
			view := f
			if scope == "admin" {
				view = adminView(f)
			}
			cookie := expect(t, view, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			survivor := expect(t, view, 1, "POST", "/login-fixture", nil, 201).Cookies()[0]
			resume := sync.OnceFunc(func() { close(f.resume) })
			t.Cleanup(resume)
			finished := make(chan poolRequestResult, 1)
			go func() { finished <- timedPoolRequest(view, 0, "POST", "/late-save", cookie) }()
			select {
			case <-f.loaded:
			case <-time.After(time.Second):
				t.Fatal("reconnect loaded-save barrier")
			}
			expect(t, view, 1, "POST", "/logout", cookie, 204)
			observer := poolObserver(t, f)
			observed := observedFixture(f, observer)
			before := memorySnapshot(t, observed)
			fillAndReleaseAPIPools(t, f, observer, apiSocketIDs{})
			old := disconnectAPISockets(t, f, observer)
			for replica := range 2 {
				checkReconnectRead(t, timedPoolRequest(view, replica, "GET", "/me", survivor))
				if checkReconnectRead(t, timedPoolRequest(view, replica, "GET", "/me", survivor)) {
					t.Fatal("surviving session remained unavailable after reconnect")
				}
			}
			fillAndReleaseAPIPools(t, f, observer, old)
			resume()
			select {
			case result := <-finished:
				if result.err != nil || result.response.StatusCode != 401 || len(result.response.Header.Values("Set-Cookie")) != 0 {
					t.Fatalf("revoked loaded save after reconnect: %+v", result)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("revoked loaded save did not finish")
			}
			assertMemoryUnchanged(t, observed, before)
			for replica := range 2 {
				rejectWithoutCookie(t, view, replica, "GET", "/me", cookie, 401)
				expect(t, view, replica, "GET", "/me", survivor, 200)
			}
			expect(t, view, 1, "POST", "/logout", survivor, 204)
			assertThreeFreshSlots(t, view)
		})
	}
}
