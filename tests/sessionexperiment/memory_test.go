package sessionexperiment

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func redisStat(t *testing.T, client *redis.Client, section, field string) int64 {
	t.Helper()
	info, err := client.Info(context.Background(), section).Result()
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(info, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), field+":"); ok {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
	}
	t.Fatalf("missing Redis statistic %s", field)
	return 0
}

// Change only this driver's private Redis, serially. Lowering the logical
// budget reproduces server OOM rejection without exhausting container/host RAM.
func limitRedisMemory(t *testing.T, f *fixture) func() {
	t.Helper()
	client := f.stores[0].client
	ctx := context.Background()
	config, err := client.ConfigGet(ctx, "maxmemory*").Result()
	if err != nil || config["maxmemory"] == "" || config["maxmemory-policy"] != "noeviction" {
		t.Fatalf("requires disposable noeviction fixture: %v", err)
	}
	reset := func() error {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := client.ConfigSet(cleanup, "maxmemory", config["maxmemory"]).Err(); err != nil {
			return err
		}
		restored, err := client.ConfigGet(cleanup, "maxmemory").Result()
		if err == nil && restored["maxmemory"] != config["maxmemory"] {
			return errors.New("Redis memory limit not restored")
		}
		return err
	}
	// Registered after setup, so this runs before closing servers/clients.
	t.Cleanup(func() {
		if err := reset(); err != nil {
			t.Errorf("restore Redis memory limit: %v", err)
		}
	})
	if err := client.ConfigSet(ctx, "maxmemory", "1").Err(); err != nil {
		t.Fatal(err)
	}
	if redisStat(t, client, "memory", "used_memory") <= redisStat(t, client, "memory", "maxmemory") {
		t.Fatal("Redis did not exceed logical memory limit")
	}
	if err := client.Set(ctx, f.stores[0].prefix+"oom-probe", "probe", 0).Err(); !redis.HasErrorPrefix(err, "OOM") {
		t.Fatalf("expected real server OOM rejection, got %v", err)
	}
	evicted := redisStat(t, client, "stats", "evicted_keys")
	t.Cleanup(func() {
		if redisStat(t, client, "stats", "evicted_keys") != evicted {
			t.Error("memory pressure evicted keys")
		}
	})
	return func() {
		t.Helper()
		if err := reset(); err != nil {
			t.Fatal(err)
		}
	}
}

type memoryRecord struct {
	fields  map[string]string
	slots   []redis.Z
	expires time.Duration
}

func memorySnapshot(t *testing.T, f *fixture) map[string]memoryRecord {
	t.Helper()
	ctx := context.Background()
	client := f.stores[0].client
	keys, err := client.Keys(ctx, f.stores[0].prefix+"*").Result()
	if err != nil {
		t.Fatal(err)
	}
	state := make(map[string]memoryRecord)
	for _, key := range keys {
		entry := memoryRecord{}
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
			t.Fatalf("session key expiry missing: %v", err)
		}
		state[key] = entry
	}
	return state
}

func assertMemoryUnchanged(t *testing.T, f *fixture, before map[string]memoryRecord) {
	t.Helper()
	if !reflect.DeepEqual(memorySnapshot(t, f), before) {
		t.Fatal("failed write changed session payload, key set, index or absolute expiry")
	}
}

func TestRedisMemoryRejectsAdmissionAcrossAPIs(t *testing.T) {
	for _, occupied := range []int{0, 2, 3} {
		t.Run(fmt.Sprintf("occupied-%d", occupied), func(t *testing.T) {
			f := setupAuth(t)
			var cookies []*http.Cookie
			for i := range occupied {
				cookies = append(cookies, expect(t, f, i%2, "POST", "/login-fixture", nil, 201).Cookies()[0])
			}
			before := memorySnapshot(t, f)
			restore := limitRedisMemory(t, f)
			for replica := range 2 {
				start := time.Now()
				rejectWithoutCookie(t, f, replica, "POST", "/login-fixture", nil, 503)
				if time.Since(start) >= 4*time.Second {
					t.Fatal("OOM admission exceeded failure budget")
				}
				for _, cookie := range cookies {
					expect(t, f, replica, "GET", "/me", cookie, 200)
				}
			}
			assertMemoryUnchanged(t, f, before)
			restore()
			for i := occupied; i < 3; i++ {
				cookie := expect(t, f, i%2, "POST", "/login-fixture", nil, 201).Cookies()[0]
				for replica := range 2 {
					expect(t, f, replica, "GET", "/me", cookie, 200)
				}
			}
			for replica := range 2 {
				rejectWithoutCookie(t, f, replica, "POST", "/login-fixture", nil, 409)
			}
		})
	}
}

func TestRedisMemoryRejectsLoadedSave(t *testing.T) {
	f := setupAuth(t)
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	before := memorySnapshot(t, f)
	finished := make(chan error, 1)
	go func() {
		r, err := request(f, 0, "POST", "/late-save", cookie)
		if err == nil && (r.StatusCode != 503 || len(r.Cookies()) != 0) {
			err = fmt.Errorf("OOM save status=%d cookies=%d", r.StatusCode, len(r.Cookies()))
		}
		finished <- err
	}()
	select {
	case <-f.loaded:
	case <-time.After(5 * time.Second):
		close(f.resume)
		t.Fatal("load barrier")
	}
	restore := limitRedisMemory(t, f)
	close(f.resume)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("OOM save exceeded failure budget")
	}
	assertMemoryUnchanged(t, f, before)
	restore()
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 200)
		expect(t, f, replica, "POST", "/login-fixture", nil, 201)
	}
	rejectWithoutCookie(t, f, 0, "POST", "/login-fixture", nil, 409)
}

func TestRedisMemoryAllowsRevocation(t *testing.T) {
	f := setupAuth(t)
	cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
	before := memorySnapshot(t, f)
	ctx := context.Background()
	data, found, err := f.stores[0].FindCtx(ctx, cookie.Value)
	if err != nil || !found {
		t.Fatal("missing admitted session")
	}
	restore := limitRedisMemory(t, f)
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", cookie, 200)
	}
	cleared := expect(t, f, 1, "POST", "/logout", cookie, 204).Cookies()
	if len(cleared) != 1 || cleared[0].Value != "" || cleared[0].MaxAge >= 0 {
		t.Fatal("successful revocation did not clear browser cookie")
	}
	for replica := range 2 {
		rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
	}
	after := memorySnapshot(t, f)
	key := f.stores[0].key(cookie.Value)
	want := before[key]
	delete(want.fields, "data")
	if len(after) != 1 || !reflect.DeepEqual(after[key], want) {
		t.Fatal("OOM logout did not preserve expiring tombstone and release index")
	}
	restore()
	if err := f.stores[0].CommitCtx(ctx, cookie.Value, data, time.Now().Add(time.Hour)); !errors.Is(err, errStale) {
		t.Fatalf("revoked session revived after OOM: %v", err)
	}
	assertThreeFreshSlots(t, f)
	for replica := range 2 {
		rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
	}
}

func TestRedisMemoryDoesNotBypassAuthority(t *testing.T) {
	for _, kind := range []string{"account", "recovery"} {
		t.Run(kind, func(t *testing.T) {
			f := setupAuth(t)
			cookie := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			restore := limitRedisMemory(t, f)
			if kind == "account" {
				invalidate(t, f.stores[1].auth, true)
			} else {
				generation := beginRecovery(t, f.stores[1].auth)
				assertRecoveryClosed(t, f, cookie)
				finishRecovery(t, f.stores[1].auth, generation)
			}
			for replica := range 2 {
				rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
				rejectWithoutCookie(t, f, replica, "POST", "/login-fixture", nil, 503)
			}
			restore()
			assertThreeFreshSlots(t, f)
			for replica := range 2 {
				rejectWithoutCookie(t, f, replica, "GET", "/me", cookie, 401)
			}
		})
	}
}
