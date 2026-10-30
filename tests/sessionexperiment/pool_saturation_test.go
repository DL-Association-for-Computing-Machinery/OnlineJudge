package sessionexperiment

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

type poolRequestResult struct {
	response *http.Response
	err      error
	elapsed  time.Duration
}

func timedPoolRequest(f *fixture, replica int, method, path string, cookie *http.Cookie) poolRequestResult {
	start := time.Now()
	r, err := request(f, replica, method, path, cookie)
	return poolRequestResult{r, err, time.Since(start)}
}

func checkPoolFailure(t *testing.T, result poolRequestResult) {
	t.Helper()
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.response.StatusCode != 503 || len(result.response.Cookies()) != 0 {
		t.Fatalf("saturated pool response=%d cookies=%d", result.response.StatusCode, len(result.response.Cookies()))
	}
	if result.elapsed >= 4*time.Second {
		t.Fatalf("saturated request exceeded failure budget: %s", result.elapsed)
	}
}

func TestSessionPoolSaturationAcrossAPIs(t *testing.T) {
	f := setupAuth(t)
	admin := adminView(f)
	views := []*fixture{f, admin}
	cookies := []*http.Cookie{
		expect(t, f, 0, "POST", "/login-fixture?remember=true", nil, 201).Cookies()[0],
		expect(t, admin, 1, "POST", "/login-fixture", nil, 201).Cookies()[0],
	}
	observer := poolObserver(t, f)
	observed := observedFixture(f, observer)
	before := memorySnapshot(t, observed)
	release0 := holdSessionPool(t, f.stores[0].client)
	release1 := holdSessionPool(t, f.stores[1].client)
	full := [2]int{sessionRedisPoolLimit, sessionRedisPoolLimit}
	if counts := apiConnectionCounts(t, f, observer); counts != full {
		t.Fatalf("held socket counts=%v want=%v", counts, full)
	}
	t.Logf("Redis CLIENT LIST: API connections=%v, total=%d; observer excluded", full, full[0]+full[1])
	start := make(chan struct{})
	results := make(chan poolRequestResult, 16)
	for scope, view := range views {
		for replica := range 2 {
			for _, operation := range [][2]string{{"GET", "/me"}, {"POST", "/activity-fixture"}, {"POST", "/logout"}, {"POST", "/login-fixture?remember=true"}} {
				cookie := cookies[scope]
				if operation[1] == "/login-fixture?remember=true" {
					cookie = nil
				}
				go func() {
					<-start
					results <- timedPoolRequest(view, replica, operation[0], operation[1], cookie)
				}()
			}
		}
	}
	close(start)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	until := time.NewTimer(5 * time.Second)
	defer until.Stop()
	samples := 0
	for remaining := 16; remaining > 0; {
		select {
		case result := <-results:
			checkPoolFailure(t, result)
			remaining--
		case <-ticker.C:
			samples++
			if counts := apiConnectionCounts(t, f, observer); counts != full {
				t.Fatalf("connection budget changed under pressure: %v", counts)
			}
		case <-until.C:
			t.Fatal("saturated HTTP requests did not finish")
		}
	}
	if counts := apiConnectionCounts(t, f, observer); counts != full {
		t.Fatalf("connection budget after saturated requests: %v", counts)
	}
	assertMemoryUnchanged(t, observed, before)
	t.Logf("16 concurrent HTTP failures with no Cookie/state changes; %d live connection samples", samples)
	release0()
	release1()
	for scope, view := range views {
		for replica := range 2 {
			expect(t, view, replica, "GET", "/me", cookies[scope], 200)
		}
		expect(t, view, 1, "POST", "/logout", cookies[scope], 204)
		for replica := range 2 {
			rejectWithoutCookie(t, view, replica, "GET", "/me", cookies[scope], 401)
		}
		assertThreeFreshSlots(t, view)
	}
	for _, count := range apiConnectionCounts(t, f, observer) {
		if count > sessionRedisPoolLimit {
			t.Fatal("recovery exceeded API connection budget")
		}
	}
}

func TestSessionPoolSaturationRejectsLoadedSave(t *testing.T) {
	for _, scope := range []string{"personal", "admin"} {
		t.Run(scope, func(t *testing.T) {
			f := setupAuth(t)
			view := f
			if scope == "admin" {
				view = adminView(f)
			}
			cookie := expect(t, view, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			original := readLifetime(t, view, cookie)
			observer := poolObserver(t, f)
			observed := observedFixture(f, observer)
			before := memorySnapshot(t, observed)
			resume := sync.OnceFunc(func() { close(f.resume) })
			t.Cleanup(resume)
			finished := make(chan poolRequestResult, 1)
			go func() { finished <- timedPoolRequest(view, 0, "POST", "/late-save", cookie) }()
			select {
			case <-f.loaded:
			case <-time.After(time.Second):
				t.Fatal("pool saturation load barrier")
			}
			release := holdSessionPool(t, f.stores[0].client)
			expect(t, view, 1, "GET", "/me", cookie, 200)
			if counts := apiConnectionCounts(t, f, observer); counts[0] != sessionRedisPoolLimit || counts[1] < 1 || counts[1] > sessionRedisPoolLimit {
				t.Fatalf("single-API saturation socket counts: %v", counts)
			}
			resume()
			select {
			case result := <-finished:
				checkPoolFailure(t, result)
			case <-time.After(4 * time.Second):
				t.Fatal("loaded save did not finish after pool saturation")
			}
			assertMemoryUnchanged(t, observed, before)
			release()
			for replica := range 2 {
				expect(t, view, replica, "GET", "/me", cookie, 200)
			}
			expect(t, view, 0, "POST", "/activity-fixture", cookie, 201)
			if renewed := readLifetime(t, view, cookie); !renewed.deadline.Equal(original.deadline) || renewed.expiry.Before(original.expiry) {
				t.Fatal("recovered save changed original deadline or shortened idle expiry")
			}
		})
	}
}
