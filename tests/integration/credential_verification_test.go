//go:build integration

package integration_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

// These application barriers stop before a real Store call, outside a database
// transaction. They do not claim PostgreSQL lock waiting or mock password work.
type verificationBarrierStore struct {
	*identitypg.Store
	beforeRehash func(context.Context) error
	beforeState  func(context.Context) error
	afterRehash  func()
}

func (s *verificationBarrierStore) RehashCAS(ctx context.Context, id string, rev identity.Revision, record password.Record) (identity.Snapshot, error) {
	if s.beforeRehash != nil {
		if err := s.beforeRehash(ctx); err != nil {
			return identity.Snapshot{}, err
		}
	}
	row, err := s.Store.RehashCAS(ctx, id, rev, record)
	if err == nil && s.afterRehash != nil {
		s.afterRehash()
		return identity.Snapshot{}, errors.Join(identity.ErrStore, context.Canceled)
	}
	return row, err
}

func (s *verificationBarrierStore) State(ctx context.Context, id string) (identity.State, error) {
	if s.beforeState != nil {
		if err := s.beforeState(ctx); err != nil {
			return identity.State{}, err
		}
	}
	return s.Store.State(ctx, id)
}

type verificationOutcome struct {
	result identity.CredentialVerification
	err    error
}

func startVerification(v *identity.CredentialVerifier, ctx context.Context, id, secret string) <-chan verificationOutcome {
	out := make(chan verificationOutcome, 1)
	go func() {
		result, err := v.VerifyPassword(ctx, id, []byte(secret))
		out <- verificationOutcome{result, err}
	}()
	return out
}

func awaitVerification(t *testing.T, out <-chan verificationOutcome) verificationOutcome {
	t.Helper()
	select {
	case result := <-out:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("credential verification did not finish")
		return verificationOutcome{}
	}
}

func verificationGate(t *testing.T) (func(context.Context) error, <-chan struct{}, func()) {
	t.Helper()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	var opened atomic.Bool
	open := func() {
		if opened.CompareAndSwap(false, true) {
			close(release)
		}
	}
	t.Cleanup(open)
	return func(ctx context.Context) error {
		entered <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, entered, open
}

func awaitVerificationGate(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("credential workflow did not reach barrier")
	}
}

func expectVerificationFailure(t *testing.T, got verificationOutcome, want error) {
	t.Helper()
	if !errors.Is(got.err, want) || got.result != (identity.CredentialVerification{}) {
		t.Fatalf("expected zero result and %v, got error=%v result_zero=%v", want, got.err, got.result == (identity.CredentialVerification{}))
	}
}

func testCredentialVerification(t *testing.T, ctx context.Context, a, b *pgxpool.Pool) {
	h := identityHasher(t, false)
	first, err := identitypg.New(a, h)
	must(t, err)
	second, err := identitypg.New(b, h)
	must(t, err)
	old := identityHasher(t, true)
	oldStore, err := identitypg.New(a, old)
	must(t, err)
	budget, err := password.NewBounded(h, password.AdmissionLimits{MaxInFlight: 2, MaxWaiting: 2, MaxQueueWait: time.Second})
	must(t, err)
	newVerifier := func(store identity.CredentialStore) *identity.CredentialVerifier {
		v, err := identity.NewCredentialVerifier(store, budget)
		must(t, err)
		return v
	}
	t.Run("current and legacy password workflow", func(t *testing.T) {
		for i, legacy := range []bool{false, true} {
			writer, policy := first, h
			if legacy {
				writer, policy = oldStore, old
			}
			row, err := writer.Initialize(ctx, identityID(500+i), true, identityRecord(t, policy, "synthetic original"))
			must(t, err)
			v := newVerifier(second)
			_, err = v.VerifyPassword(ctx, row.ID, []byte("wrong password"))
			mustCredentialError(t, err, identity.ErrCredentials)
			sameIdentity(t, ctx, first, row)
			_, err = v.VerifyPassword(ctx, row.ID, nil)
			mustCredentialError(t, err, password.ErrPassword)
			sameIdentity(t, ctx, first, row)
			got, err := v.VerifyPassword(ctx, row.ID, []byte("synthetic original"))
			must(t, err)
			want := row.State
			if legacy {
				want.CredentialVersion++
			}
			if got != (identity.CredentialVerification{State: want, Upgraded: legacy}) {
				t.Fatal("wrong successful verification state")
			}
			stored, err := first.Get(ctx, row.ID)
			must(t, err)
			match, err := h.Verify([]byte("synthetic original"), stored.Record)
			must(t, err)
			if !match.Match || match.NeedsUpgrade || stored.State != want {
				t.Fatal("upgrade changed password or account generation")
			}
			if !legacy {
				sameIdentity(t, ctx, first, row)
			}
		}
	})
	t.Run("two stores compete for one legacy upgrade", func(t *testing.T) {
		row, err := oldStore.Initialize(ctx, identityID(502), true, identityRecord(t, old, "synthetic original"))
		must(t, err)
		gate, entered, release := verificationGate(t)
		one := startVerification(newVerifier(&verificationBarrierStore{Store: first, beforeRehash: gate}), ctx, row.ID, "synthetic original")
		two := startVerification(newVerifier(&verificationBarrierStore{Store: second, beforeRehash: gate}), ctx, row.ID, "synthetic original")
		awaitVerificationGate(t, entered)
		awaitVerificationGate(t, entered)
		release()
		success := 0
		for _, out := range []<-chan verificationOutcome{one, two} {
			got := awaitVerification(t, out)
			if got.err != nil {
				expectVerificationFailure(t, got, identity.ErrConflict)
			} else {
				success++
				want := row.State
				want.CredentialVersion++
				if got.result != (identity.CredentialVerification{State: want, Upgraded: true}) {
					t.Fatal("upgrade winner returned wrong state")
				}
			}
		}
		if success != 1 {
			t.Fatal("legacy upgrade did not have exactly one winner")
		}
		stored, err := first.Get(ctx, row.ID)
		must(t, err)
		match, err := h.Verify([]byte("synthetic original"), stored.Record)
		must(t, err)
		if stored.Revision != (identity.Revision{CredentialVersion: 2, AuthGeneration: 1}) || !stored.Enabled || !match.Match || match.NeedsUpgrade {
			t.Fatal("competing upgrade changed final password or revocation facts")
		}
	})
	t.Run("concurrent authority changes", func(t *testing.T) {
		testVerificationAuthorityChanges(t, ctx, first, second, oldStore, h, old, newVerifier)
	})
	t.Run("committed upgrade then synthetic uncertain return", func(t *testing.T) {
		row, err := oldStore.Initialize(ctx, identityID(520), true, identityRecord(t, old, "synthetic original"))
		must(t, err)
		call, cancel := context.WithCancel(ctx)
		defer cancel()
		var writes atomic.Int32
		wrapper := &verificationBarrierStore{Store: first, afterRehash: func() { writes.Add(1); cancel() }}
		got, err := newVerifier(wrapper).VerifyPassword(call, row.ID, []byte("synthetic original"))
		if got != (identity.CredentialVerification{}) || !errors.Is(err, identity.ErrStore) || !errors.Is(err, context.Canceled) || writes.Load() != 1 {
			t.Fatal("uncertain committed upgrade lost classification or retried")
		}
		stored, err := second.Get(ctx, row.ID)
		must(t, err)
		match, err := h.Verify([]byte("synthetic original"), stored.Record)
		must(t, err)
		if !match.Match || match.NeedsUpgrade || stored.Revision != (identity.Revision{CredentialVersion: 2, AuthGeneration: 1}) || !stored.Enabled {
			t.Fatal("failed verification compensated a committed upgrade")
		}
	})
}

func mustCredentialError(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("credential error=%v, want %v", got, want)
	}
}
