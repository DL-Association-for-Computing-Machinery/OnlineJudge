//go:build integration

package sessionprotocol_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

type fixture struct {
	ctx         context.Context
	pools       [2]*pgxpool.Pool
	identities  [2]*identitypg.Store
	clients     [2]*redis.Client
	control     *redis.Client
	hasher, old *password.Hasher
	budget      *password.BoundedHasher
	next        int
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func hash(text string) string       { sum := sha256.Sum256([]byte(text)); return hex.EncodeToString(sum[:]) }
func namespace(t *testing.T) string { return "p_" + hash(t.Name())[:24] }
func tokenKey(ns string, c protocol.Credential) string {
	return "oj-session:" + ns + ":token:" + hash(c.Value())
}
func redisMember(member string, expiry int64) redis.Z {
	return redis.Z{Score: float64(expiry), Member: member}
}

func setup(t *testing.T) *fixture {
	t.Helper()
	if os.Getenv("OJ_PROTOCOL_FIXTURE") != "1" || os.Getenv("OJ_PROTOCOL_DATABASE_URL") == "" || os.Getenv("OJ_PROTOCOL_REDIS_ADDR") == "" {
		t.Fatal("use scripts/test-session-protocol.py with disposable stores")
	}
	ctx := context.Background()
	conn, err := sql.Open("pgx", os.Getenv("OJ_PROTOCOL_DATABASE_URL"))
	must(t, err)
	defer func() { must(t, conn.Close()) }()
	var count int
	must(t, conn.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&count))
	if count != 0 {
		t.Fatal("protocol database must start empty")
	}
	provider, err := db.Provider(conn)
	must(t, err)
	_, err = provider.Up(ctx)
	must(t, err)
	current := password.Parameters{MemoryKiB: 128, Iterations: 2, Parallelism: 2, SaltBytes: 16, DigestBytes: 32}
	legacy := password.Parameters{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltBytes: 8, DigestBytes: 24}
	limits := password.Limits{MaxMemoryKiB: 256, MaxIterations: 4, MaxParallelism: 4, MaxPasswordBytes: 128, MaxSaltBytes: 64, MaxDigestBytes: 64}
	h, err := password.New(password.Policy{Current: current, Legacy: []password.Parameters{legacy}}, limits)
	must(t, err)
	old, err := password.New(password.Policy{Current: legacy}, limits)
	must(t, err)
	budget, err := password.NewBounded(h, password.AdmissionLimits{MaxInFlight: 4, MaxWaiting: 16, MaxQueueWait: time.Second})
	must(t, err)
	f := &fixture{ctx: ctx, hasher: h, old: old, budget: budget, next: 100}
	for i := range 2 {
		f.pools[i], err = submissionpg.Open(ctx, os.Getenv("OJ_PROTOCOL_DATABASE_URL"))
		must(t, err)
		t.Cleanup(f.pools[i].Close)
		f.identities[i], err = identitypg.New(f.pools[i], h)
		must(t, err)
		f.clients[i], err = protocol.NewClient(protocol.ClientConfig{Addr: os.Getenv("OJ_PROTOCOL_REDIS_ADDR"), Connections: 8, Timeout: time.Second})
		must(t, err)
		t.Cleanup(func() { must(t, f.clients[i].Close()) })
	}
	f.control, err = protocol.NewClient(protocol.ClientConfig{Addr: os.Getenv("OJ_PROTOCOL_REDIS_ADDR"), Connections: 2, Timeout: time.Second})
	must(t, err)
	t.Cleanup(func() { must(t, f.control.Close()) })
	size, err := f.control.DBSize(ctx).Result()
	must(t, err)
	if size != 0 {
		t.Fatal("protocol Redis must start empty")
	}
	f.open(t)
	return f
}

func (f *fixture) open(t *testing.T) string {
	t.Helper()
	epoch, err := f.identities[0].BeginRecovery(f.ctx)
	must(t, err)
	must(t, f.identities[0].FinishRecovery(f.ctx, epoch))
	return epoch
}
func (f *fixture) subject(t *testing.T, legacy bool) identity.Personal {
	t.Helper()
	f.next++
	email, err := identity.ParseEmail(fmt.Sprintf("protocol-%d@example.invalid", f.next))
	must(t, err)
	writer, h := f.identities[0], f.hasher
	if legacy {
		h = f.old
		writer, err = identitypg.New(f.pools[0], h)
		must(t, err)
	}
	r, err := h.Create([]byte("synthetic protocol password"))
	must(t, err)
	p, err := writer.InitializePersonal(f.ctx, fmt.Sprintf("%032x", f.next), email, true, r)
	must(t, err)
	return p
}
func (f *fixture) store(t *testing.T, ns string, i int, p protocol.Policy) *protocol.Store {
	t.Helper()
	s, err := protocol.New(f.ctx, f.clients[i], f.identities[i], f.budget, ns, p)
	must(t, err)
	return s
}
func (f *fixture) create(t *testing.T, s *protocol.Store, p identity.Personal) (*protocol.Attempt, protocol.Credential) {
	t.Helper()
	a, err := s.Create(f.ctx, p.Email, []byte("synthetic protocol password"))
	must(t, err)
	if a == nil {
		t.Fatal("admission omitted attempt")
	}
	c, err := s.Check(f.ctx, a)
	must(t, err)
	return a, c
}
func (f *fixture) waitTime(t *testing.T, target time.Time) {
	t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		now, err := f.control.Time(ctx).Result()
		must(t, err)
		if !now.Before(target) {
			return
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			t.Fatal("Redis time boundary not reached")
		}
	}
}
func await(t *testing.T, out <-chan error) error {
	t.Helper()
	select {
	case err := <-out:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("protocol operation did not finish")
		return nil
	}
}
func wantError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

type hook struct {
	op     string
	used   atomic.Bool
	before func(context.Context, redis.Cmder) error
	after  func(context.Context, redis.Cmder) error
}

func (h *hook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *hook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *hook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		matches := h.op == "time" && cmd.Name() == "time"
		if cmd.Name() == "eval" && len(args) >= 2 {
			script, ok := args[1].(string)
			matches = ok && strings.Contains(script, "-- session-protocol:"+h.op+"\n")
		}
		if !matches || !h.used.CompareAndSwap(false, true) {
			return next(ctx, cmd)
		}
		if h.before != nil {
			if err := h.before(ctx, cmd); err != nil {
				return err
			}
		}
		if err := next(ctx, cmd); err != nil {
			return err
		}
		if h.after != nil {
			return h.after(ctx, cmd)
		}
		return nil
	}
}

func (f *fixture) faultStore(t *testing.T, ns string, h *hook, timeout time.Duration) *protocol.Store {
	t.Helper()
	c, err := protocol.NewClient(protocol.ClientConfig{Addr: os.Getenv("OJ_PROTOCOL_REDIS_ADDR"), Connections: 4, Timeout: timeout})
	must(t, err)
	t.Cleanup(func() { must(t, c.Close()) })
	s, err := protocol.New(f.ctx, c, f.identities[0], f.budget, ns, protocol.OrdinaryPolicy())
	must(t, err)
	c.AddHook(h)
	return s
}
