//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type backend struct {
	pid     int32
	started time.Time
}

func oneBackend(t *testing.T, ctx context.Context, pool *pgxpool.Pool, blocker int32) backend {
	t.Helper()
	call, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		rows, err := pool.Query(call, `SELECT pid,backend_start FROM pg_stat_activity WHERE datname=current_database() AND
            (($1::int=0 AND state='idle in transaction' AND query LIKE '%identity_credentials%' AND query LIKE '%FOR SHARE%') OR
             ($1::int<>0 AND wait_event_type='Lock' AND $1=ANY(pg_blocking_pids(pid))))`, blocker)
		must(t, err)
		var all []backend
		for rows.Next() {
			var b backend
			must(t, rows.Scan(&b.pid, &b.started))
			all = append(all, b)
		}
		must(t, rows.Err())
		rows.Close()
		if len(all) == 1 {
			return all[0]
		}
		if len(all) > 1 {
			t.Fatal("authority backend was not unique")
		}
		select {
		case <-tick.C:
		case <-call.Done():
			t.Fatal("authority backend not observed")
		}
	}
}
func gone(t *testing.T, ctx context.Context, pool *pgxpool.Pool, b backend) {
	t.Helper()
	call, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var exists bool
		must(t, pool.QueryRow(call, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND backend_start=$2)`, b.pid, b.started).Scan(&exists))
		if !exists {
			return
		}
		select {
		case <-tick.C:
		case <-call.Done():
			t.Fatal("exact canceled backend did not leave")
		}
	}
}

type windowAuthority struct {
	*identitypg.Store
	before func()
}

func (s *windowAuthority) WithSession(ctx context.Context, id string, v identity.SessionVersion, op func(context.Context, identity.SessionState) error) error {
	if s.before != nil {
		s.before()
	}
	return s.Store.WithSession(ctx, id, v, op)
}

func testLocks(t *testing.T, f *fixture) {
	t.Run("cancel real account lock waiter before any Redis save", func(t *testing.T) {
		p := f.subject(t, false)
		ns := namespace(t)
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		held, err := f.pools[0].Begin(f.ctx)
		must(t, err)
		defer func() {
			if err := held.Rollback(f.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Error(err)
			}
		}()
		var pid int32
		must(t, held.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&pid))
		_, err = held.Exec(f.ctx, `SELECT id FROM identity_credentials WHERE id=$1 FOR UPDATE`, p.ID)
		must(t, err)
		call, cancel := context.WithCancel(f.ctx)
		defer cancel()
		out := make(chan error, 1)
		go func() {
			a, err := s.Create(call, p.Email, []byte("synthetic protocol password"))
			if a != nil {
				out <- errors.New("waiter froze an attempt before entering authority")
				return
			}
			out <- err
		}()
		b := oneBackend(t, f.ctx, f.pools[1], pid)
		keys, err := f.control.Keys(f.ctx, "oj-session:"+ns+":*").Result()
		must(t, err)
		if len(keys) != 0 {
			t.Fatal("blocked authority wrote Redis")
		}
		cancel()
		err = await(t, out)
		wantError(t, err, context.Canceled)
		if errors.Is(err, identity.ErrGuardedOperation) {
			t.Fatal("pre-callback cancellation acquired guarded-effects classification")
		}
		gone(t, f.ctx, f.pools[1], b)
		must(t, held.Rollback(f.ctx))
	})
	t.Run("record changes after locate are rechecked inside guard", func(t *testing.T) {
		p := f.subject(t, false)
		ns := namespace(t)
		auth := &windowAuthority{Store: f.identities[0]}
		s, err := protocol.New(f.ctx, f.clients[0], auth, f.budget, ns, protocol.OrdinaryPolicy())
		must(t, err)
		_, c := f.create(t, s, p)
		auth.before = func() { must(t, f.control.HSet(f.ctx, tokenKey(ns, c), "credential", "2").Err()) }
		v, err := s.Read(f.ctx, c)
		wantError(t, err, protocol.ErrConflict)
		if v != (protocol.View{}) {
			t.Fatal("preloaded record escaped the final Redis recheck")
		}
	})
	t.Run("lost backend permits late Redis effect but cannot authorize it", func(t *testing.T) {
		p := f.subject(t, false)
		ns := namespace(t)
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		open := func() { once.Do(func() { close(release) }) }
		defer open()
		h := &hook{op: "touch", before: func(context.Context, redis.Cmder) error { close(entered); <-release; return nil }}
		s := f.faultStore(t, ns, h, time.Second)
		_, c := f.create(t, s, p)
		before, err := s.Read(f.ctx, c)
		must(t, err)
		f.waitTime(t, before.CreatedAt.Add(10*time.Millisecond))
		out := make(chan error, 1)
		go func() {
			v, err := s.Touch(f.ctx, c)
			if v != (protocol.View{}) {
				out <- errors.New("lost authority returned an authenticated view")
				return
			}
			out <- err
		}()
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("touch did not reach real authority callback")
		}
		b := oneBackend(t, f.ctx, f.pools[1], 0)
		var killed bool
		must(t, f.pools[1].QueryRow(f.ctx, `SELECT pg_terminate_backend($1)`, b.pid).Scan(&killed))
		if !killed {
			t.Fatal("authority backend not terminated")
		}
		gone(t, f.ctx, f.pools[1], b)
		r, err := f.hasher.Create([]byte("synthetic changed password"))
		must(t, err)
		_, err = f.identities[1].ReplacePasswordCAS(f.ctx, p.ID, p.Revision, r)
		must(t, err)
		open()
		err = await(t, out)
		wantError(t, err, identity.ErrGuardedOperation)
		wantError(t, err, identity.ErrStore)
		idle, err := f.control.HGet(f.ctx, tokenKey(ns, c), "idle").Int64()
		must(t, err)
		if idle <= before.IdleExpiresAt.UnixMilli() {
			t.Fatal("fixture did not demonstrate a real late Redis side effect")
		}
		_, err = s.Read(f.ctx, c)
		wantError(t, err, identity.ErrConflict)
	})
}
