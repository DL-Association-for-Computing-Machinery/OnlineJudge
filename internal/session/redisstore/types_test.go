package redisstore

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

func TestCredentialValidationAndRedaction(t *testing.T) {
	raw := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	c, err := ParseCredential(raw)
	if err != nil || c.Value() != raw {
		t.Fatal("canonical credential rejected")
	}
	for _, value := range []any{c, &c, Attempt{credential: c}, &Attempt{credential: c}} {
		if strings.Contains(fmt.Sprintf("%v %+v %#v", value, value, value), raw) {
			t.Fatal("credential leaked through formatting")
		}
	}
	for _, bad := range []string{"", raw + "=", raw[:42], strings.Repeat("!", 43), raw[:42] + "B"} {
		if c, err := ParseCredential(bad); err != ErrInvalid || c != (Credential{}) {
			t.Fatal("noncanonical credential accepted")
		}
	}
}

func TestOrdinaryPolicyAndFrozenAttempt(t *testing.T) {
	p := OrdinaryPolicy()
	if !p.valid() || p.Idle != 2*time.Hour || p.Absolute != 12*time.Hour {
		t.Fatal("ordinary policy changed")
	}
	for _, bad := range []Policy{{}, {Idle: time.Nanosecond, Absolute: time.Second}, {Idle: time.Hour, Absolute: time.Hour}, {Idle: 3 * time.Hour, Absolute: 12 * time.Hour}, {Idle: time.Hour, Absolute: 13 * time.Hour}} {
		if bad.valid() {
			t.Fatal("invalid/unbounded policy accepted")
		}
	}
	c, _ := ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	a := Attempt{namespace: "fixture", credential: c, state: identity.SessionState{State: identity.State{ID: strings.Repeat("1", 32), Enabled: true,
		Revision: identity.Revision{CredentialVersion: 1, AuthGeneration: 1}}, RecoveryID: strings.Repeat("2", 32)}, created: 1000, absolute: 1000 + p.Absolute.Milliseconds(), initial: 1000 + p.Idle.Milliseconds()}
	if !a.valid("fixture", p) {
		t.Fatal("valid frozen attempt rejected")
	}
	for _, mutate := range []func(*Attempt){func(a *Attempt) { a.namespace = "other" }, func(a *Attempt) { a.initial++ }, func(a *Attempt) { a.absolute++ }, func(a *Attempt) { a.state.AuthGeneration = 0 }, func(a *Attempt) { a.state.Enabled = false }, func(a *Attempt) { a.credential = Credential{} }} {
		bad := a
		mutate(&bad)
		if bad.valid("fixture", p) {
			t.Fatal("changed attempt accepted")
		}
	}
}

func TestExplicitClientBounds(t *testing.T) {
	for _, cfg := range []ClientConfig{{}, {Addr: "fixture", Connections: 1}, {Addr: "fixture", Connections: 1, Timeout: 6 * time.Second}} {
		if c, err := NewClient(cfg); c != nil || err != ErrInvalid {
			t.Fatal("unbounded client accepted")
		}
	}
	c, err := NewClient(ClientConfig{Addr: "unused.invalid:6379", Connections: 2, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	})
	if !boundedClient(c) {
		t.Fatal("constructed client failed its bounds")
	}
	if _, err := New(context.Background(), c, nil, nil, "fixture", OrdinaryPolicy()); err != identity.ErrInvalid {
		t.Fatal("nil authority accepted")
	}
	for _, s := range []*Store{nil, {}} {
		if a, err := s.Create(context.Background(), identity.Email{}, nil); a != nil || err != ErrInvalid {
			t.Fatal("zero store did not fail closed")
		}
		if _, err := s.Read(context.Background(), Credential{}); err != ErrInvalid {
			t.Fatal("zero store read")
		}
	}
}

type failedEntropy struct{ calls int }

func (r *failedEntropy) Read([]byte) (int, error) {
	r.calls++
	return 0, errors.New("synthetic entropy failure")
}

type unusedAuthority struct {
	identity.PersonalCredentialStore
}

func (*unusedAuthority) WithSession(context.Context, string, identity.SessionVersion, func(context.Context, identity.SessionState) error) error {
	panic("unexpected authority use")
}

func TestEntropyFailureAndCancellationBeforeAuthority(t *testing.T) {
	c, err := NewClient(ClientConfig{Addr: "unused.invalid:6379", Connections: 1, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	})
	auth := &unusedAuthority{}
	g, err := identity.NewPersonalCredentialGate(auth, &password.BoundedHasher{})
	if err != nil {
		t.Fatal(err)
	}
	r := &failedEntropy{}
	s := &Store{client: c, authority: auth, gate: g, namespace: "fixture", policy: OrdinaryPolicy(), random: r}
	email, err := identity.ParseEmail("synthetic@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	if a, err := s.Create(context.Background(), email, []byte("synthetic")); a != nil || err != ErrRandom || r.calls != 1 {
		t.Fatal("failed entropy reached authority or returned partial attempt")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a, err := s.Create(ctx, email, nil); a != nil || !errors.Is(err, context.Canceled) || r.calls != 1 {
		t.Fatal("pre-canceled call consumed entropy")
	}
}
