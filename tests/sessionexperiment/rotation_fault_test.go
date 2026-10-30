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

func TestLoadedRotationRejectsInvalidationAndExpiry(t *testing.T) {
	for _, profile := range []string{"personal", "remember", "admin"} {
		for _, fault := range []string{"logout", "account", "recovery", "gate", "idle", "absolute"} {
			t.Run(profile+"/"+fault, func(t *testing.T) {
				var f *fixture
				path := "/login-fixture"
				if fault == "idle" || fault == "absolute" {
					policy := rememberPolicy{time.Hour, time.Second}
					if fault == "absolute" {
						policy = rememberPolicy{time.Second, time.Hour}
					}
					f = setupAuthWithProfiles(t, policy, policy, shortLifetime(policy.lifetime, policy.idle))
					switch profile {
					case "admin":
						f = adminView(f)
					case "remember":
						path += "?remember=true"
					}
				} else {
					f, path = rotationProfile(t, profile)
				}
				old := expect(t, f, 0, "POST", path, nil, 201).Cookies()[0]
				initial := readLifetime(t, f, old)
				resume := func() { close(f.resume) }
				// The barrier must also be released on an assertion failure.
				defer func() {
					select {
					case <-f.resume:
					default:
						resume()
					}
				}()
				finished := make(chan poolRequestResult, 1)
				go func() { finished <- timedPoolRequest(f, 0, "POST", "/late-rotate-fixture", old) }()
				waitRotationLoad(t, f)
				status := 401
				switch fault {
				case "logout":
					expect(t, f, 1, "POST", "/logout", old, 204)
				case "account":
					invalidate(t, f.stores[1].auth, true)
				case "recovery":
					generation := beginRecovery(t, f.stores[1].auth)
					finishRecovery(t, f.stores[1].auth, generation)
				case "gate":
					beginRecovery(t, f.stores[1].auth)
					status = 503
				case "idle":
					awaitRedisTime(t, f, initial.expiry.Add(20*time.Millisecond))
				case "absolute":
					awaitRedisTime(t, f, initial.deadline.Add(20*time.Millisecond))
				}
				before := memorySnapshot(t, f)
				resume()
				rotationResponse(t, <-finished, status)
				assertMemoryUnchanged(t, f, before)
				if fault != "gate" {
					for replica := range 2 {
						rejectWithoutCookie(t, f, replica, "GET", "/me", old, 401)
					}
					assertThreeFreshSlots(t, f)
				}
			})
		}
	}
}

func TestRotationPoolFailureAndOOMLeaveOldSession(t *testing.T) {
	for _, profile := range []string{"personal", "admin"} {
		for _, fault := range []string{"pool", "oom"} {
			t.Run(profile+"/"+fault, func(t *testing.T) {
				f, path := rotationProfile(t, profile)
				old := expect(t, f, 0, "POST", path, nil, 201).Cookies()[0]
				resume := func() { close(f.resume) }
				defer func() {
					select {
					case <-f.resume:
					default:
						resume()
					}
				}()
				finished := make(chan poolRequestResult, 1)
				go func() { finished <- timedPoolRequest(f, 0, "POST", "/late-rotate-fixture", old) }()
				waitRotationLoad(t, f)
				observer := poolObserver(t, f)
				observed := observedFixture(f, observer)
				before := memorySnapshot(t, observed)
				var recover func()
				if fault == "pool" {
					recover = holdSessionPool(t, f.stores[0].client)
				} else {
					recover = limitRedisMemory(t, f)
				}
				resume()
				rotationResponse(t, <-finished, 503)
				assertMemoryUnchanged(t, observed, before)
				recover()
				for replica := range 2 {
					expect(t, f, replica, "GET", "/me", old, 200)
				}
				next := rotateCookie(t, f, 1, old)
				assertRotationTransfer(t, f, old, next, before)
			})
		}
	}
}

func preparedRotation(t *testing.T, f *fixture, old *http.Cookie) (*scs.SessionManager, context.Context, *rotationStore) {
	t.Helper()
	manager := *f.managers[0]
	store := &rotationStore{guardedStore: f.stores[0], old: old.Value}
	manager.Store = store
	ctx, err := manager.Load(context.Background(), old.Value)
	if err != nil {
		t.Fatal(err)
	}
	deadline := manager.Deadline(ctx)
	if err := manager.RenewToken(ctx); err != nil {
		t.Fatal(err)
	}
	manager.SetDeadline(ctx, deadline)
	return &manager, ctx, store
}

type rawRotationRecord struct {
	kind    string
	fields  map[string]string
	slots   []redis.Z
	value   string
	expires int64
}

// Malformed records intentionally fail the normal snapshot validator. These
// probes use only hour-lived private keys, so expiry cannot race the commands.
func rawRotationSnapshot(t *testing.T, f *fixture) map[string]rawRotationRecord {
	t.Helper()
	client := f.stores[0].client
	keys, err := client.Keys(context.Background(), f.stores[0].prefix+"*").Result()
	if err != nil {
		t.Fatal(err)
	}
	state := make(map[string]rawRotationRecord)
	for _, key := range keys {
		kind, err := client.Type(context.Background(), key).Result()
		if err != nil {
			t.Fatal(err)
		}
		record := rawRotationRecord{kind: kind}
		// DUMP is not a canonical hash encoding. Retain malformed values,
		// while comparing complete logical content instead of iteration order.
		switch kind {
		case "hash":
			record.fields, err = client.HGetAll(context.Background(), key).Result()
		case "zset":
			record.slots, err = client.ZRangeWithScores(context.Background(), key, 0, -1).Result()
		case "string":
			record.value, err = client.Get(context.Background(), key).Result()
		default:
			t.Fatal("unexpected malformed fixture key type")
		}
		if err != nil {
			t.Fatal(err)
		}
		expires, err := client.Do(context.Background(), "PEXPIRETIME", key).Int64()
		if err != nil {
			t.Fatal(err)
		}
		record.expires = expires
		state[key] = record
	}
	return state
}

func TestRotationPreconditionsRejectWithoutWrites(t *testing.T) {
	for _, fault := range []string{"index-type", "missing-index", "score", "index-ttl", "token-ttl", "old-expiry", "index-extreme", "source-data", "capacity", "collision", "payload", "deadline", "unstaged", "same-token", "duplicate"} {
		t.Run(fault, func(t *testing.T) {
			f := setupAuth(t)
			old := expect(t, f, 0, "POST", "/login-fixture", nil, 201).Cookies()[0]
			state := readLifetime(t, f, old)
			manager, ctx, store := preparedRotation(t, f, old)
			client := f.stores[0].client
			var err error
			switch fault {
			case "index-type":
				err = client.Del(ctx, state.slot).Err()
				if err == nil {
					err = client.Set(ctx, state.slot, "invalid", time.Hour).Err()
				}
			case "missing-index":
				err = client.Del(ctx, state.slot).Err()
			case "score":
				err = client.ZAdd(ctx, state.slot, redis.Z{Member: store.key(old.Value), Score: float64(state.expiry.UnixMilli() + 1)}).Err()
			case "index-ttl":
				err = client.PExpireAt(ctx, state.slot, state.expiry.Add(-time.Minute)).Err()
			case "token-ttl":
				err = client.Persist(ctx, store.key(old.Value)).Err()
			case "old-expiry":
				err = client.HSet(ctx, store.key(old.Value), "expiry", "+inf").Err()
			case "index-extreme":
				err = client.ZAdd(ctx, state.slot, redis.Z{Member: "extreme-fixture", Score: 9223372036854775808.0}).Err()
				if err == nil {
					err = client.Do(ctx, "PEXPIREAT", state.slot, "9223372036854775807").Err()
				}
			case "source-data":
				err = client.HSet(ctx, store.key(old.Value), "data", "changed-fixture").Err()
			case "capacity":
				err = client.ZAdd(ctx, state.slot,
					redis.Z{Member: "fixture-2", Score: float64(state.expiry.UnixMilli())},
					redis.Z{Member: "fixture-3", Score: float64(state.expiry.UnixMilli())},
					redis.Z{Member: "fixture-4", Score: float64(state.expiry.UnixMilli())}).Err()
			case "collision":
				err = client.Set(ctx, store.key(manager.Token(ctx)), "collision", time.Hour).Err()
			case "payload":
				manager.Put(ctx, "remember_login", true)
			case "deadline":
				manager.SetDeadline(ctx, manager.Deadline(ctx).Add(time.Hour))
			case "unstaged":
				store.staged = false
			case "same-token":
				store.old = manager.Token(ctx)
			case "duplicate":
				_, _, err = manager.Commit(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := rawRotationSnapshot(t, f)
			_, _, err = manager.Commit(ctx)
			if !errors.Is(err, errRotationConflict) {
				t.Fatalf("unsafe rotation not rejected: %v", err)
			}
			if !reflect.DeepEqual(rawRotationSnapshot(t, f), before) {
				t.Fatal("rejected rotation wrote partial state")
			}
		})
	}
}
