//go:build integration

package sessionprotocol_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func changeRules(t *testing.T, minimum int, blocked string) *password.Acceptance {
	t.Helper()
	r, err := password.NewAcceptance(minimum, 128, [][32]byte{sha256.Sum256([]byte(blocked))})
	must(t, err)
	return r
}

func changeLimits() protocol.PasswordChangeLimits {
	b := protocol.LoginBucket{Count: 10, Window: time.Hour}
	return protocol.PasswordChangeLimits{Global: b, Source: b, Subject: b}
}

func changeSnapshot(t *testing.T, f *fixture, ns string) map[string]rateRecord {
	t.Helper()
	return admissionSnapshot(t, f, "oj-password-change-limit:"+ns+":*")
}

func unchangedChange(t *testing.T, f *fixture, ns string, before map[string]rateRecord) {
	t.Helper()
	if !reflect.DeepEqual(before, changeSnapshot(t, f, ns)) {
		t.Fatal("password change rejection modified counters or absolute deadlines")
	}
}

func testPasswordChangeLimits(t *testing.T, f *fixture) {
	rules := changeRules(t, 15, "synthetic blocked password")
	t.Run("three independent quotas are atomic across replicas", func(t *testing.T) {
		for _, dimension := range []string{"global", "source", "subject"} {
			t.Run(dimension, func(t *testing.T) {
				ns := namespace(t)
				secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
				limits := changeLimits()
				switch dimension {
				case "global":
					limits.Global.Count = 1
				case "source":
					limits.Source.Count = 1
				case "subject":
					limits.Subject.Count = 1
				}
				a, b := f.store(t, ns, 0, protocol.OrdinaryPolicy()), f.store(t, ns, 1, protocol.OrdinaryPolicy())
				login := limitedManager(t, f, a, secrets, rateLimits(), true)
				loginBefore := rateSnapshot(t, f, ns)
				must(t, protocol.InitializePasswordChangeLimits(f.ctx, a, secrets, limits, rules))
				first, err := protocol.NewPasswordChangeAdmission(f.ctx, a, secrets, limits, rules)
				must(t, err)
				second, err := protocol.NewPasswordChangeAdmission(f.ctx, b, secrets, limits, rules)
				must(t, err)
				p, q := f.subject(t, false), f.subject(t, false)
				source := netip.MustParseAddr("192.0.2.1")
				must(t, first.Admit(f.ctx, source, p.ID))
				before := changeSnapshot(t, f, ns)
				nextSource, nextSubject := netip.MustParseAddr("192.0.2.2"), q.ID
				if dimension == "source" {
					nextSource = source
				}
				if dimension == "subject" {
					nextSubject = p.ID
				}
				wantError(t, second.Admit(f.ctx, nextSource, nextSubject), protocol.ErrPasswordChangeRate)
				unchangedChange(t, f, ns, before)
				unchangedRate(t, f, ns, loginBefore)
				// Login admission and password-change quotas remain independent.
				must(t, login.AdmitBegin(f.ctx, source, p.Email))
				unchangedChange(t, f, ns, before)
			})
		}
	})
	t.Run("policy mismatch and lost metadata never recreate budgets", func(t *testing.T) {
		ns := namespace(t)
		secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		limits := changeLimits()
		before := changeSnapshot(t, f, ns)
		if a, err := protocol.NewPasswordChangeAdmission(f.ctx, s, secrets, limits, rules); a != nil || !errors.Is(err, protocol.ErrStore) {
			t.Fatal("missing marker admitted change")
		}
		unchangedChange(t, f, ns, before)
		must(t, protocol.InitializePasswordChangeLimits(f.ctx, s, secrets, limits, rules))
		a, err := protocol.NewPasswordChangeAdmission(f.ctx, s, secrets, limits, rules)
		must(t, err)
		p := f.subject(t, false)
		must(t, a.Admit(f.ctx, netip.MustParseAddr("192.0.2.1"), p.ID))
		before = changeSnapshot(t, f, ns)
		larger, err := password.NewAcceptance(15, 256, [][32]byte{sha256.Sum256([]byte("synthetic blocked password"))})
		must(t, err)
		for _, other := range []*password.Acceptance{changeRules(t, 16, "synthetic blocked password"), changeRules(t, 15, "synthetic other blocked password"), larger} {
			if b, err := protocol.NewPasswordChangeAdmission(f.ctx, s, secrets, limits, other); b != nil || !errors.Is(err, protocol.ErrStore) {
				t.Fatal("mismatched password policy admitted")
			}
			wantError(t, protocol.InitializePasswordChangeLimits(f.ctx, s, secrets, limits, other), protocol.ErrStore)
			unchangedChange(t, f, ns, before)
		}
		key := "oj-password-change-limit:" + ns + ":config"
		must(t, f.control.Set(f.ctx, key, "synthetic mismatched marker", 0).Err())
		afterMismatch := changeSnapshot(t, f, ns)
		wantError(t, a.Admit(f.ctx, netip.MustParseAddr("192.0.2.2"), p.ID), protocol.ErrStore)
		unchangedChange(t, f, ns, afterMismatch)
		must(t, f.control.Del(f.ctx, key).Err())
		afterDelete := changeSnapshot(t, f, ns)
		wantError(t, a.Admit(f.ctx, netip.MustParseAddr("192.0.2.2"), p.ID), protocol.ErrStore)
		unchangedChange(t, f, ns, afterDelete)
	})
	t.Run("rate errors unknown replies and canceled calls never retry", func(t *testing.T) {
		for _, outcome := range []string{"deny", "lost reply", "malformed reply", "cancel"} {
			t.Run(outcome, func(t *testing.T) {
				ns := namespace(t)
				secrets := loadedLoginSecrets(t, loginKey(t), loginKey(t))
				base := f.store(t, ns, 0, protocol.OrdinaryPolicy())
				limits := changeLimits()
				must(t, protocol.InitializePasswordChangeLimits(f.ctx, base, secrets, limits, rules))
				calls := 0
				h := &hook{op: "password-change-admit", after: func(_ context.Context, cmd redis.Cmder) error {
					calls++
					switch outcome {
					case "deny":
						cmd.(*redis.Cmd).SetVal(int64(0))
					case "lost reply":
						return errors.New("synthetic lost admission reply")
					case "malformed reply":
						cmd.(*redis.Cmd).SetVal("unexpected")
					}
					return nil
				}}
				fault := f.faultStore(t, ns, h, time.Second)
				a, err := protocol.NewPasswordChangeAdmission(f.ctx, fault, secrets, limits, rules)
				must(t, err)
				ctx := f.ctx
				if outcome == "cancel" {
					canceled, cancel := context.WithCancel(ctx)
					cancel()
					ctx = canceled
				}
				before := changeSnapshot(t, f, ns)
				err = a.Admit(ctx, netip.MustParseAddr("192.0.2.1"), f.subject(t, false).ID)
				if outcome == "cancel" {
					if !errors.Is(err, context.Canceled) || calls != 0 {
						t.Fatal("canceled admission touched storage")
					}
					unchangedChange(t, f, ns, before)
				} else {
					want := protocol.ErrStore
					if outcome == "deny" {
						want = protocol.ErrPasswordChangeRate
					}
					if !errors.Is(err, want) || calls != 1 {
						t.Fatal("admission failure lost classification or retried")
					}
					// Fault injection is after a real successful EVAL. Charges remain
					// finite; unknown/rewritten replies must never be refunded.
					after := changeSnapshot(t, f, ns)
					if len(after) != len(before)+3 {
						t.Fatal("fault fixture did not execute one real atomic charge")
					}
					for key, r := range after {
						if !strings.HasSuffix(key, ":config") && r.deadline <= 0 {
							t.Fatal("partial charge lost expiry")
						}
					}
				}
			})
		}
	})
}
