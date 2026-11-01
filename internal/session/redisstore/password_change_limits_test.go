package redisstore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

func unitPasswordChangeLimits() PasswordChangeLimits {
	b := LoginBucket{Count: 3, Window: time.Minute}
	return PasswordChangeLimits{b, b, b}
}

func unitPasswordAcceptance(t *testing.T) *password.Acceptance {
	t.Helper()
	a, err := password.NewAcceptance(15, 128, [][32]byte{sha256.Sum256([]byte("synthetic blocked password"))})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestPasswordChangeAdmissionBoundsAndSeparation(t *testing.T) {
	store, _, _ := sealingFixture(t)
	secrets, err := parseLoginSecrets([]byte(syntheticSecrets()))
	if err != nil {
		t.Fatal(err)
	}
	acceptance := unitPasswordAcceptance(t)
	for i := range 3 {
		for _, bad := range []LoginBucket{{0, time.Minute}, {-1, time.Minute}, {1000001, time.Minute}, {1, 0}, {1, time.Nanosecond}, {1, 25 * time.Hour}, {1, time.Millisecond + 1}} {
			limits := unitPasswordChangeLimits()
			buckets := []*LoginBucket{&limits.Global, &limits.Source, &limits.Subject}
			*buckets[i] = bad
			if limits.Validate() != ErrInvalid {
				t.Fatal("invalid explicit quota accepted")
			}
			if a, err := newPasswordChangeAdmission(store, secrets, limits, acceptance); a != nil || err != ErrInvalid {
				t.Fatal("invalid admission constructed")
			}
		}
	}
	a, err := newPasswordChangeAdmission(store, secrets, unitPasswordChangeLimits(), acceptance)
	if err != nil {
		t.Fatal(err)
	}
	l, err := newLoginAdmission(store, secrets, unitLoginLimits())
	if err != nil {
		t.Fatal(err)
	}
	if a.prefix == l.prefix || a.mac("config", "same") == l.mac("config", "same") || a.mac("source", "192.0.2.1") == a.mac("subject", "192.0.2.1") {
		t.Fatal("password change and login quota identities collided")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, source := range []string{"0.0.0.0", "::", "224.0.0.1", "ff02::1", "fe80::1%eth0"} {
		if a.Admit(t.Context(), netip.MustParseAddr(source), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") != ErrInvalid {
			t.Fatal("invalid source reached Redis")
		}
	}
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{{nil, ErrInvalid}, {ctx, context.Canceled}} {
		if a.Admit(tc.ctx, netip.MustParseAddr("192.0.2.1"), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") != tc.want {
			t.Fatal("invalid context reached Redis")
		}
	}
	for _, zero := range []*PasswordChangeAdmission{nil, {}} {
		if zero.Admit(t.Context(), netip.MustParseAddr("192.0.2.1"), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") != ErrInvalid {
			t.Fatal("zero admission accepted")
		}
	}
	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%b", "%o", "%O", "%U", "%c", "%f", "%e", "%g"} {
		for _, value := range []any{*a, a} {
			if fmt.Sprintf(format, value) != "[redacted password change admission]" {
				t.Fatal("admission exposed key or owner")
			}
		}
	}
	for _, value := range []any{*a, a, []any{a}, map[string]any{"admission": a}} {
		body, err := json.Marshal(value)
		if err != nil || len(body) > 32 {
			t.Fatal("admission JSON exposed private data")
		}
	}
	owner := PasswordChange{login: &Login{store: store, admission: l}, admission: a, acceptance: acceptance}
	for _, value := range []any{owner, &owner} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%b", "%o", "%O", "%U", "%c", "%f", "%e", "%g"} {
			if fmt.Sprintf(format, value) != "[redacted password change owner]" {
				t.Fatal("password change owner exposed nested secret")
			}
		}
		body, err := json.Marshal(value)
		if err != nil || string(body) != "{}" {
			t.Fatal("password change owner JSON exposed private state")
		}
	}
}
