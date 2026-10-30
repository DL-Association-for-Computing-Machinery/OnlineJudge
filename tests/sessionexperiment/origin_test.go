package sessionexperiment

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// Direct TLS fixture policy. Do not infer a public origin from Forwarded
// headers or silently add trusted origins/bypasses. Production proxy and
// browser compatibility require separate validation.
func sessionOriginProtection(next http.Handler) http.Handler {
	deny := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusForbidden)
	})
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(deny)
	guarded := protection.Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			guarded.ServeHTTP(w, r)
			return
		}
		origin, site := r.Header.Values("Origin"), r.Header.Values("Sec-Fetch-Site")
		// The standard fallback compares Host only. Explicit origins must
		// also retain HTTPS; metadata cannot override a contradictory Origin.
		if len(origin) > 0 && (len(origin) != 1 || origin[0] != "https://"+r.Host) ||
			len(site) > 0 && (len(site) != 1 || site[0] == "") {
			deny.ServeHTTP(w, r)
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

func originRequest(f *fixture, replica int, method, path string, cookie *http.Cookie, headers http.Header) poolRequestResult {
	start := time.Now()
	req, err := http.NewRequest(method, f.servers[replica].URL+f.routePrefix+path, nil)
	if err != nil {
		return poolRequestResult{err: err}
	}
	if headers != nil {
		req.Header = headers.Clone()
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	response, err := f.servers[replica].Client().Do(req)
	if err == nil {
		_ = response.Body.Close()
	}
	return poolRequestResult{response, err, time.Since(start)}
}

func originResponse(t *testing.T, result poolRequestResult, status int) *http.Response {
	t.Helper()
	if result.err != nil {
		t.Fatal(result.err)
	}
	r := result.response
	if result.elapsed >= 4*time.Second || r.StatusCode != status {
		t.Fatalf("origin response status=%d elapsed=%s want=%d", r.StatusCode, result.elapsed, status)
	}
	if status != 201 && status != 204 && len(r.Header.Values("Set-Cookie")) != 0 {
		t.Fatal("origin read/rejection emitted Set-Cookie")
	}
	if r.Header.Get("Access-Control-Allow-Origin") != "" || r.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("origin guard unexpectedly granted CORS access")
	}
	if status == 403 && r.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("origin denial is cacheable")
	}
	return r
}

func originHeaders(f *fixture, replica int) http.Header {
	return http.Header{"Origin": {f.servers[replica].URL}, "Sec-Fetch-Site": {"same-origin"}}
}

func hostileOrigins(f *fixture, replica int) map[string]http.Header {
	other := f.servers[1-replica].URL
	return map[string]http.Header{
		"cross-site":               {"Origin": {"https://foreign.invalid"}, "Sec-Fetch-Site": {"cross-site"}},
		"same-site":                {"Origin": {other}, "Sec-Fetch-Site": {"same-site"}},
		"metadata-only":            {"Sec-Fetch-Site": {"cross-site"}},
		"legacy-origin":            {"Origin": {"https://foreign.invalid"}},
		"other-port":               {"Origin": {other}},
		"downgrade":                {"Origin": {strings.Replace(f.servers[replica].URL, "https:", "http:", 1)}},
		"opaque":                   {"Origin": {"null"}},
		"malformed":                {"Origin": {"://invalid"}},
		"path":                     {"Origin": {f.servers[replica].URL + "/path"}},
		"empty-origin":             {"Origin": {""}},
		"empty-site":               {"Sec-Fetch-Site": {""}},
		"unknown-site":             {"Sec-Fetch-Site": {"invalid"}},
		"contradictory-origin":     {"Origin": {"https://foreign.invalid"}, "Sec-Fetch-Site": {"same-origin"}},
		"contradictory-site":       {"Origin": {f.servers[replica].URL}, "Sec-Fetch-Site": {"cross-site"}},
		"none-with-foreign-origin": {"Origin": {"https://foreign.invalid"}, "Sec-Fetch-Site": {"none"}},
		"multiple-origins":         {"Origin": {f.servers[replica].URL, "https://foreign.invalid"}},
		"multiple-sites":           {"Sec-Fetch-Site": {"same-origin", "cross-site"}},
		"combined-origins":         {"Origin": {f.servers[replica].URL + ", https://foreign.invalid"}},
		"combined-sites":           {"Sec-Fetch-Site": {"same-origin, cross-site"}},
		"forwarded-spoof":          {"Origin": {"https://foreign.invalid"}, "X-Forwarded-Host": {"foreign.invalid"}, "X-Forwarded-Proto": {"https"}, "Forwarded": {"host=foreign.invalid;proto=https"}},
	}
}

func TestCrossOriginSessionWritesHaveNoSideEffects(t *testing.T) {
	for _, profile := range []string{"personal", "remember", "admin"} {
		t.Run(profile, func(t *testing.T) {
			f, login := rotationProfile(t, profile)
			cookie := expect(t, f, 0, "POST", login, nil, 201).Cookies()[0]
			before := memorySnapshot(t, f)
			for replica := range 2 {
				for name, headers := range hostileOrigins(f, replica) {
					t.Run(name, func(t *testing.T) {
						for _, path := range []string{"/login-fixture", "/late-login-fixture", "/activity-fixture", "/logout", "/rotate-fixture", "/late-rotate-fixture", "/late-save"} {
							credential := cookie
							if strings.Contains(path, "login-fixture") {
								credential = nil
							}
							originResponse(t, originRequest(f, replica, "POST", path, credential, headers), 403)
						}
					})
				}
			}
			assertMemoryUnchanged(t, f, before)
			select {
			case <-f.loaded:
				t.Fatal("cross-origin request entered a loaded-session barrier")
			default:
			}
		})
	}
}

func TestSameOriginSessionLifecycle(t *testing.T) {
	for _, profile := range []string{"personal", "remember", "admin"} {
		for _, mode := range []string{"modern", "legacy", "non-browser", "user-initiated"} {
			t.Run(profile+"/"+mode, func(t *testing.T) {
				f, login := rotationProfile(t, profile)
				headers := func(replica int) http.Header {
					switch mode {
					case "legacy":
						return http.Header{"Origin": {f.servers[replica].URL}}
					case "non-browser":
						return nil
					case "user-initiated":
						return http.Header{"Sec-Fetch-Site": {"none"}}
					default:
						return originHeaders(f, replica)
					}
				}
				r := originResponse(t, originRequest(f, 0, "POST", login, nil, headers(0)), 201)
				cookie := r.Cookies()[0]
				original := readLifetime(t, f, cookie)
				originResponse(t, originRequest(f, 1, "POST", "/activity-fixture", cookie, headers(1)), 201)
				r = originResponse(t, originRequest(f, 1, "POST", "/rotate-fixture", cookie, headers(1)), 201)
				next := r.Cookies()[0]
				if next.Value == cookie.Value || readLifetime(t, f, next).deadline != original.deadline {
					t.Fatal("same-origin lifecycle changed deadline or failed rotation")
				}
				originResponse(t, originRequest(f, 0, "GET", "/me", next, headers(0)), 200)
				originResponse(t, originRequest(f, 0, "POST", "/logout", next, headers(0)), 204)
				originResponse(t, originRequest(f, 1, "GET", "/me", next, headers(1)), 401)
			})
		}
	}
}

func TestSafeMethodsCannotReachSessionWrites(t *testing.T) {
	for _, profile := range []string{"personal", "admin"} {
		t.Run(profile, func(t *testing.T) {
			f, login := rotationProfile(t, profile)
			cookie := expect(t, f, 0, "POST", login, nil, 201).Cookies()[0]
			before := memorySnapshot(t, f)
			// Releasing this barrier makes an accidental GET commit observable
			// immediately instead of confusing its timeout with method rejection.
			close(f.resume)
			for replica := range 2 {
				for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
					for _, path := range []string{"/late-save", "/login-fixture", "/late-login-fixture", "/activity-fixture", "/logout", "/rotate-fixture", "/late-rotate-fixture"} {
						originResponse(t, originRequest(f, replica, method, path, cookie, http.Header{"Origin": {"https://foreign.invalid"}, "Sec-Fetch-Site": {"cross-site"}}), 405)
					}
				}
				originResponse(t, originRequest(f, replica, "GET", "/me", cookie, http.Header{"Sec-Fetch-Site": {"cross-site"}}), 200)
			}
			assertMemoryUnchanged(t, f, before)
			select {
			case <-f.loaded:
				t.Fatal("safe method reached a write barrier")
			default:
			}
		})
	}
}

func TestOriginRejectionPrecedesBackendAccess(t *testing.T) {
	for _, backend := range []string{"redis", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			f := setupAuth(t)
			views := []*fixture{f, adminView(f)}
			cookies := make([]*http.Cookie, len(views))
			for i, view := range views {
				cookies[i] = expect(t, view, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			}
			observer := poolObserver(t, f)
			observed := observedFixture(f, observer)
			before := memorySnapshot(t, observed)
			for replica := range 2 {
				if backend == "redis" {
					if err := f.stores[replica].client.Close(); err != nil {
						t.Fatal(err)
					}
				} else {
					f.stores[replica].auth.pool.Close()
				}
			}
			for scope, view := range views {
				for replica := range 2 {
					for _, path := range []string{"/login-fixture", "/activity-fixture", "/logout", "/rotate-fixture"} {
						credential := cookies[scope]
						if path == "/login-fixture" {
							credential = nil
						}
						originResponse(t, originRequest(view, replica, "POST", path, credential, http.Header{"Sec-Fetch-Site": {"cross-site"}}), 403)
						originResponse(t, originRequest(view, replica, "POST", path, credential, originHeaders(view, replica)), 503)
					}
				}
			}
			assertMemoryUnchanged(t, observed, before)
		})
	}
}

func TestMixedOriginsDoNotInterfereAcrossAPIs(t *testing.T) {
	f := setupAuth(t)
	views := []*fixture{f, adminView(f)}
	cookies := make([]*http.Cookie, len(views))
	deadlines := make([]time.Time, len(views))
	for scope, view := range views {
		cookies[scope] = expect(t, view, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
		deadlines[scope] = readLifetime(t, view, cookies[scope]).deadline
	}
	start := make(chan struct{})
	finished := make(chan struct {
		result poolRequestResult
		status int
	}, 48)
	var workers sync.WaitGroup
	for scope, view := range views {
		for replica := range 2 {
			for i := range 12 {
				workers.Add(1)
				go func() {
					defer workers.Done()
					<-start
					headers, status := originHeaders(view, replica), 201
					if i%2 == 1 {
						headers, status = http.Header{"Sec-Fetch-Site": {"same-site"}}, 403
					}
					finished <- struct {
						result poolRequestResult
						status int
					}{originRequest(view, replica, "POST", "/activity-fixture", cookies[scope], headers), status}
				}()
			}
		}
	}
	close(start)
	defer workers.Wait()
	for range cap(finished) {
		item := <-finished
		originResponse(t, item.result, item.status)
	}
	for scope, view := range views {
		if readLifetime(t, view, cookies[scope]).deadline != deadlines[scope] {
			t.Fatal("mixed origins changed original deadline")
		}
		for range 2 {
			expect(t, view, 0, "POST", "/login-fixture", nil, 201)
		}
		rejectWithoutCookie(t, view, 1, "POST", "/login-fixture", nil, 409)
	}
}
