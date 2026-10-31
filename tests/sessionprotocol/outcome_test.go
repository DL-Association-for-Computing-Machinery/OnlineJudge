//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testUnknown(t *testing.T, f *fixture) {
	t.Run("TIME failure omits attempt without undoing committed rehash", func(t *testing.T) {
		p := f.subject(t, true)
		ns := namespace(t)
		s := f.faultStore(t, ns, &hook{op: "time", before: func(context.Context, redis.Cmder) error {
			return errors.New("synthetic Redis clock unavailable")
		}}, time.Second)
		a, err := s.Create(f.ctx, p.Email, []byte("synthetic protocol password"))
		wantError(t, err, protocol.ErrStore)
		wantError(t, err, identity.ErrGuardedOperation)
		if a != nil {
			t.Fatal("clock failure froze an attempt")
		}
		var credential, auth int64
		must(t, f.pools[1].QueryRow(f.ctx, `SELECT credential_version,auth_generation FROM identity_credentials WHERE id=$1`, p.ID).Scan(&credential, &auth))
		if credential != 2 || auth != 1 {
			t.Fatal("committed legacy upgrade was not retained")
		}
		keys, err := f.control.Keys(f.ctx, "oj-session:"+ns+":*").Result()
		must(t, err)
		if len(keys) != 0 {
			t.Fatal("clock failure wrote Redis")
		}
	})
	t.Run("successful Redis create retains attempt after authority commit fails", func(t *testing.T) {
		p := f.subject(t, false)
		s := f.faultStore(t, namespace(t), &hook{op: "retry", after: func(context.Context, redis.Cmder) error {
			b := oneBackend(t, f.ctx, f.pools[1], 0)
			var killed bool
			must(t, f.pools[1].QueryRow(f.ctx, `SELECT pg_terminate_backend($1)`, b.pid).Scan(&killed))
			if !killed {
				t.Fatal("create authority backend not terminated")
			}
			gone(t, f.ctx, f.pools[1], b)
			return nil
		}}, time.Second)
		a, err := s.Create(f.ctx, p.Email, []byte("synthetic protocol password"))
		wantError(t, err, identity.ErrStore)
		wantError(t, err, identity.ErrGuardedOperation)
		if a == nil {
			t.Fatal("commit failure lost frozen create attempt")
		}
		c, err := s.Check(f.ctx, a)
		must(t, err)
		_, err = s.Read(f.ctx, c)
		must(t, err)
		must(t, s.Abandon(f.ctx, a))
		wantError(t, s.Retry(f.ctx, a), protocol.ErrStale)
	})
	t.Run("runtime error after record write cannot confirm partial receipt", func(t *testing.T) {
		p := f.subject(t, false)
		ns := namespace(t)
		s := f.faultStore(t, ns, &hook{op: "retry", before: func(_ context.Context, cmd redis.Cmder) error {
			args := cmd.Args()
			script := args[1].(string)
			needle := "\nput(c)\nredis.call('ZADD'"
			if strings.Count(script, needle) != 1 {
				t.Fatal("partial-write injection point changed")
			}
			args[1] = strings.Replace(script, needle, "\nput(c)\nerror('synthetic partial write')\nredis.call('ZADD'", 1)
			return nil
		}}, time.Second)
		a, err := s.Create(f.ctx, p.Email, []byte("synthetic protocol password"))
		wantError(t, err, protocol.ErrStore)
		wantError(t, err, identity.ErrGuardedOperation)
		if a == nil {
			t.Fatal("partial write lost frozen attempt")
		}
		keys, err := f.control.Keys(f.ctx, "oj-session:"+ns+":token:*").Result()
		must(t, err)
		if len(keys) != 1 {
			t.Fatal("injected script did not leave one token record")
		}
		fields, err := f.control.HLen(f.ctx, keys[0]).Result()
		must(t, err)
		if fields != 12 {
			t.Fatal("injected error did not follow complete record write")
		}
		c, err := s.Check(f.ctx, a)
		wantError(t, err, protocol.ErrConflict)
		if c.Value() != "" {
			t.Fatal("partial receipt issued credential")
		}
		must(t, s.Abandon(f.ctx, a))
		wantError(t, s.Retry(f.ctx, a), protocol.ErrStale)
	})
	for _, written := range []bool{false, true} {
		t.Run(map[bool]string{false: "not written", true: "written lost receipt"}[written], func(t *testing.T) {
			ns := namespace(t)
			p := f.subject(t, false)
			var delayed []any
			h := &hook{op: "retry"}
			unknown := errors.New("synthetic acknowledgement unavailable")
			if written {
				h.after = func(context.Context, redis.Cmder) error { return unknown }
			} else {
				h.before = func(_ context.Context, cmd redis.Cmder) error {
					delayed = append([]any(nil), cmd.Args()...)
					return unknown
				}
			}
			s := f.faultStore(t, ns, h, time.Second)
			a, err := s.Create(f.ctx, p.Email, []byte("synthetic protocol password"))
			if a == nil || !errors.Is(err, protocol.ErrStore) || !errors.Is(err, identity.ErrGuardedOperation) {
				t.Fatal("uncertain write lost frozen attempt/classification")
			}
			c, err := s.Check(f.ctx, a)
			if written {
				must(t, err)
				_, err = s.Read(f.ctx, c)
				must(t, err)
			} else {
				wantError(t, err, protocol.ErrUnknown)
				if c.Value() != "" {
					t.Fatal("missing receipt issued credential")
				}
			}
			must(t, s.Abandon(f.ctx, a))
			if !written {
				values, err := f.control.Do(f.ctx, delayed...).Slice()
				must(t, err)
				if len(values) != 1 || values[0] != int64(2) {
					t.Fatal("late original wire save crossed abandonment tombstone")
				}
			}
			_, err = s.Check(f.ctx, a)
			wantError(t, err, protocol.ErrStale)
			wantError(t, s.Retry(f.ctx, a), protocol.ErrStale)
		})
	}
	t.Run("unknown unwritten attempt can be retried identically", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		s := f.faultStore(t, ns, &hook{op: "retry", before: func(context.Context, redis.Cmder) error { return errors.New("synthetic unwritten acknowledgement") }}, time.Second)
		a, err := s.Create(f.ctx, p.Email, []byte("synthetic protocol password"))
		wantError(t, err, protocol.ErrStore)
		if a == nil {
			t.Fatal("unwritten frozen attempt omitted")
		}
		_, err = s.Check(f.ctx, a)
		wantError(t, err, protocol.ErrUnknown)
		must(t, s.Retry(f.ctx, a))
		c, err := s.Check(f.ctx, a)
		must(t, err)
		_, err = s.Read(f.ctx, c)
		must(t, err)
	})
	t.Run("authentication failure never returns a usable attempt", func(t *testing.T) {
		p := f.subject(t, false)
		s := f.store(t, namespace(t), 0, protocol.OrdinaryPolicy())
		a, err := s.Create(f.ctx, p.Email, []byte("wrong password"))
		wantError(t, err, identity.ErrCredentials)
		if a != nil {
			t.Fatal("failed credentials returned attempt")
		}
		_, err = f.identities[0].BeginRecovery(f.ctx)
		must(t, err)
		a, err = s.Create(f.ctx, p.Email, []byte("synthetic protocol password"))
		wantError(t, err, identity.ErrRecovering)
		if a != nil {
			t.Fatal("closed recovery froze attempt")
		}
		f.open(t)
	})
}

func testOOM(t *testing.T, f *fixture) {
	ns := namespace(t)
	p := f.subject(t, false)
	s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
	a, c := f.create(t, s, p)
	key := tokenKey(ns, c)
	before, err := f.control.HGetAll(f.ctx, key).Result()
	must(t, err)
	old, err := f.control.ConfigGet(f.ctx, "maxmemory").Result()
	must(t, err)
	defer func() { must(t, f.control.ConfigSet(f.ctx, "maxmemory", old["maxmemory"]).Err()) }()
	must(t, f.control.ConfigSet(f.ctx, "maxmemory", "1").Err())
	_, err = s.Check(f.ctx, a)
	must(t, err)
	_, err = s.Read(f.ctx, c)
	must(t, err)
	err = s.Abandon(f.ctx, a)
	wantError(t, err, protocol.ErrStore)
	wantError(t, err, identity.ErrGuardedOperation)
	err = s.Retry(f.ctx, a)
	wantError(t, err, protocol.ErrStore)
	after, err := f.control.HGetAll(f.ctx, key).Result()
	must(t, err)
	if len(before) != len(after) {
		t.Fatal("already-OOM write mutated record")
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatal("declared write bypassed entry OOM check")
		}
	}
	must(t, f.control.ConfigSet(f.ctx, "maxmemory", old["maxmemory"]).Err())
	must(t, s.Abandon(f.ctx, a))
}

func testRedisPause(t *testing.T, f *fixture) {
	ns := namespace(t)
	p := f.subject(t, false)
	h := &hook{op: "retry", before: func(ctx context.Context, _ redis.Cmder) error {
		return f.control.Do(ctx, "CLIENT", "PAUSE", 1200, "ALL").Err()
	}}
	s := f.faultStore(t, ns, h, 100*time.Millisecond)
	started := time.Now()
	a, err := s.Create(f.ctx, p.Email, []byte("synthetic protocol password"))
	if a == nil || !errors.Is(err, protocol.ErrStore) || !errors.Is(err, identity.ErrGuardedOperation) || time.Since(started) > 3*time.Second {
		t.Fatal("paused write was not bounded/uncertain with a frozen attempt")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		err = f.control.Ping(f.ctx).Err()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Redis did not unpause")
		}
	}
	c, err := s.Check(f.ctx, a)
	// A real transport timeout deliberately cannot tell whether Redis accepted
	// the queued command. Both observations must preserve the same frozen attempt.
	if err == nil {
		_, err = s.Read(f.ctx, c)
		must(t, err)
	} else if !errors.Is(err, protocol.ErrUnknown) {
		t.Fatal(err)
	}
	must(t, s.Abandon(f.ctx, a))
	wantError(t, s.Retry(f.ctx, a), protocol.ErrStale)
	_, err = s.Check(f.ctx, a)
	wantError(t, err, protocol.ErrStale)
}
