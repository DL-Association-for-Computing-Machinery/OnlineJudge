package identity

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

func TestPersonalCredentialGateInvalidAndPreCanceled(t *testing.T) {
	f := newPersonalGateFixture(t, false)
	op := func(context.Context, SessionState) error { t.Fatal("invalid request invoked operation"); return nil }
	for _, g := range []*PersonalCredentialGate{nil, {}} {
		if err := g.WithPersonalCredentials(context.Background(), f.email, f.secret, op); err != ErrInvalid {
			t.Fatal("zero gate did not fail closed")
		}
	}
	if g, err := NewPersonalCredentialGate(nil, &password.BoundedHasher{}); g != nil || err != ErrInvalid {
		t.Fatal("nil store accepted")
	}
	if g, err := NewPersonalCredentialGate(f.store, nil); g != nil || err != ErrInvalid {
		t.Fatal("nil password budget accepted")
	}
	var absent context.Context
	if f.gate.WithPersonalCredentials(absent, f.email, f.secret, op) != ErrInvalid ||
		f.gate.WithPersonalCredentials(context.Background(), Email{}, f.secret, op) != ErrInvalid ||
		f.gate.WithPersonalCredentials(context.Background(), f.email, f.secret, nil) != ErrInvalid {
		t.Fatal("invalid boundary input accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.gate.WithPersonalCredentials(ctx, f.email, f.secret, op); !errors.Is(err, context.Canceled) || len(f.events) != 0 {
		t.Fatal("invalid or pre-canceled request started storage/password work")
	}
}

func TestPersonalCredentialGateCancellationStopsNextStage(t *testing.T) {
	for _, phase := range []string{"recovery", "lookup", "state"} {
		t.Run(phase, func(t *testing.T) {
			f := newPersonalGateFixture(t, false)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := []string{"recovery"}
			switch phase {
			case "recovery":
				recovery := f.store.recovery
				f.store.recovery = func(ctx context.Context) (RecoveryState, error) {
					state, err := recovery(ctx)
					cancel()
					return state, err
				}
			case "lookup":
				lookup := f.store.lookup
				f.store.lookup = func(ctx context.Context, e Email) (Personal, error) {
					p, err := lookup(ctx, e)
					cancel()
					return p, err
				}
				want = []string{"recovery", "lookup"}
			case "state":
				state := f.verificationFixture.store.state
				f.verificationFixture.store.state = func(ctx context.Context, id string) (State, error) {
					row, err := state(ctx, id)
					cancel()
					return row, err
				}
				want = []string{"recovery", "lookup", "get", "verify", "state"}
			}
			err := f.gate.WithPersonalCredentials(ctx, f.email, f.secret, func(context.Context, SessionState) error { t.Fatal("canceled workflow invoked operation"); return nil })
			if !errors.Is(err, context.Canceled) || !slices.Equal(f.events, want) {
				t.Fatalf("canceled workflow continued: events=%v err=%v", f.events, err)
			}
		})
	}
}
