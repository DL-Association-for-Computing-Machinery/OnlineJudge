package identity

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

type personalGateStore struct {
	*verificationStore
	recovery func(context.Context) (RecoveryState, error)
	lookup   func(context.Context, Email) (Personal, error)
	guard    func(context.Context, string, Revision, string, func(context.Context, SessionState) error) error
}

func (s *personalGateStore) RecoveryState(ctx context.Context) (RecoveryState, error) {
	return s.recovery(ctx)
}
func (s *personalGateStore) LookupPersonalByEmail(ctx context.Context, email Email) (Personal, error) {
	return s.lookup(ctx, email)
}
func (s *personalGateStore) WithSessionCredentials(ctx context.Context, id string, rev Revision, epoch string, op func(context.Context, SessionState) error) error {
	return s.guard(ctx, id, rev, epoch, op)
}

type personalGateFixture struct {
	*verificationFixture
	store *personalGateStore
	gate  *PersonalCredentialGate
	email Email
}

func newPersonalGateFixture(t *testing.T, legacy bool) *personalGateFixture {
	t.Helper()
	f := &personalGateFixture{verificationFixture: newVerificationFixture(t, legacy)}
	var err error
	f.email, err = ParseEmail("Person@Example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	f.store = &personalGateStore{verificationStore: f.verificationFixture.store}
	f.store.recovery = func(context.Context) (RecoveryState, error) {
		f.events = append(f.events, "recovery")
		return RecoveryState{Generation: verificationID, Enabled: true}, nil
	}
	f.store.lookup = func(_ context.Context, email Email) (Personal, error) {
		f.events = append(f.events, "lookup")
		if email.Key() != f.email.Key() {
			t.Fatal("wrong email lookup")
		}
		// Deliberately stale metadata is not the credential verification result.
		return Personal{State: State{ID: verificationID, Revision: Revision{CredentialVersion: 1, AuthGeneration: 1}}, Email: f.email}, nil
	}
	f.store.guard = func(ctx context.Context, id string, rev Revision, epoch string, op func(context.Context, SessionState) error) error {
		f.events = append(f.events, "guard")
		if id != verificationID || rev != f.row.Revision || epoch != verificationID {
			t.Fatal("guard did not receive captured epoch and exact verified revisions")
		}
		return op(ctx, SessionState{State: f.row.State, RecoveryID: epoch})
	}
	// The existing verifier fixture supplies deterministic password-stage hooks.
	// Real constructor, bounded KDF and PostgreSQL integration are tested separately.
	f.gate = &PersonalCredentialGate{store: f.store, verifier: f.v}
	return f
}

func TestPersonalCredentialGateUsesCapturedEpochAndVerifiedRevisions(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "upgraded legacy"}[legacy], func(t *testing.T) {
			f := newPersonalGateFixture(t, legacy)
			email, err := ParseEmail("person@EXAMPLE.INVALID")
			if err != nil {
				t.Fatal(err)
			}
			err = f.gate.WithPersonalCredentials(context.Background(), email, f.secret, func(_ context.Context, state SessionState) error {
				f.events = append(f.events, "operation")
				if state.State != f.row.State || state.RecoveryID != verificationID {
					t.Fatal("operation did not receive guarded authority")
				}
				return nil
			})
			want := []string{"recovery", "lookup", "get", "verify", "state", "guard", "operation"}
			if legacy {
				want = []string{"recovery", "lookup", "get", "verify", "create", "rehash", "state", "guard", "operation"}
			}
			if err != nil || !slices.Equal(f.events, want) {
				t.Fatalf("credential stages changed: events=%v err=%v", f.events, err)
			}
		})
	}
}

func TestPersonalCredentialGateRecoveryFailsBeforeLookupOrPassword(t *testing.T) {
	for _, state := range []RecoveryState{{}, {Generation: verificationID}, {Enabled: true}, {Generation: "bad", Enabled: true}, {Generation: "bad"}} {
		f := newPersonalGateFixture(t, false)
		f.store.recovery = func(context.Context) (RecoveryState, error) {
			f.events = append(f.events, "recovery")
			return state, nil
		}
		want := ErrRecovering
		if state.Generation == "bad" || state.Enabled {
			want = ErrStore
		}
		if err := f.gate.WithPersonalCredentials(context.Background(), f.email, f.secret, func(context.Context, SessionState) error { t.Fatal("closed gate invoked operation"); return nil }); err != want || !slices.Equal(f.events, []string{"recovery"}) {
			t.Fatal("closed or malformed recovery performed later work")
		}
	}
	for _, want := range []error{ErrRecovering, errors.Join(ErrStore, context.Canceled)} {
		f := newPersonalGateFixture(t, false)
		f.store.recovery = func(context.Context) (RecoveryState, error) { return RecoveryState{}, want }
		if err := f.gate.WithPersonalCredentials(context.Background(), f.email, f.secret, func(context.Context, SessionState) error { t.Fatal("missing recovery invoked operation"); return nil }); err != want || len(f.events) != 0 {
			t.Fatal("recovery error was replaced or ignored")
		}
	}
}

func TestPersonalCredentialGateRejectsBadMappings(t *testing.T) {
	for _, mutate := range []func(*Personal){
		func(p *Personal) { p.ID = "nickname" },
		func(p *Personal) { p.Revision = Revision{} },
		func(p *Personal) { p.Email = Email{} },
		func(p *Personal) { p.Email, _ = ParseEmail("other@example.invalid") },
	} {
		f := newPersonalGateFixture(t, false)
		lookup := f.store.lookup
		f.store.lookup = func(ctx context.Context, e Email) (Personal, error) {
			p, err := lookup(ctx, e)
			mutate(&p)
			return p, err
		}
		err := f.gate.WithPersonalCredentials(context.Background(), f.email, f.secret, func(context.Context, SessionState) error { t.Fatal("bad mapping invoked operation"); return nil })
		if err != ErrStore || !slices.Equal(f.events, []string{"recovery", "lookup"}) {
			t.Fatal("bad mapping reached password or guard")
		}
	}
}

func TestPersonalCredentialGatePreservesFailures(t *testing.T) {
	for _, phase := range []string{"missing email", "lookup", "verify", "upgrade", "guard"} {
		t.Run(phase, func(t *testing.T) {
			f := newPersonalGateFixture(t, true)
			want := errors.Join(ErrStore, context.Canceled)
			switch phase {
			case "missing email":
				f.store.lookup = func(context.Context, Email) (Personal, error) { return Personal{}, ErrNotFound }
				want = ErrCredentials
			case "lookup":
				f.store.lookup = func(context.Context, Email) (Personal, error) { return Personal{}, want }
			case "verify":
				f.work.verify = func(context.Context, []byte, password.Record) (password.Verification, error) {
					return password.Verification{}, password.ErrBusy
				}
				want = password.ErrBusy
			case "upgrade":
				rehash := f.verificationFixture.store.rehash
				f.verificationFixture.store.rehash = func(ctx context.Context, id string, rev Revision, r password.Record) (Snapshot, error) {
					_, _ = rehash(ctx, id, rev, r)
					return Snapshot{}, want
				}
			case "guard":
				f.store.guard = func(context.Context, string, Revision, string, func(context.Context, SessionState) error) error {
					return want
				}
			}
			err := f.gate.WithPersonalCredentials(context.Background(), f.email, f.secret, func(context.Context, SessionState) error { t.Fatal("failed credentials invoked operation"); return nil })
			if err != want {
				t.Fatal("stage failure classification changed")
			}
			if phase == "upgrade" && f.row.CredentialVersion != 5 {
				t.Fatal("unknown committed upgrade was compensated")
			}
		})
	}
	f := newPersonalGateFixture(t, false)
	want := errors.Join(ErrGuardedOperation, ErrStore, context.Canceled)
	f.store.guard = func(ctx context.Context, _ string, _ Revision, _ string, op func(context.Context, SessionState) error) error {
		_ = op(ctx, SessionState{})
		return want
	}
	calls := 0
	if err := f.gate.WithPersonalCredentials(context.Background(), f.email, f.secret, func(context.Context, SessionState) error { calls++; return errors.New("synthetic callback detail") }); err != want || calls != 1 {
		t.Fatal("guarded unknown effect was retried or reclassified")
	}
}
