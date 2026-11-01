package redisstore

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

func unitLoginLimits() LoginLimits {
	b := LoginBucket{Count: 3, Window: time.Minute}
	return LoginLimits{b, b, b, b, b}
}

func TestLoginLimitsRequireAllExplicitBudgets(t *testing.T) {
	store, _, _ := sealingFixture(t)
	secrets, err := parseLoginSecrets([]byte(syntheticSecrets()))
	if err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		for _, bad := range []LoginBucket{{0, time.Minute}, {-1, time.Minute}, {1000001, time.Minute}, {1, 0}, {1, time.Nanosecond}, {1, 25 * time.Hour}, {1, time.Millisecond + 1}} {
			c := unitLoginLimits()
			buckets := []*LoginBucket{&c.BeginGlobal, &c.BeginSource, &c.BeginEmail, &c.RecoveryGlobal, &c.RecoverySource}
			*buckets[i] = bad
			if a, err := newLoginAdmission(store, secrets, c); a != nil || err != ErrInvalid {
				t.Fatal("invalid budget accepted")
			}
		}
	}
	a, err := newLoginAdmission(store, secrets, unitLoginLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{Login{admission: a}, &Login{admission: a}} {
		if fmt.Sprintf("%v %+v %#v", v, v, v) != strings.Repeat("[redacted login manager] ", 2)+"[redacted login manager]" {
			t.Fatal("manager exposed admission fields")
		}
	}
	if a.mac("begin-source", "192.0.2.1") == a.mac("recovery-source", "192.0.2.1") || a.mac("begin-source", "192.0.2.1") == a.mac("begin-email", "192.0.2.1") {
		t.Fatal("HMAC purposes collided")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{{nil, ErrInvalid}, {ctx, context.Canceled}} {
		if err := a.run(tc.ctx, "", nil, nil); err != tc.want {
			t.Fatal("invalid context reached Redis")
		}
	}
	for _, l := range []*Login{nil, {}, {store: store}} {
		if l.RateLimited() || l.AdmitRecovery(context.Background(), netip.MustParseAddr("192.0.2.1")) != ErrInvalid || l.AdmitBegin(context.Background(), netip.Addr{}, identity.Email{}) != ErrInvalid {
			t.Fatal("unconfigured admission permitted")
		}
	}
}
