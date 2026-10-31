//go:build integration

package sessionprotocol_test

import (
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestOrdinaryProtocol(t *testing.T) {
	f := setup(t)
	t.Run("parallel admission has exactly three slots", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		stores := [2]*protocol.Store{f.store(t, ns, 0, protocol.OrdinaryPolicy()), f.store(t, ns, 1, protocol.OrdinaryPolicy())}
		type result struct {
			a   *protocol.Attempt
			err error
		}
		out := make(chan result, 12)
		start := make(chan struct{})
		for i := range 12 {
			go func() {
				<-start
				a, err := stores[i%2].Create(f.ctx, p.Email, []byte("synthetic protocol password"))
				out <- result{a, err}
			}()
		}
		close(start)
		var accepted []*protocol.Attempt
		var limited *protocol.Attempt
		for range 12 {
			r := <-out
			if r.a == nil {
				t.Fatal("authenticated admission omitted frozen attempt")
			}
			if r.err == nil {
				accepted = append(accepted, r.a)
			} else if errors.Is(r.err, protocol.ErrLimit) {
				limited = r.a
			} else {
				t.Fatal(r.err)
			}
		}
		if len(accepted) != 3 || limited == nil {
			t.Fatal("ordinary quota was not exactly three")
		}
		a := accepted[0]
		c, err := stores[1].Check(f.ctx, a)
		must(t, err)
		key := tokenKey(ns, c)
		before, err := f.control.HGetAll(f.ctx, key).Result()
		must(t, err)
		for range 3 {
			must(t, stores[1].Retry(f.ctx, a))
		}
		after, err := f.control.HGetAll(f.ctx, key).Result()
		must(t, err)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("idempotent retry changed frozen record")
		}
		v, err := stores[1].Read(f.ctx, c)
		must(t, err)
		if v.SubjectID != p.ID || v.AuthGeneration != 1 {
			t.Fatal("replica lost session ownership")
		}
		must(t, stores[1].Revoke(f.ctx, c))
		_, err = stores[0].Read(f.ctx, c)
		wantError(t, err, protocol.ErrStale)
		must(t, stores[1].Retry(f.ctx, limited))
		_, err = stores[0].Check(f.ctx, limited)
		must(t, err)
		wantError(t, stores[0].Retry(f.ctx, a), protocol.ErrStale)
	})
	t.Run("read is pure and touch cannot replay initial admission", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		other := f.store(t, ns, 1, protocol.OrdinaryPolicy())
		a, c := f.create(t, s, p)
		v, err := s.Read(f.ctx, c)
		must(t, err)
		f.waitTime(t, v.CreatedAt.Add(10*time.Millisecond))
		poll, err := other.Read(f.ctx, c)
		must(t, err)
		if poll != v {
			t.Fatal("read renewed or changed the session")
		}
		touched, err := other.Touch(f.ctx, c)
		must(t, err)
		if !touched.IdleExpiresAt.After(v.IdleExpiresAt) || touched.AbsoluteDeadline != v.AbsoluteDeadline || touched.CreatedAt != v.CreatedAt {
			t.Fatal("touch changed absolute identity or failed to renew idle")
		}
		key := tokenKey(ns, c)
		before, err := f.control.HGetAll(f.ctx, key).Result()
		must(t, err)
		wantError(t, s.Retry(f.ctx, a), protocol.ErrConflict)
		_, err = s.Check(f.ctx, a)
		wantError(t, err, protocol.ErrConflict)
		after, err := f.control.HGetAll(f.ctx, key).Result()
		must(t, err)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("retry rewound touched expiry")
		}
		_, err = s.Read(f.ctx, c)
		must(t, err)
		must(t, s.Abandon(f.ctx, a))
		_, err = other.Read(f.ctx, c)
		wantError(t, err, protocol.ErrStale)
		must(t, other.Abandon(f.ctx, a))
		wantError(t, other.Retry(f.ctx, a), protocol.ErrStale)
	})
	t.Run("idle and absolute deadlines reject delayed operations", func(t *testing.T) {
		p := f.subject(t, false)
		s := f.store(t, namespace(t), 0, protocol.Policy{Idle: time.Second, Absolute: 3 * time.Second})
		a, c := f.create(t, s, p)
		v, err := s.Read(f.ctx, c)
		must(t, err)
		f.waitTime(t, v.IdleExpiresAt)
		_, err = s.Touch(f.ctx, c)
		wantError(t, err, protocol.ErrStale)
		wantError(t, s.Retry(f.ctx, a), protocol.ErrStale)
		_, err = s.Check(f.ctx, a)
		wantError(t, err, protocol.ErrStale)
		must(t, s.Abandon(f.ctx, a))
		f.waitTime(t, v.AbsoluteDeadline)
		_, err = s.Read(f.ctx, c)
		wantError(t, err, protocol.ErrNotFound)
		wantError(t, s.Retry(f.ctx, a), protocol.ErrStale)
		_, err = s.Check(f.ctx, a)
		wantError(t, err, protocol.ErrStale)
	})
	t.Run("active renewals stop at original absolute deadline", func(t *testing.T) {
		p := f.subject(t, false)
		s := f.store(t, namespace(t), 0, protocol.Policy{Idle: time.Second, Absolute: 3 * time.Second})
		_, c := f.create(t, s, p)
		v, err := s.Read(f.ctx, c)
		must(t, err)
		absolute := v.AbsoluteDeadline
		for i := range 4 {
			f.waitTime(t, v.CreatedAt.Add(time.Duration(i+1)*600*time.Millisecond))
			v, err = s.Touch(f.ctx, c)
			must(t, err)
			if v.AbsoluteDeadline != absolute || v.IdleExpiresAt.After(absolute) {
				t.Fatal("touch exceeded absolute deadline")
			}
		}
		if v.IdleExpiresAt != absolute {
			t.Fatal("late active expiry was not clamped to original deadline")
		}
		f.waitTime(t, absolute)
		_, err = s.Touch(f.ctx, c)
		wantError(t, err, protocol.ErrNotFound)
	})
	t.Run("complete receipts reject partial or corrupt records", func(t *testing.T) { testCorruptReceipts(t, f) })
	t.Run("authority revisions and recovery", func(t *testing.T) { testAuthority(t, f) })
	t.Run("unknown outcomes and delayed writes", func(t *testing.T) { testUnknown(t, f) })
	t.Run("real locks and lost backend", func(t *testing.T) { testLocks(t, f) })
	t.Run("OOM distinguishes readonly checks and writes", func(t *testing.T) { testOOM(t, f) })
	t.Run("actual Redis pause retains uncertain attempt", func(t *testing.T) { testRedisPause(t, f) })
	for _, c := range f.clients {
		if c.PoolStats().TotalConns > 8 {
			t.Fatal("Redis client exceeded explicit connection ceiling")
		}
	}
}

func testCorruptReceipts(t *testing.T, f *fixture) {
	for _, change := range []string{"version", "owner", "credential", "auth", "recovery", "oversized", "missing field", "extra field", "token ttl", "missing slot", "index ttl", "index type", "excess members"} {
		t.Run(change, func(t *testing.T) {
			ns := namespace(t)
			p := f.subject(t, false)
			s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
			a, c := f.create(t, s, p)
			key := tokenKey(ns, c)
			groups, err := f.control.Keys(f.ctx, "oj-session:"+ns+":subject:*").Result()
			must(t, err)
			if len(groups) != 1 {
				t.Fatal("fixture did not have one slot group")
			}
			group := groups[0]
			switch change {
			case "version":
				err = f.control.HSet(f.ctx, key, "v", "2").Err()
			case "owner":
				err = f.control.HSet(f.ctx, key, "subject", "000000000000000000000000000000ff").Err()
			case "credential":
				err = f.control.HSet(f.ctx, key, "credential", "2").Err()
			case "auth":
				err = f.control.HSet(f.ctx, key, "auth", "2").Err()
			case "recovery":
				err = f.control.HSet(f.ctx, key, "recovery", "000000000000000000000000000000ff").Err()
			case "oversized":
				err = f.control.HSet(f.ctx, key, "subject", string(make([]byte, 1024))).Err()
			case "missing field":
				err = f.control.HDel(f.ctx, key, "initial").Err()
			case "extra field":
				err = f.control.HSet(f.ctx, key, "unexpected", "value").Err()
			case "token ttl":
				err = f.control.Persist(f.ctx, key).Err()
			case "missing slot":
				err = f.control.ZRem(f.ctx, group, key).Err()
			case "index ttl":
				now, e := f.control.Time(f.ctx).Result()
				must(t, e)
				err = f.control.PExpireAt(f.ctx, group, now.Add(time.Hour)).Err()
			case "index type":
				must(t, f.control.Del(f.ctx, group).Err())
				err = f.control.Set(f.ctx, group, "wrong type", 0).Err()
			case "excess members":
				expiry, e := f.control.HGet(f.ctx, key, "idle").Int64()
				must(t, e)
				for i := range 3 {
					err = f.control.ZAdd(f.ctx, group, redisMember(strconv.Itoa(i), expiry)).Err()
					must(t, err)
				}
			}
			must(t, err)
			credential, err := s.Check(f.ctx, a)
			wantError(t, err, protocol.ErrConflict)
			if credential.Value() != "" {
				t.Fatal("corrupt receipt returned credential")
			}
			v, err := s.Read(f.ctx, c)
			if err == nil || v != (protocol.View{}) {
				t.Fatal("partial/corrupt Redis record became an authenticated view")
			}
		})
	}
}

func testAuthority(t *testing.T, f *fixture) {
	t.Run("rehash preserves existing session but not initial attempt", func(t *testing.T) {
		p := f.subject(t, true)
		ns := namespace(t)
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		a, c := f.create(t, s, p)
		state, err := f.identities[1].State(f.ctx, p.ID)
		must(t, err)
		if state.CredentialVersion != 2 {
			t.Fatal("Create did not conditionally upgrade legacy credentials")
		}
		r, err := f.hasher.Create([]byte("synthetic protocol password"))
		must(t, err)
		_, err = f.identities[1].RehashCAS(f.ctx, p.ID, state.Revision, r)
		must(t, err)
		_, err = s.Read(f.ctx, c)
		must(t, err)
		_, err = s.Touch(f.ctx, c)
		must(t, err)
		_, err = s.Check(f.ctx, a)
		wantError(t, err, identity.ErrConflict)
		wantError(t, s.Retry(f.ctx, a), identity.ErrConflict)
		must(t, s.Abandon(f.ctx, a))
		_, err = s.Read(f.ctx, c)
		wantError(t, err, protocol.ErrStale)
	})
	for _, change := range []string{"replace", "disable", "recovery"} {
		t.Run(change, func(t *testing.T) {
			p := f.subject(t, false)
			ns := namespace(t)
			s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
			a, c := f.create(t, s, p)
			switch change {
			case "replace":
				r, err := f.hasher.Create([]byte("synthetic changed password"))
				must(t, err)
				_, err = f.identities[1].ReplacePasswordCAS(f.ctx, p.ID, p.Revision, r)
				must(t, err)
			case "disable":
				_, err := f.identities[1].DisableCAS(f.ctx, p.ID, p.Revision)
				must(t, err)
			case "recovery":
				f.open(t)
			}
			_, err := s.Read(f.ctx, c)
			wantError(t, err, identity.ErrConflict)
			wantError(t, s.Retry(f.ctx, a), identity.ErrConflict)
			_, err = s.Check(f.ctx, a)
			wantError(t, err, identity.ErrConflict)
			if change != "disable" {
				for range 3 {
					secret := "synthetic protocol password"
					if change == "replace" {
						secret = "synthetic changed password"
					}
					fresh, err := s.Create(f.ctx, p.Email, []byte(secret))
					must(t, err)
					_, err = s.Check(f.ctx, fresh)
					must(t, err)
				}
			}
		})
	}
	t.Run("int64 versions are exact beyond Lua integer precision", func(t *testing.T) {
		p := f.subject(t, false)
		ns := namespace(t)
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_credentials SET credential_version=9223372036854775807,auth_generation=9223372036854775807 WHERE id=$1`, p.ID)
		must(t, err)
		_, c := f.create(t, s, p)
		v, err := s.Read(f.ctx, c)
		must(t, err)
		if v.AuthGeneration != int64(9223372036854775807) {
			t.Fatal("int64 version lost precision")
		}
		_, err = f.pools[0].Exec(f.ctx, `UPDATE identity_credentials SET auth_generation=9223372036854775806 WHERE id=$1`, p.ID)
		must(t, err)
		_, err = s.Read(f.ctx, c)
		wantError(t, err, identity.ErrConflict)
	})
}
