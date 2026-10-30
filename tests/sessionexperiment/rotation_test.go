package sessionexperiment

import (
	"context"
	"maps"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

func rotationProfile(t *testing.T, profile string) (*fixture, string) {
	t.Helper()
	f := setupAuth(t)
	if profile == "admin" {
		return adminView(f), "/login-fixture"
	}
	if profile == "remember" {
		return f, "/login-fixture?remember=true"
	}
	return f, "/login-fixture"
}

func rotationResponse(t *testing.T, result poolRequestResult, status int) *http.Response {
	t.Helper()
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.elapsed >= 4*time.Second || result.response.StatusCode != status {
		t.Fatalf("rotation response status=%d elapsed=%s want=%d", result.response.StatusCode, result.elapsed, status)
	}
	if status != 201 && len(result.response.Header.Values("Set-Cookie")) != 0 {
		t.Fatal("unsuccessful rotation emitted Set-Cookie")
	}
	return result.response
}

func rotateCookie(t *testing.T, f *fixture, replica int, old *http.Cookie) *http.Cookie {
	t.Helper()
	r := rotationResponse(t, timedPoolRequest(f, replica, "POST", "/rotate-fixture?remember=true", old), 201)
	if len(r.Header.Values("Set-Cookie")) != 1 || len(r.Cookies()) != 1 {
		t.Fatal("rotation did not emit exactly one valid Cookie")
	}
	next := r.Cookies()[0]
	if next.Value == "" || next.Value == old.Value || next.Name != old.Name || next.Path != old.Path ||
		!next.Secure || !next.HttpOnly || next.SameSite != http.SameSiteStrictMode {
		t.Fatal("rotation credential or Cookie policy")
	}
	if old.Expires.IsZero() {
		if !next.Expires.IsZero() || next.MaxAge != 0 {
			t.Fatal("rotation upgraded persistence")
		}
	} else if next.Expires.IsZero() || next.MaxAge <= 0 {
		t.Fatal("rotation lost explicit persistence")
	}
	return next
}

func rotationMillis(t *testing.T, fields map[string]string, key string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(fields[key], 10, 64)
	if err != nil || n <= 0 {
		t.Fatalf("invalid rotation %s: %v", key, err)
	}
	return n
}

// Compare the entire namespace, allowing only one new token, one tombstone and
// the exact member/expiry exchange in the existing subject index.
func assertRotationTransfer(t *testing.T, f *fixture, old, next *http.Cookie, before map[string]memoryRecord) {
	t.Helper()
	after := memorySnapshot(t, f)
	oldKey, newKey := f.stores[0].key(old.Value), f.stores[0].key(next.Value)
	index := f.stores[0].prefix + "subject:" + before[oldKey].fields["subject"]
	if len(after) != len(before)+1 {
		t.Fatal("rotation changed unexpected key count")
	}
	for key, record := range before {
		if key == index {
			continue
		}
		if key == oldKey {
			record.fields = maps.Clone(record.fields)
			delete(record.fields, "data")
		}
		if !reflect.DeepEqual(after[key], record) {
			t.Fatal("rotation changed tombstone deadline or another session")
		}
	}
	nextRecord := after[newKey]
	prior := before[oldKey]
	deadline := rotationMillis(t, prior.fields, "deadline")
	expiry := rotationMillis(t, nextRecord.fields, "expiry")
	if len(nextRecord.fields) != 4 || nextRecord.fields["subject"] != prior.fields["subject"] ||
		nextRecord.fields["deadline"] != prior.fields["deadline"] || nextRecord.expires != prior.expires ||
		expiry < rotationMillis(t, prior.fields, "expiry") || expiry > deadline {
		t.Fatal("rotation changed identity, shortened idle expiry or extended absolute deadline")
	}
	priorDeadline, priorValues, err := (scs.GobCodec{}).Decode([]byte(prior.fields["data"]))
	if err != nil {
		t.Fatal(err)
	}
	nextDeadline, nextValues, err := (scs.GobCodec{}).Decode([]byte(nextRecord.fields["data"]))
	if err != nil || !nextDeadline.Equal(priorDeadline) || !reflect.DeepEqual(priorValues, nextValues) {
		t.Fatal("rotation changed payload or login deadline")
	}
	want, got := make(map[string]float64), make(map[string]float64)
	var latest int64
	for _, slot := range before[index].slots {
		want[slot.Member.(string)] = slot.Score
	}
	delete(want, oldKey)
	want[newKey] = float64(expiry)
	for _, slot := range after[index].slots {
		got[slot.Member.(string)] = slot.Score
		latest = max(latest, int64(slot.Score))
	}
	if !reflect.DeepEqual(want, got) || after[index].expires.Milliseconds() != latest {
		t.Fatal("rotation did not transfer exactly one slot with correct index expiry")
	}
	if !next.Expires.IsZero() && next.Expires.After(time.UnixMilli(deadline)) {
		t.Fatal("persistent Cookie exceeds original deadline")
	}
}

func TestRotationAtFullCapacityPreservesPolicy(t *testing.T) {
	for _, profile := range []string{"personal", "remember", "admin"} {
		t.Run(profile, func(t *testing.T) {
			f, path := rotationProfile(t, profile)
			old := expect(t, f, 0, "POST", path, nil, 201).Cookies()[0]
			others := []*http.Cookie{
				expect(t, f, 0, "POST", "/login-fixture?remember=true", nil, 201).Cookies()[0],
				expect(t, f, 1, "POST", "/login-fixture", nil, 201).Cookies()[0],
			}
			before := memorySnapshot(t, f)
			next := rotateCookie(t, f, 1, old)
			assertRotationTransfer(t, f, old, next, before)
			for replica := range 2 {
				rotationResponse(t, timedPoolRequest(f, replica, "POST", "/rotate-fixture", old), 401)
				rejectWithoutCookie(t, f, replica, "GET", "/me", old, 401)
				for _, cookie := range append(others, next) {
					expect(t, f, replica, "GET", "/me", cookie, 200)
				}
				rejectWithoutCookie(t, f, replica, "POST", "/login-fixture", nil, 409)
			}
			// Repeated successful rotations also retain the original login deadline.
			before = memorySnapshot(t, f)
			last := rotateCookie(t, f, 0, next)
			assertRotationTransfer(t, f, next, last, before)
			expect(t, f, 1, "POST", "/logout", last, 204)
			expect(t, f, 0, "POST", "/login-fixture", nil, 201)
			rejectWithoutCookie(t, f, 1, "POST", "/login-fixture", nil, 409)
		})
	}
}

func waitRotationLoad(t *testing.T, f *fixture) {
	t.Helper()
	select {
	case <-f.loaded:
	case <-time.After(time.Second):
		t.Fatal("rotation load barrier")
	}
}

func TestConcurrentRotationHasOneWinner(t *testing.T) {
	for _, profile := range []string{"personal", "remember", "admin"} {
		t.Run(profile, func(t *testing.T) {
			f, path := rotationProfile(t, profile)
			old := expect(t, f, 0, "POST", path, nil, 201).Cookies()[0]
			for range 2 {
				expect(t, f, 1, "POST", "/login-fixture", nil, 201)
			}
			before := memorySnapshot(t, f)
			start := make(chan struct{})
			resume := sync.OnceFunc(func() { close(f.resume); close(start) })
			t.Cleanup(resume)
			finished := make(chan poolRequestResult, 12)
			logins := make(chan poolRequestResult, 24)
			for i := range cap(finished) {
				go func() { finished <- timedPoolRequest(f, i%2, "POST", "/late-rotate-fixture", old) }()
			}
			for i := range cap(logins) {
				go func() {
					<-start
					logins <- timedPoolRequest(f, i%2, "POST", "/login-fixture", nil)
				}()
			}
			for range cap(finished) {
				waitRotationLoad(t, f)
			}
			resume()
			var next *http.Cookie
			for range cap(finished) {
				result := <-finished
				if result.err != nil {
					t.Fatal(result.err)
				}
				if result.response.StatusCode == 201 {
					if next != nil || len(result.response.Cookies()) != 1 {
						t.Fatal("concurrent rotation produced multiple credentials")
					}
					next = rotationResponse(t, result, 201).Cookies()[0]
				} else {
					rotationResponse(t, result, 401)
				}
			}
			if next == nil {
				t.Fatal("concurrent rotation had no winner")
			}
			for range cap(logins) {
				rotationResponse(t, <-logins, 409)
			}
			assertRotationTransfer(t, f, old, next, before)
			for replica := range 2 {
				expect(t, f, replica, "GET", "/me", next, 200)
				rejectWithoutCookie(t, f, replica, "GET", "/me", old, 401)
			}
			rejectWithoutCookie(t, f, 1, "POST", "/login-fixture", nil, 409)
		})
	}
}

func TestRotationRejectsPreviouslyLoadedSave(t *testing.T) {
	for _, profile := range []string{"personal", "admin"} {
		t.Run(profile, func(t *testing.T) {
			f, path := rotationProfile(t, profile)
			old := expect(t, f, 0, "POST", path, nil, 201).Cookies()[0]
			resume := sync.OnceFunc(func() { close(f.resume) })
			t.Cleanup(resume)
			finished := make(chan poolRequestResult, 1)
			go func() { finished <- timedPoolRequest(f, 0, "POST", "/late-save", old) }()
			waitRotationLoad(t, f)
			next := rotateCookie(t, f, 1, old)
			before := memorySnapshot(t, f)
			resume()
			rotationResponse(t, <-finished, 401)
			assertMemoryUnchanged(t, f, before)
			for replica := range 2 {
				expect(t, f, replica, "GET", "/me", next, 200)
				rejectWithoutCookie(t, f, replica, "GET", "/me", old, 401)
			}
		})
	}
}

func TestRotationMethodAndAnonymousRejection(t *testing.T) {
	f := setupAuth(t)
	old := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	before := memorySnapshot(t, f)
	rotationResponse(t, timedPoolRequest(f, 0, "GET", "/rotate-fixture", old), 405)
	rotationResponse(t, timedPoolRequest(f, 0, "POST", "/rotate-fixture", nil), 401)
	assertMemoryUnchanged(t, f, before)
}

func TestRotationLostAcknowledgementDoesNotIssueCookie(t *testing.T) {
	pending := make(chan saveAttempt, 1)
	f := setupAuth(t, func(_ *scs.SessionManager, store *guardedStore) {
		store.rotationReceipt = func(attempt saveAttempt) error {
			pending <- attempt
			return errOutcomeUnknown
		}
	})
	old := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	before := memorySnapshot(t, f)
	rotationResponse(t, timedPoolRequest(f, 0, "POST", "/rotate-fixture", old), 503)
	var attempt saveAttempt
	select {
	case attempt = <-pending:
	case <-time.After(time.Second):
		t.Fatal("rotation acknowledgement fault was not reached")
	}
	next := attemptCookie(f, attempt) // Privileged observation, never delivered to the browser.
	assertRotationTransfer(t, f, old, next, before)
	if err := attempt.check(context.Background(), f.stores[1]); err != nil {
		t.Fatal(err)
	}
	for replica := range 2 {
		rejectWithoutCookie(t, f, replica, "GET", "/me", old, 401)
		expect(t, f, replica, "GET", "/me", next, 200)
	}
	for range 2 {
		expect(t, f, 1, "POST", "/login-fixture", nil, 201)
	}
	before = memorySnapshot(t, f)
	rotationResponse(t, timedPoolRequest(f, 1, "POST", "/rotate-fixture", old), 401)
	assertMemoryUnchanged(t, f, before)
	if len(pending) != 0 {
		t.Fatal("uncertain rotation retried or generated another candidate")
	}
	rejectWithoutCookie(t, f, 1, "POST", "/login-fixture", nil, 409)
}
