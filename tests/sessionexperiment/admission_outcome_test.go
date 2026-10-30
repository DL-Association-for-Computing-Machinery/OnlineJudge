package sessionexperiment

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/redis/go-redis/v9"
)

func uncertainLogin(t *testing.T, write bool, configure ...sessionConfigurer) (*fixture, saveAttempt) {
	t.Helper()
	pending := make(chan saveAttempt, 1)
	configured := false
	f := setupAuth(t, append(configure, func(manager *scs.SessionManager, store *guardedStore) {
		if !configured {
			manager.Store = &uncertainStore{guardedStore: store, write: write, pending: pending}
			configured = true
		}
	})...)
	rejectWithoutCookie(t, f, 0, "POST", "/login-fixture", nil, 503)
	select {
	case attempt := <-pending:
		return f, attempt
	case <-time.After(3 * time.Second):
		t.Fatal("save candidate not captured")
		return nil, saveAttempt{}
	}
}

func attemptCookie(f *fixture, a saveAttempt) *http.Cookie {
	// Privileged test probe using SCS's actual generated candidate; no HTTP
	// success response or browser Cookie was issued for the uncertain login.
	return &http.Cookie{Name: f.managers[0].Cookie.Name, Value: a.token}
}

func admittedIndex(t *testing.T, f *fixture, a saveAttempt) string {
	t.Helper()
	hash, err := f.stores[0].client.HGet(context.Background(), f.stores[0].key(a.token), "subject").Result()
	if err != nil {
		t.Fatal(err)
	}
	return f.stores[0].prefix + "subject:" + hash
}

func TestAdmissionLostAcknowledgement(t *testing.T) {
	f, a := uncertainLogin(t, true)
	ctx := context.Background()
	before := memorySnapshot(t, f)
	if len(before) != 2 {
		t.Fatal("unknown login did not leave one token and one slot index")
	}
	if err := a.check(ctx, f.stores[1]); err != nil {
		t.Fatal(err)
	}
	retried := make(chan error, 10)
	for i := range 10 {
		go func() { retried <- a.retry(ctx, f.stores[i%2]) }()
	}
	for range 10 {
		if err := <-retried; err != nil {
			t.Fatal(err)
		}
	}
	assertMemoryUnchanged(t, f, before)
	for replica := range 2 {
		expect(t, f, replica, "GET", "/me", attemptCookie(f, a), 200)
	}
	for range 2 {
		if err := a.abandon(ctx, f.stores[1]); err != nil {
			t.Fatal(err)
		}
	}
	key := f.stores[0].key(a.token)
	want := before[key]
	delete(want.fields, "data")
	after := memorySnapshot(t, f)
	if len(after) != 1 || !reflect.DeepEqual(after[key], want) {
		t.Fatal("abandonment did not preserve original tombstone fields/absolute expiry or release slot")
	}
	if err := a.check(ctx, f.stores[0]); !errors.Is(err, errStale) {
		t.Fatalf("abandoned receipt: %v", err)
	}
	if err := a.retry(ctx, f.stores[0]); !errors.Is(err, errStale) {
		t.Fatalf("abandoned retry: %v", err)
	}
	for replica := range 2 {
		rejectWithoutCookie(t, f, replica, "GET", "/me", attemptCookie(f, a), 401)
	}
	assertThreeFreshSlots(t, f)
}

func TestAdmissionAbandonBeforeDelayedSave(t *testing.T) {
	f, a := uncertainLogin(t, false)
	ctx := context.Background()
	for replica := range 2 {
		if err := a.check(ctx, f.stores[replica]); !errors.Is(err, errOutcomeUnknown) {
			t.Fatalf("missing receipt is not a failed save: %v", err)
		}
	}
	if err := a.abandon(ctx, f.stores[1]); err != nil {
		t.Fatal(err)
	}
	before := memorySnapshot(t, f)
	if len(before) != 1 || before[f.stores[0].key(a.token)].fields["data"] != "" {
		t.Fatal("absent save was not fenced by an expiring tombstone")
	}
	deadline, _, err := (scs.GobCodec{}).Decode(a.data)
	if err != nil || before[f.stores[0].key(a.token)].expires.Milliseconds() != deadline.UnixMilli() {
		t.Fatal("absent-save tombstone did not retain original absolute expiry")
	}
	if err := a.retry(ctx, f.stores[0]); !errors.Is(err, errStale) {
		t.Fatalf("delayed original save bypassed abandonment: %v", err)
	}
	assertMemoryUnchanged(t, f, before)
	if err := a.check(ctx, f.stores[0]); !errors.Is(err, errStale) {
		t.Fatalf("abandoned receipt: %v", err)
	}
	assertThreeFreshSlots(t, f)
}

func TestAdmissionCheckRequiresCompleteRecord(t *testing.T) {
	for _, fault := range []string{"missing-slot", "wrong-score", "token-expiry", "index-expiry", "payload", "owner", "deadline", "over-limit"} {
		t.Run(fault, func(t *testing.T) {
			f, a := uncertainLogin(t, true)
			ctx := context.Background()
			client := f.stores[0].client
			key, index := f.stores[0].key(a.token), admittedIndex(t, f, a)
			deadline, _, err := (scs.GobCodec{}).Decode(a.data)
			if err != nil {
				t.Fatal(err)
			}
			switch fault {
			case "missing-slot":
				err = client.ZRem(ctx, index, key).Err()
			case "wrong-score":
				err = client.ZAdd(ctx, index, redis.Z{Member: key, Score: float64(a.expiry.UnixMilli() - 1)}).Err()
			case "token-expiry":
				err = client.PExpireAt(ctx, key, deadline.Add(time.Minute)).Err()
			case "index-expiry":
				err = client.PExpireAt(ctx, index, a.expiry.Add(-time.Minute)).Err()
			case "payload":
				err = client.HSet(ctx, key, "data", "different-fixture-payload").Err()
			case "owner":
				err = client.HSet(ctx, key, "subject", digest("another-fixture-owner")).Err()
			case "deadline":
				err = client.HSet(ctx, key, "deadline", deadline.Add(time.Minute).UnixMilli()).Err()
			case "over-limit":
				err = client.ZAdd(ctx, index,
					redis.Z{Member: "extra-fixture-1", Score: float64(a.expiry.UnixMilli())},
					redis.Z{Member: "extra-fixture-2", Score: float64(a.expiry.UnixMilli())},
					redis.Z{Member: "extra-fixture-3", Score: float64(a.expiry.UnixMilli())}).Err()
			}
			if err != nil {
				t.Fatal(err)
			}
			before := memorySnapshot(t, f)
			if err := a.check(ctx, f.stores[1]); !errors.Is(err, errAdmissionConflict) {
				t.Fatalf("incomplete record confirmed: %v", err)
			}
			if fault == "owner" || fault == "deadline" {
				if err := a.abandon(ctx, f.stores[1]); !errors.Is(err, errAdmissionConflict) {
					t.Fatalf("abandoned another candidate's record: %v", err)
				}
			}
			assertMemoryUnchanged(t, f, before)
		})
	}
}

func TestAdmissionCheckAllowsLongerIndexTTL(t *testing.T) {
	f, a := uncertainLogin(t, true)
	ctx := context.Background()
	index := admittedIndex(t, f, a)
	later := newSaveAttempt("later-expiry-fixture", a.data, a.expiry.Add(time.Minute))
	if err := later.retry(ctx, f.stores[1]); err != nil {
		t.Fatal(err)
	}
	if err := f.stores[1].DeleteCtx(ctx, later.token); err != nil {
		t.Fatal(err)
	}
	expiry, err := f.stores[0].client.PExpireTime(ctx, index).Result()
	if err != nil || expiry.Milliseconds() <= a.expiry.UnixMilli() {
		t.Fatal("fixture did not retain a valid longer index lifetime")
	}
	if err := a.check(ctx, f.stores[1]); err != nil {
		t.Fatalf("valid receipt rejected after another session's logout: %v", err)
	}
	now, err := f.stores[0].client.Time(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.stores[0].client.ZAdd(ctx, index,
		redis.Z{Member: "expired-fixture-1", Score: float64(now.UnixMilli())},
		redis.Z{Member: "expired-fixture-2", Score: float64(now.UnixMilli() - 1)},
		redis.Z{Member: "expired-fixture-3", Score: float64(now.UnixMilli() - 2)}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := a.check(ctx, f.stores[1]); err != nil {
		t.Fatalf("expired index members counted as active admissions: %v", err)
	}
}

func TestAdmissionAuthorityRejectsOldAttempt(t *testing.T) {
	for _, change := range []string{"logout", "account", "recovery"} {
		t.Run(change, func(t *testing.T) {
			f, a := uncertainLogin(t, true)
			ctx := context.Background()
			switch change {
			case "logout":
				expect(t, f, 1, "POST", "/logout", attemptCookie(f, a), 204)
			case "account":
				invalidate(t, f.stores[1].auth, true)
			case "recovery":
				generation := beginRecovery(t, f.stores[1].auth)
				if err := a.check(ctx, f.stores[0]); !errors.Is(err, errRecovering) {
					t.Fatalf("closed recovery gate confirmed receipt: %v", err)
				}
				finishRecovery(t, f.stores[1].auth, generation)
			}
			for replica := range 2 {
				if err := a.check(ctx, f.stores[replica]); !errors.Is(err, errStale) {
					t.Fatalf("old receipt: %v", err)
				}
				if err := a.retry(ctx, f.stores[replica]); !errors.Is(err, errStale) {
					t.Fatalf("old attempt replay: %v", err)
				}
				rejectWithoutCookie(t, f, replica, "GET", "/me", attemptCookie(f, a), 401)
			}
			assertThreeFreshSlots(t, f)
		})
	}
}

func TestAdmissionReadAndAbandonFailures(t *testing.T) {
	for _, write := range []bool{false, true} {
		name := "not-applied"
		if write {
			name = "applied"
		}
		t.Run(name, func(t *testing.T) {
			f, a := uncertainLogin(t, write)
			ctx := context.Background()
			before := memorySnapshot(t, f)
			restore := limitRedisMemory(t, f)
			err := a.check(ctx, f.stores[0])
			if (write && err != nil) || (!write && !errors.Is(err, errOutcomeUnknown)) {
				t.Fatalf("read-only receipt during OOM: %v", err)
			}
			if err := a.abandon(ctx, f.stores[0]); !redis.HasErrorPrefix(err, "OOM") {
				t.Fatalf("OOM abandonment falsely acknowledged: %v", err)
			}
			assertMemoryUnchanged(t, f, before)
			if err := f.stores[1].client.Close(); err != nil {
				t.Fatal(err)
			}
			for _, err := range []error{a.check(ctx, f.stores[1]), a.abandon(ctx, f.stores[1])} {
				if err == nil || errors.Is(err, errStale) || errors.Is(err, errOutcomeUnknown) {
					t.Fatalf("storage failure mistaken for a receipt/abandonment: %v", err)
				}
			}
			restore()
			if err := a.abandon(ctx, f.stores[0]); err != nil {
				t.Fatal(err)
			}
			if err := a.retry(ctx, f.stores[0]); !errors.Is(err, errStale) {
				t.Fatalf("late save after recovered abandonment: %v", err)
			}
		})
	}
}

func TestAdmissionExpiredAttemptCannotRestart(t *testing.T) {
	f, a := uncertainLogin(t, false, func(manager *scs.SessionManager, _ *guardedStore) {
		manager.Lifetime = 250 * time.Millisecond
		manager.IdleTimeout = 0
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		now, err := f.stores[0].client.Time(ctx).Result()
		if err != nil {
			t.Fatal(err)
		}
		if !now.Before(a.expiry) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, err := range []error{a.check(ctx, f.stores[1]), a.retry(ctx, f.stores[1]), a.abandon(ctx, f.stores[1])} {
		if !errors.Is(err, errStale) {
			t.Fatalf("expired original candidate accepted: %v", err)
		}
	}
	if len(memorySnapshot(t, f)) != 0 {
		t.Fatal("expired attempt created session metadata")
	}
}
