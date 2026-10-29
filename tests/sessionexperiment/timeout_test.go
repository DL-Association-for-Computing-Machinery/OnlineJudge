package sessionexperiment

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

// CLIENT PAUSE affects the entire disposable Redis, so these tests must remain
// serial. Its finite duration also restores service if an assertion fails.
func pauseRedis(t *testing.T, f *fixture) {
	t.Helper()
	if err := f.stores[0].client.Do(context.Background(), "CLIENT", "PAUSE", 5000, "ALL").Err(); err != nil {
		t.Fatal(err)
	}
}

func awaitRedis(t *testing.T, f *fixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for {
		if err := f.stores[0].client.Ping(ctx).Err(); err == nil {
			return
		}
		if ctx.Err() != nil {
			t.Fatal("Redis did not resume within recovery budget")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRedisTimeoutAcrossAPIs(t *testing.T) {
	f := setup(t)
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	pauseRedis(t, f)
	// Both operations block while loading the session. Logout must not report
	// success or remove the browser cookie when shared storage cannot be read.
	type result struct {
		response *http.Response
		err      error
		elapsed  time.Duration
	}
	results := make(chan result, 2)
	for replica := range 2 {
		go func() {
			method, path := "GET", "/me"
			if replica == 1 {
				method, path = "POST", "/logout"
			}
			start := time.Now()
			r, err := request(f, replica, method, path, cookie)
			results <- result{r, err, time.Since(start)}
		}()
	}
	for range 2 {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.response.StatusCode != 503 || len(r.response.Cookies()) != 0 {
			t.Fatalf("timeout status=%d cookies=%d", r.response.StatusCode, len(r.response.Cookies()))
		}
		if r.elapsed >= 4*time.Second {
			t.Fatalf("request exceeded bounded failure budget: %s", r.elapsed)
		}
	}
	awaitRedis(t, f)
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 200)
	}
	// Reconcile and explicitly retry after recovery, then verify shared revoke
	// and admission accounting rather than assuming a timeout is a logout.
	expect(t, f, 1, "POST", "/logout", cookie, 204)
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 401)
	}
	for i := range 3 {
		expect(t, f, i%2, "POST", "/login-fixture", nil, 201)
	}
	expect(t, f, 0, "POST", "/login-fixture", nil, 409)
}

func TestRedisTimeoutDuringLoadedSave(t *testing.T) {
	f := setup(t)
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	before, found, err := f.stores[0].FindCtx(context.Background(), cookie.Value)
	if err != nil || !found {
		t.Fatal("missing initial session")
	}
	deadline, _, err := (scs.GobCodec{}).Decode(before)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan *http.Response, 1)
	failed := make(chan error, 1)
	go func() {
		r, err := request(f, 0, "POST", "/late-save", cookie)
		if err != nil {
			failed <- err
			return
		}
		finished <- r
	}()
	select {
	case <-f.loaded:
	case <-time.After(5 * time.Second):
		close(f.resume)
		t.Fatal("load barrier")
	}
	pauseRedis(t, f)
	close(f.resume)
	select {
	case r := <-finished:
		if r.StatusCode != 503 || len(r.Cookies()) != 0 {
			t.Fatalf("save timeout status=%d cookies=%d", r.StatusCode, len(r.Cookies()))
		}
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(4 * time.Second):
		t.Fatal("save exceeded bounded failure budget")
	}
	awaitRedis(t, f)
	// A timed-out write may still execute. We assert the invariant (one slot,
	// same absolute deadline), not that cancellation rolled the write back.
	after, found, err := f.stores[1].FindCtx(context.Background(), cookie.Value)
	if err != nil || !found {
		t.Fatal("missing session after recovery")
	}
	recoveredDeadline, _, err := (scs.GobCodec{}).Decode(after)
	if err != nil || !recoveredDeadline.Equal(deadline) {
		t.Fatal("absolute deadline changed after timeout")
	}
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 200)
	}
	for i := range 2 {
		expect(t, f, i, "POST", "/login-fixture", nil, 201)
	}
	expect(t, f, 1, "POST", "/login-fixture", nil, 409)
	expect(t, f, 1, "POST", "/logout", cookie, 204)
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 401)
	}
}

func TestRedisAdmissionTimeoutHasUnknownOutcome(t *testing.T) {
	f := setup(t)
	pauseRedis(t, f)
	start := time.Now()
	r := expect(t, f, 0, "POST", "/login-fixture", nil, 503)
	if len(r.Cookies()) != 0 || time.Since(start) >= 4*time.Second {
		t.Fatal("timed-out admission sent cookie or exceeded failure budget")
	}
	awaitRedis(t, f)
	// This is fixture-only inspection, not a production reconciliation API.
	// A timeout cannot prove rollback: a queued creation may consume one slot
	// without delivering its cookie. Do not retry it as if nothing happened.
	occupied, err := f.stores[1].client.ZCard(context.Background(),
		f.stores[1].prefix+"subject:"+digest("fixture-user")).Result()
	if err != nil || occupied < 0 || occupied > 1 {
		t.Fatalf("unexpected admission accounting: occupied=%d err=%v", occupied, err)
	}
	t.Logf("undelivered admission occupies %d slot(s) after recovery; timeout is not rollback", occupied)
	for i := int64(0); i < 3-occupied; i++ {
		cookie := expect(t, f, int(i%2), "POST", "/login-fixture", nil, 201).Cookies()[0]
		for replica := range 2 {
			expect(t, f, replica, "GET", "/me", cookie, 200)
		}
	}
	for replica := range 2 {
		expect(t, f, replica, "POST", "/login-fixture", nil, 409)
	}
}
