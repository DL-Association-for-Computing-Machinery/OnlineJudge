package sessionexperiment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"
)

func beginRecovery(t *testing.T, a *authAuthority) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	generation, err := a.beginRecovery(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return generation
}

func finishRecovery(t *testing.T, a *authAuthority, generation int64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := a.finishRecovery(ctx, generation); err != nil {
		t.Fatal(err)
	}
}

func assertRecoveryClosed(t *testing.T, f *fixture, cookie *http.Cookie) {
	t.Helper()
	for replica := range 2 {
		for _, req := range []struct{ method, path string }{
			{"GET", "/me"}, {"POST", "/logout"}, {"POST", "/login-fixture"},
		} {
			rejectWithoutCookie(t, f, replica, req.method, req.path, cookie, 503)
		}
		// Login without a cookie must also consult the persistent gate.
		rejectWithoutCookie(t, f, replica, "POST", "/login-fixture", nil, 503)
	}
}

func assertThreeFreshSlots(t *testing.T, f *fixture) {
	t.Helper()
	for i := range 3 {
		cookie := expect(t, f, i%2, "POST", "/login-fixture", nil, 201).Cookies()[0]
		for replica := range 2 {
			expect(t, f, replica, "GET", "/me", cookie, 200)
		}
	}
	for replica := range 2 {
		rejectWithoutCookie(t, f, replica, "POST", "/login-fixture", nil, 409)
	}
}

func redisRunID(t *testing.T, client *redis.Client) string {
	t.Helper()
	info, err := client.Info(context.Background(), "server").Result()
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(info, "\n") {
		if id, ok := strings.CutPrefix(strings.TrimSpace(line), "run_id:"); ok && id != "" {
			return id
		}
	}
	t.Fatal("Redis run_id missing")
	return ""
}

// The host driver owns only this invocation's recovery Redis. The protocol has
// no arbitrary command, container name or host path supplied by the test.
func restoreRedisSnapshot(t *testing.T) {
	t.Helper()
	nonce := os.Getenv("OJ_SESSION_RECOVERY_NONCE")
	if len(nonce) != 32 || strings.Trim(nonce, "0123456789abcdef") != "" {
		t.Fatal("requires disposable recovery driver")
	}
	fmt.Println("OJ_SESSION_RESTORE_REQUEST " + nonce)
	until := time.Now().Add(20 * time.Second)
	for {
		_, err := os.Stat("/tmp/oj-session-restored-" + nonce)
		if err == nil {
			return
		}
		if !errors.Is(err, os.ErrNotExist) || time.Now().After(until) {
			t.Fatalf("recovery acknowledgement: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRedisSnapshotRecovery(t *testing.T) {
	addr := os.Getenv("OJ_SESSION_RECOVERY_REDIS_ADDR")
	if addr == "" {
		t.Fatal("requires private recovery Redis: run scripts/test-session.py")
	}
	t.Setenv("OJ_SESSION_REDIS_ADDR", addr)
	f := setupAuth(t)
	ctx := context.Background()
	client := f.stores[0].client
	var old []*http.Cookie
	for i := range 3 {
		old = append(old, expect(t, f, i%2, "POST", "/login-fixture", nil, 201).Cookies()[0])
	}
	// A second account changes only its account generation after the snapshot;
	// fixture-user stays at account generation 1, isolating the global fence.
	a := f.stores[0].auth
	if _, err := a.pool.Exec(ctx, `INSERT INTO session_auth_fixture VALUES ($1,$2,1,true)`, a.namespace, "reset-user"); err != nil {
		t.Fatal(err)
	}
	resetDeadline := time.Now().Add(time.Hour)
	resetData, err := (scs.GobCodec{}).Encode(resetDeadline, map[string]interface{}{
		"subject": "reset-user", "auth_generation": int64(1), "recovery_generation": int64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.stores[0].CommitCtx(ctx, "reset-fixture", resetData, resetDeadline); err != nil {
		t.Fatal(err)
	}
	keys, err := client.Keys(ctx, f.stores[0].prefix+"*").Result()
	if err != nil || len(keys) != 6 {
		t.Fatalf("snapshot keys=%d err=%v", len(keys), err)
	}
	// Compare logical fields and sorted members; Redis may re-encode hashes on
	// RDB loading, so serialized DUMP bytes are not a semantic equality check.
	type record struct {
		fields  map[string]string
		slots   []redis.Z
		expires time.Duration
	}
	snapshot := make(map[string]record)
	for _, key := range keys {
		entry := record{}
		if strings.Contains(key, ":token:") {
			entry.fields, err = client.HGetAll(ctx, key).Result()
		} else {
			entry.slots, err = client.ZRangeWithScores(ctx, key, 0, -1).Result()
		}
		if err != nil {
			t.Fatal(err)
		}
		entry.expires, err = client.PExpireTime(ctx, key).Result()
		if err != nil || entry.expires <= 0 {
			t.Fatalf("snapshot expiry: %v", err)
		}
		snapshot[key] = entry
	}
	data, found, err := f.stores[0].FindCtx(ctx, old[0].Value)
	if err != nil || !found {
		t.Fatal("snapshot session missing")
	}
	before := redisRunID(t, client)
	if err := client.Save(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	expect(t, f, 1, "POST", "/logout", old[0], 204)
	rejectWithoutCookie(t, f, 0, "GET", "/me", old[0], 401)
	if client.HExists(ctx, f.stores[0].key(old[0].Value), "data").Val() {
		t.Fatal("logout did not remove live payload")
	}
	if _, err := a.pool.Exec(ctx, `UPDATE session_auth_fixture SET generation=generation+1
		WHERE namespace=$1 AND subject=$2`, a.namespace, "reset-user"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.stores[1].FindCtx(ctx, "reset-fixture"); !errors.Is(err, errStale) {
		t.Fatalf("account invalidation: %v", err)
	}
	for _, cookie := range old[1:] {
		expect(t, f, 1, "GET", "/me", cookie, 200)
	}
	generation := beginRecovery(t, f.stores[1].auth)
	assertRecoveryClosed(t, f, old[1])
	restoreRedisSnapshot(t)
	if redisRunID(t, client) == before {
		t.Fatal("Redis process did not restart")
	}
	// Prove the stale payloads AND the full three-slot index returned from disk.
	for key, want := range snapshot {
		if want.fields != nil {
			got, err := client.HGetAll(ctx, key).Result()
			if err != nil || !reflect.DeepEqual(got, want.fields) {
				t.Fatalf("snapshot session fields not restored: %v", err)
			}
		} else {
			got, err := client.ZRangeWithScores(ctx, key, 0, -1).Result()
			if err != nil || !reflect.DeepEqual(got, want.slots) {
				t.Fatalf("snapshot slot members/scores not restored: %v", err)
			}
		}
		expires, err := client.PExpireTime(ctx, key).Result()
		if err != nil || expires != want.expires {
			t.Fatalf("snapshot absolute expiry changed: %v", err)
		}
		ttl, err := client.PTTL(ctx, key).Result()
		if err != nil || ttl <= 0 {
			t.Fatalf("snapshot TTL lost: %v", err)
		}
	}
	assertRecoveryClosed(t, f, old[0])
	// New pools observe the persistent gate, not an API's memory.
	fresh := newAuthority(t, f.stores[0].auth.namespace)
	if _, err := fresh.current(ctx, "fixture-user"); !errors.Is(err, errRecovering) {
		t.Fatalf("fresh authority admitted during recovery: %v", err)
	}
	finishRecovery(t, fresh, generation)
	version, err := fresh.current(ctx, "fixture-user")
	if err != nil || version != (authVersion{1, 2}) {
		t.Fatalf("persisted versions=%+v err=%v", version, err)
	}
	if _, _, err := f.stores[1].FindCtx(ctx, "reset-fixture"); !errors.Is(err, errStale) {
		t.Fatalf("restored reset account session: %v", err)
	}
	for replica := range 2 {
		for _, cookie := range old {
			rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
			rejectWithoutCookie(t, f, replica, "POST", "/logout", cookie, 401)
		}
	}
	if err := f.stores[0].CommitCtx(ctx, old[0].Value, data, time.Now().Add(time.Hour)); !errors.Is(err, errStale) {
		t.Fatalf("old snapshot save: %v", err)
	}
	assertThreeFreshSlots(t, f)
}

func TestRecoveryRejectsLoadedSaveAndLogin(t *testing.T) {
	for _, path := range []string{"/late-save", "/late-login-fixture"} {
		t.Run(path, func(t *testing.T) {
			f := setupAuth(t)
			var cookie *http.Cookie
			if path == "/late-save" {
				cookie = expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			}
			finished := make(chan error, 1)
			go func() {
				r, err := request(f, 0, "POST", path, cookie)
				if err == nil && (r.StatusCode != 401 || len(r.Cookies()) != 0) {
					err = fmt.Errorf("late operation status=%d cookies=%d", r.StatusCode, len(r.Cookies()))
				}
				finished <- err
			}()
			select {
			case <-f.loaded:
			case <-time.After(5 * time.Second):
				close(f.resume)
				t.Fatal("load barrier")
			}
			generation := beginRecovery(t, f.stores[1].auth)
			finishRecovery(t, f.stores[1].auth, generation)
			close(f.resume)
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			assertThreeFreshSlots(t, f)
		})
	}
}

func TestRecoveryGateFencesStaleCompletion(t *testing.T) {
	f := setupAuth(t)
	ctx := context.Background()
	a := f.stores[1].auth
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	old := beginRecovery(t, a)
	current := beginRecovery(t, a)
	if err := a.finishRecovery(ctx, old); !errors.Is(err, errRecovering) {
		t.Fatalf("stale recovery reopened gate: %v", err)
	}
	assertRecoveryClosed(t, f, cookie)
	finishRecovery(t, a, current)
	if err := a.finishRecovery(ctx, current); !errors.Is(err, errRecovering) {
		t.Fatalf("duplicate completion succeeded: %v", err)
	}
	for replica := range 2 {
		rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
	}
	assertThreeFreshSlots(t, f)
	if _, err := a.pool.Exec(ctx, `DELETE FROM session_recovery_fixture WHERE namespace=$1`, a.namespace); err != nil {
		t.Fatal(err)
	}
	assertRecoveryClosed(t, f, cookie)
	if _, err := a.beginRecovery(ctx); err == nil {
		t.Fatal("missing recovery authority initialized itself")
	}
}

func TestRecoveryWaitsForSaveGuard(t *testing.T) {
	f := setupAuth(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := f.stores[0].auth.withValid(ctx, "fixture-user", authVersion{1, 1}, func() error {
		tx, err := f.stores[1].auth.pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='100ms'`); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE session_recovery_fixture SET generation=generation+1, enabled=false
			WHERE namespace=$1`, f.stores[1].auth.namespace)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
			return fmt.Errorf("expected blocked recovery, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	generation := beginRecovery(t, f.stores[1].auth)
	finishRecovery(t, f.stores[1].auth, generation)
	err = f.stores[0].auth.withValid(ctx, "fixture-user", authVersion{1, 1}, func() error {
		t.Error("old recovery generation entered save")
		return nil
	})
	if !errors.Is(err, errStale) {
		t.Fatalf("old recovery generation: %v", err)
	}
}
