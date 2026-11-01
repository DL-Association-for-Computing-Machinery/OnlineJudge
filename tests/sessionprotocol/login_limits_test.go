//go:build integration

package sessionprotocol_test

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testLoginLimits(t *testing.T, f *fixture) {
	f.open(t)
	t.Run("replicas compete for exactly one global admission", func(t *testing.T) {
		ns := namespace(t)
		limits := rateLimits()
		limits.BeginGlobal.Count = 1
		secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
		a := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), secrets, limits, true)
		b := limitedManager(t, f, f.store(t, ns, 1, protocol.OrdinaryPolicy()), secrets, limits, false)
		email, err := identity.ParseEmail("race@example.invalid")
		must(t, err)
		out := make(chan error, 16)
		start := make(chan struct{})
		for i := range 16 {
			go func() {
				<-start
				l := a
				if i%2 == 1 {
					l = b
				}
				out <- l.AdmitBegin(f.ctx, netip.MustParseAddr(fmt.Sprintf("192.0.2.%d", i+1)), email)
			}()
		}
		close(start)
		accepted := 0
		for range 16 {
			err := <-out
			if err == nil {
				accepted++
			} else {
				wantError(t, err, protocol.ErrLoginRate)
			}
		}
		if accepted != 1 || len(rateCounterKeys(t, f, ns)) != 3 {
			t.Fatal("replicas exceeded global admission or denial created keys")
		}
		if len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("admission itself created a session")
		}
	})
	t.Run("email denial and high cardinality global denial write nothing", func(t *testing.T) {
		ns := namespace(t)
		limits := rateLimits()
		limits.BeginGlobal.Count = 2
		limits.BeginEmail.Count = 1
		l := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), loadedLoginSecrets(t, loginKey(t), loginKey(t)), limits, true)
		e1, err := identity.ParseEmail("Case@example.invalid")
		must(t, err)
		e1upper, err := identity.ParseEmail("CASE@EXAMPLE.INVALID")
		must(t, err)
		e2, err := identity.ParseEmail("other@example.invalid")
		must(t, err)
		must(t, l.AdmitBegin(f.ctx, netip.MustParseAddr("192.0.2.1"), e1))
		before := rateSnapshot(t, f, ns)
		wantError(t, l.AdmitBegin(f.ctx, netip.MustParseAddr("192.0.2.2"), e1upper), protocol.ErrLoginRate)
		unchangedRate(t, f, ns, before)
		must(t, l.AdmitBegin(f.ctx, netip.MustParseAddr("192.0.2.2"), e2))
		before = rateSnapshot(t, f, ns)
		for i := range 32 {
			e, err := identity.ParseEmail(fmt.Sprintf("unknown%d@example.invalid", i))
			must(t, err)
			wantError(t, l.AdmitBegin(f.ctx, netip.MustParseAddr(fmt.Sprintf("198.51.100.%d", i+1)), e), protocol.ErrLoginRate)
		}
		unchangedRate(t, f, ns, before)
		for k, v := range before {
			if strings.Contains(k+v.dump, "example.invalid") || strings.Contains(k+v.dump, "192.0.2.") {
				t.Fatal("rate records retained raw identities")
			}
		}
		must(t, l.AdmitRecovery(f.ctx, netip.MustParseAddr("192.0.2.1")))
	})
	t.Run("mapped source shares quota and source denial does not charge email", func(t *testing.T) {
		ns := namespace(t)
		limits := rateLimits()
		limits.BeginSource.Count = 1
		l := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), loadedLoginSecrets(t, loginKey(t), loginKey(t)), limits, true)
		e1, err := identity.ParseEmail("first@example.invalid")
		must(t, err)
		e2, err := identity.ParseEmail("second@example.invalid")
		must(t, err)
		must(t, l.AdmitBegin(f.ctx, netip.MustParseAddr("192.0.2.1"), e1))
		before := rateSnapshot(t, f, ns)
		wantError(t, l.AdmitBegin(f.ctx, netip.MustParseAddr("::ffff:192.0.2.1"), e2), protocol.ErrLoginRate)
		unchangedRate(t, f, ns, before)
		must(t, l.AdmitBegin(f.ctx, netip.MustParseAddr("192.0.2.2"), e2))
	})
	t.Run("recovery source quota is shared and rejection writes nothing", func(t *testing.T) {
		ns := namespace(t)
		limits := rateLimits()
		limits.RecoverySource.Count = 1
		l := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), loadedLoginSecrets(t, loginKey(t), loginKey(t)), limits, true)
		must(t, l.AdmitRecovery(f.ctx, netip.MustParseAddr("192.0.2.1")))
		before := rateSnapshot(t, f, ns)
		wantError(t, l.AdmitRecovery(f.ctx, netip.MustParseAddr("::ffff:192.0.2.1")), protocol.ErrLoginRate)
		unchangedRate(t, f, ns, before)
		must(t, l.AdmitRecovery(f.ctx, netip.MustParseAddr("192.0.2.2")))
	})
	t.Run("expiry follows Redis time and resets bounded windows", func(t *testing.T) {
		ns := namespace(t)
		limits := rateLimits()
		b := protocol.LoginBucket{Count: 1, Window: time.Second}
		limits.BeginGlobal = b
		limits.BeginSource = b
		limits.BeginEmail = b
		l := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), loadedLoginSecrets(t, loginKey(t), loginKey(t)), limits, true)
		email, err := identity.ParseEmail("expiry@example.invalid")
		must(t, err)
		source := netip.MustParseAddr("192.0.2.1")
		now, err := f.control.Time(f.ctx).Result()
		must(t, err)
		if remaining := 1000 - now.UnixMilli()%1000; remaining < 500 {
			f.waitTime(t, time.UnixMilli(now.UnixMilli()+remaining+1))
		}
		must(t, l.AdmitBegin(f.ctx, source, email))
		keys := rateCounterKeys(t, f, ns)
		if len(keys) != 3 {
			t.Fatal("accepted window missing counters")
		}
		deadline, err := f.control.PExpireTime(f.ctx, keys[0]).Result()
		must(t, err)
		f.waitTime(t, time.UnixMilli(deadline.Milliseconds()).Add(time.Millisecond))
		if keys := rateCounterKeys(t, f, ns); len(keys) != 0 {
			t.Fatal("expired window retained counters")
		}
		must(t, l.AdmitBegin(f.ctx, source, email))
	})
	t.Run("expired short windows stay absent when long email bucket refuses", func(t *testing.T) {
		ns := namespace(t)
		limits := rateLimits()
		limits.BeginGlobal = protocol.LoginBucket{Count: 10, Window: time.Second}
		limits.BeginSource = limits.BeginGlobal
		limits.BeginEmail.Count = 1
		l := limitedManager(t, f, f.store(t, ns, 0, protocol.OrdinaryPolicy()), loadedLoginSecrets(t, loginKey(t), loginKey(t)), limits, true)
		email, err := identity.ParseEmail("long-window@example.invalid")
		must(t, err)
		source := netip.MustParseAddr("192.0.2.1")
		now, err := f.control.Time(f.ctx).Result()
		must(t, err)
		if remaining := time.Hour.Milliseconds() - now.UnixMilli()%time.Hour.Milliseconds(); remaining < 2000 {
			f.waitTime(t, time.UnixMilli(now.UnixMilli()+remaining+1))
		}
		now, err = f.control.Time(f.ctx).Result()
		must(t, err)
		if remaining := 1000 - now.UnixMilli()%1000; remaining < 500 {
			f.waitTime(t, time.UnixMilli(now.UnixMilli()+remaining+1))
		}
		must(t, l.AdmitBegin(f.ctx, source, email))
		keys := rateCounterKeys(t, f, ns)
		if len(keys) != 3 {
			t.Fatal("initial mixed windows missing counters")
		}
		var shortDeadline, longDeadline time.Duration
		for _, k := range keys {
			d, err := f.control.PExpireTime(f.ctx, k).Result()
			must(t, err)
			if shortDeadline == 0 || d < shortDeadline {
				shortDeadline = d
			}
			if d > longDeadline {
				longDeadline = d
			}
		}
		if shortDeadline >= longDeadline {
			t.Fatal("mixed window fixture has no independent deadlines")
		}
		f.waitTime(t, time.UnixMilli(shortDeadline.Milliseconds()).Add(time.Millisecond))
		before := rateSnapshot(t, f, ns)
		if len(before) != 2 {
			t.Fatal("short counters not expired independently of email")
		}
		wantError(t, l.AdmitBegin(f.ctx, source, email), protocol.ErrLoginRate)
		unchangedRate(t, f, ns, before)
		other, err := identity.ParseEmail("new-window@example.invalid")
		must(t, err)
		must(t, l.AdmitBegin(f.ctx, source, other))
		after := rateSnapshot(t, f, ns)
		for k, v := range before {
			if after[k] != v {
				t.Fatal("unrelated long window record changed")
			}
		}
	})
	t.Run("metadata never repaired by construction or requests", func(t *testing.T) {
		ns := namespace(t)
		limits := rateLimits()
		limits.BeginGlobal.Count = 1
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		sealing, limiting := loginKey(t), loginKey(t)
		secrets := loadedLoginSecrets(t, sealing, limiting)
		if l, err := protocol.NewLimitedLogin(f.ctx, s, loginOrigin, time.Minute, secrets, limits); l != nil || !errors.Is(err, protocol.ErrStore) {
			t.Fatal("missing initial marker accepted")
		}
		if len(rateSnapshot(t, f, ns)) != 0 {
			t.Fatal("constructor provisioned metadata")
		}
		l := limitedManager(t, f, s, secrets, limits, true)
		must(t, protocol.InitializeLoginLimits(f.ctx, s, secrets, limits))
		email, err := identity.ParseEmail("config@example.invalid")
		must(t, err)
		source := netip.MustParseAddr("192.0.2.1")
		must(t, l.AdmitBegin(f.ctx, source, email))
		before := rateSnapshot(t, f, ns)
		changed := limits
		changed.BeginEmail.Count++
		for _, c := range []struct {
			secrets *protocol.LoginSecrets
			limits  protocol.LoginLimits
		}{{secrets, changed}, {loadedLoginSecrets(t, sealing, loginKey(t)), limits}} {
			if next, err := protocol.NewLimitedLogin(f.ctx, s, loginOrigin, time.Minute, c.secrets, c.limits); next != nil || !errors.Is(err, protocol.ErrStore) {
				t.Fatal("inconsistent replica admitted")
			}
			wantError(t, protocol.InitializeLoginLimits(f.ctx, s, c.secrets, c.limits), protocol.ErrStore)
		}
		unchangedRate(t, f, ns, before)
		rotated := limitedManager(t, f, s, loadedLoginSecrets(t, loginKey(t), limiting), limits, false)
		wantError(t, rotated.AdmitBegin(f.ctx, source, email), protocol.ErrLoginRate)
		unchangedRate(t, f, ns, before)
		key := "oj-login-limit:" + ns + ":config"
		must(t, f.control.Del(f.ctx, key).Err())
		before = rateSnapshot(t, f, ns)
		wantError(t, l.AdmitRecovery(f.ctx, source), protocol.ErrStore)
		if next, err := protocol.NewLimitedLogin(f.ctx, s, loginOrigin, time.Minute, secrets, limits); next != nil || !errors.Is(err, protocol.ErrStore) {
			t.Fatal("restart repaired deleted metadata")
		}
		unchangedRate(t, f, ns, before)
	})
}
