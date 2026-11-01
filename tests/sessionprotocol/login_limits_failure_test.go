//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testLoginLimitFailures(t *testing.T, f *fixture) {
	email, err := identity.ParseEmail("fault@example.invalid")
	must(t, err)
	source := netip.MustParseAddr("192.0.2.1")
	for _, mutation := range []string{"type", "format", "zero count", "over count", "noncanonical", "unaligned", "bad deadline", "no TTL", "wrong TTL"} {
		t.Run("corrupt counter "+mutation, func(t *testing.T) {
			ns := namespace(t)
			limits := rateLimits()
			limits.BeginGlobal.Count = 1
			var key string
			h := &hook{op: "login-admit", before: func(_ context.Context, cmd redis.Cmder) error { key = cmd.Args()[6].(string); return nil }}
			l := limitedManager(t, f, f.faultStore(t, ns, h, time.Second), loadedLoginSecrets(t, loginKey(t), loginKey(t)), limits, true)
			must(t, l.AdmitBegin(f.ctx, source, email))
			if key == "" {
				t.Fatal("email fault target not captured")
			}
			raw, err := f.control.Get(f.ctx, key).Result()
			must(t, err)
			deadline, err := f.control.PExpireTime(f.ctx, key).Result()
			must(t, err)
			parts := strings.Split(raw, ":")
			switch mutation {
			case "type":
				must(t, f.control.Del(f.ctx, key).Err())
				must(t, f.control.HSet(f.ctx, key, "bad", "field").Err())
			case "no TTL":
				must(t, f.control.Persist(f.ctx, key).Err())
			case "wrong TTL":
				must(t, f.control.PExpireAt(f.ctx, key, time.UnixMilli(deadline.Milliseconds()+1)).Err())
			default:
				switch mutation {
				case "format":
					parts = []string{"invalid"}
				case "zero count":
					parts[1] = "0"
				case "over count":
					parts[1] = "11"
				case "noncanonical":
					parts[1] = "01"
				case "unaligned":
					parts[0] = "1"
				case "bad deadline":
					parts[2] = "1"
				}
				must(t, f.control.Do(f.ctx, "SET", key, strings.Join(parts, ":"), "PXAT", deadline.Milliseconds()).Err())
			}
			before := rateSnapshot(t, f, ns)
			// Global is already full. A corrupt later dimension must still win
			// as unavailable, rather than being hidden behind a quota response.
			wantError(t, l.AdmitBegin(f.ctx, source, email), protocol.ErrStore)
			unchangedRate(t, f, ns, before)
		})
	}
	for _, mutation := range []string{"type", "value", "TTL"} {
		t.Run("corrupt metadata "+mutation, func(t *testing.T) {
			ns := namespace(t)
			limits := rateLimits()
			s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
			secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
			l := limitedManager(t, f, s, secrets, limits, true)
			key := "oj-login-limit:" + ns + ":config"
			switch mutation {
			case "type":
				must(t, f.control.Del(f.ctx, key).Err())
				must(t, f.control.HSet(f.ctx, key, "bad", "field").Err())
			case "value":
				must(t, f.control.Set(f.ctx, key, "inconsistent", 0).Err())
			case "TTL":
				must(t, f.control.PExpire(f.ctx, key, time.Hour).Err())
			}
			before := rateSnapshot(t, f, ns)
			wantError(t, l.AdmitRecovery(f.ctx, source), protocol.ErrStore)
			if next, err := protocol.NewLimitedLogin(f.ctx, s, loginOrigin, time.Minute, secrets, limits); next != nil || !errors.Is(err, protocol.ErrStore) {
				t.Fatal("corrupt metadata allowed construction")
			}
			unchangedRate(t, f, ns, before)
		})
	}
	t.Run("logical window equality tolerates retained physical keys", func(t *testing.T) {
		ns := namespace(t)
		limits := rateLimits()
		limits.BeginGlobal.Count = 1
		secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
		l := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), secrets, limits, true)
		must(t, l.AdmitBegin(f.ctx, source, email))
		keys := rateCounterKeys(t, f, ns)
		deadline, err := f.control.PExpireTime(f.ctx, keys[0]).Result()
		must(t, err)
		h := &hook{op: "login-admit", before: func(_ context.Context, cmd redis.Cmder) error {
			args := cmd.Args()
			script := args[1].(string)
			needle := "local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)"
			if strings.Count(script, needle) != 1 {
				t.Fatal("clock injection point changed")
			}
			args[1] = strings.Replace(script, needle, "local now = "+strconv.FormatInt(deadline.Milliseconds(), 10), 1)
			return nil
		}}
		other := limitedManager(t, f, f.faultStore(t, ns, h, time.Second), secrets, limits, false)
		must(t, other.AdmitBegin(f.ctx, source, email))
		if !h.used.Load() {
			t.Fatal("equality injection unused")
		}
	})
	t.Run("partial runtime error keeps only bounded global charge", func(t *testing.T) {
		ns := namespace(t)
		h := &hook{op: "login-admit", before: func(_ context.Context, cmd redis.Cmder) error {
			args := cmd.Args()
			script := args[1].(string)
			needle := "redis.call('SET', KEYS[i], raw, 'PXAT', string.format('%.0f', deadlines[i]))"
			if strings.Count(script, needle) != 1 {
				t.Fatal("partial charge injection point changed")
			}
			args[1] = strings.Replace(script, needle, needle+"\n    if i == 2 then error('synthetic partial quota write') end", 1)
			return nil
		}}
		l := limitedManager(t, f, f.faultStore(t, ns, h, time.Second), loadedLoginSecrets(t, loginKey(t), loginKey(t)), rateLimits(), true)
		wantError(t, l.AdmitBegin(f.ctx, source, email), protocol.ErrStore)
		keys := rateCounterKeys(t, f, ns)
		if len(keys) != 1 {
			t.Fatal("partial quota error retried or wrote beyond global")
		}
		ttl, err := f.control.PTTL(f.ctx, keys[0]).Result()
		must(t, err)
		if ttl <= 0 || ttl > time.Hour {
			t.Fatal("partial counter has no bounded TTL")
		}
	})
	t.Run("OOM rejects admission without writes", func(t *testing.T) {
		ns := namespace(t)
		l := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), loadedLoginSecrets(t, loginKey(t), loginKey(t)), rateLimits(), true)
		before := rateSnapshot(t, f, ns)
		old, err := f.control.ConfigGet(f.ctx, "maxmemory").Result()
		must(t, err)
		defer func() { must(t, f.control.ConfigSet(f.ctx, "maxmemory", old["maxmemory"]).Err()) }()
		must(t, f.control.ConfigSet(f.ctx, "maxmemory", "1").Err())
		wantError(t, l.AdmitBegin(f.ctx, source, email), protocol.ErrStore)
		unchangedRate(t, f, ns, before)
	})
}
