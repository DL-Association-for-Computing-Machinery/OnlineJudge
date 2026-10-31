//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

// Hooks pause at application boundaries, outside the final authority transaction.
// The embedded Store performs every real lookup, KDF persistence and lock check.
type personalCredentialBarrierStore struct {
	*identitypg.Store
	afterLookup, beforeState, beforeGuard, beforeRehash func(context.Context) error
	afterRehash                                         func()
	recoveryReads                                       atomic.Int32
	rehashCalls                                         atomic.Int32
}

func (s *personalCredentialBarrierStore) RecoveryState(ctx context.Context) (identity.RecoveryState, error) {
	s.recoveryReads.Add(1)
	return s.Store.RecoveryState(ctx)
}
func (s *personalCredentialBarrierStore) LookupPersonalByEmail(ctx context.Context, email identity.Email) (identity.Personal, error) {
	p, err := s.Store.LookupPersonalByEmail(ctx, email)
	if err == nil && s.afterLookup != nil {
		err = s.afterLookup(ctx)
	}
	return p, err
}
func (s *personalCredentialBarrierStore) State(ctx context.Context, id string) (identity.State, error) {
	if s.beforeState != nil {
		if err := s.beforeState(ctx); err != nil {
			return identity.State{}, err
		}
	}
	return s.Store.State(ctx, id)
}
func (s *personalCredentialBarrierStore) RehashCAS(ctx context.Context, id string, rev identity.Revision, record password.Record) (identity.Snapshot, error) {
	s.rehashCalls.Add(1)
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
func (s *personalCredentialBarrierStore) WithSessionCredentials(ctx context.Context, id string, rev identity.Revision, epoch string, op func(context.Context, identity.SessionState) error) error {
	if s.beforeGuard != nil {
		if err := s.beforeGuard(ctx); err != nil {
			return err
		}
	}
	return s.Store.WithSessionCredentials(ctx, id, rev, epoch, op)
}

func startPersonalCredentials(g *identity.PersonalCredentialGate, ctx context.Context, email identity.Email, op func(context.Context, identity.SessionState) error) <-chan error {
	out := make(chan error, 1)
	go func() { out <- g.WithPersonalCredentials(ctx, email, []byte("synthetic personal password"), op) }()
	return out
}

func testPersonalCredentialGate(t *testing.T, ctx context.Context, a, b *pgxpool.Pool) {
	h := identityHasher(t, false)
	first, err := identitypg.New(a, h)
	must(t, err)
	second, err := identitypg.New(b, h)
	must(t, err)
	budget, err := password.NewBounded(h, password.AdmissionLimits{MaxInFlight: 2})
	must(t, err)
	newGate := func(s identity.PersonalCredentialStore) *identity.PersonalCredentialGate {
		g, err := identity.NewPersonalCredentialGate(s, budget)
		must(t, err)
		return g
	}
	email := personalEmail(t, "Credential.Path@Example.invalid")
	row, err := first.InitializePersonal(ctx, identityID(2000), email, true, identityRecord(t, h, "synthetic personal password"))
	must(t, err)
	t.Run("closed and missing recovery prevent credential work", func(t *testing.T) {
		_, err := first.BeginRecovery(ctx)
		must(t, err)
		store := &personalCredentialBarrierStore{Store: first, afterLookup: func(context.Context) error { t.Fatal("closed recovery reached email lookup"); return nil }}
		calls := 0
		op := func(context.Context, identity.SessionState) error { calls++; return nil }
		mustCredentialError(t, newGate(store).WithPersonalCredentials(ctx, email, []byte("synthetic personal password"), op), identity.ErrRecovering)
		before, err := first.RecoveryState(ctx)
		must(t, err)
		_, err = a.Exec(ctx, `DELETE FROM identity_session_recovery`)
		must(t, err)
		mustCredentialError(t, newGate(store).WithPersonalCredentials(ctx, email, []byte("synthetic personal password"), op), identity.ErrRecovering)
		_, err = a.Exec(ctx, `INSERT INTO identity_session_recovery(singleton,generation,enabled) VALUES(true,$1,$2)`, before.Generation, before.Enabled)
		must(t, err)
		if calls != 0 || store.recoveryReads.Load() != 2 {
			t.Fatal("closed/missing recovery invoked callback or retried")
		}
	})
	t.Run("real constructor resolves case and preserves separate verification fact", func(t *testing.T) {
		epoch := openRecovery(t, ctx, first)
		g := newGate(second)
		calls := 0
		err := g.WithPersonalCredentials(ctx, personalEmail(t, "credential.path@EXAMPLE.INVALID"), []byte("synthetic personal password"), func(_ context.Context, got identity.SessionState) error {
			calls++
			if got.State != row.State || got.RecoveryID != epoch {
				t.Fatal("real credential path lost checked state")
			}
			return nil
		})
		must(t, err)
		if calls != 1 || row.EmailVerified {
			t.Fatal("credential path changed email activation or callback count")
		}
		// This callback has no training/HTTP effects. Successful credentials do
		// not define the future product policy for unverified-user sessions.
		for _, request := range []struct {
			email  identity.Email
			secret []byte
			want   error
		}{
			{personalEmail(t, "missing-credential@example.invalid"), []byte("synthetic personal password"), identity.ErrCredentials},
			{email, []byte("wrong password"), identity.ErrCredentials},
			{email, nil, password.ErrPassword},
		} {
			mustCredentialError(t, g.WithPersonalCredentials(ctx, request.email, request.secret, func(context.Context, identity.SessionState) error { calls++; return nil }), request.want)
		}
		if calls != 1 {
			t.Fatal("failed credentials invoked callback")
		}
		_, err = first.DisableCAS(ctx, row.ID, row.Revision)
		must(t, err)
		mustCredentialError(t, g.WithPersonalCredentials(ctx, email, []byte("synthetic personal password"), func(context.Context, identity.SessionState) error { calls++; return nil }), identity.ErrCredentials)
		if calls != 1 {
			t.Fatal("disabled account invoked callback")
		}
	})
	for i, phase := range []string{"after lookup", "during verification", "before guard"} {
		t.Run("recovery "+phase, func(t *testing.T) {
			email := personalEmail(t, fmt.Sprintf("credential-recovery-%d@example.invalid", i))
			initial, err := first.InitializePersonal(ctx, identityID(2010+i), email, true, identityRecord(t, h, "synthetic personal password"))
			must(t, err)
			snapshot := initialSnapshot(t, ctx, first, initial.ID)
			before := openRecovery(t, ctx, first)
			barrier, entered, release := verificationGate(t)
			store := &personalCredentialBarrierStore{Store: first}
			switch phase {
			case "after lookup":
				store.afterLookup = barrier
			case "during verification":
				store.beforeState = barrier
			case "before guard":
				store.beforeGuard = barrier
			}
			var calls atomic.Int32
			op := func(context.Context, identity.SessionState) error { calls.Add(1); return nil }
			out := startPersonalCredentials(newGate(store), ctx, email, op)
			awaitVerificationGate(t, entered)
			after := openRecovery(t, ctx, second)
			if before == after {
				t.Fatal("recovery did not rotate epoch")
			}
			sameIdentity(t, ctx, second, snapshot)
			release()
			mustCredentialError(t, awaitAuthority(t, out), identity.ErrConflict)
			state, err := second.State(ctx, initial.ID)
			must(t, err)
			if state != initial.State || calls.Load() != 0 || store.recoveryReads.Load() != 1 {
				t.Fatal("old credential work was relabeled, invoked, or changed subject authority")
			}
			sameIdentity(t, ctx, second, snapshot)
			// A new invocation must restart password verification under the new epoch.
			must(t, newGate(second).WithPersonalCredentials(ctx, email, []byte("synthetic personal password"), op))
			if calls.Load() != 1 {
				t.Fatal("fresh credential workflow did not restart")
			}
		})
	}
	t.Run("upgrade and mutation boundaries", func(t *testing.T) { testPersonalCredentialMutations(t, ctx, a, first, second, budget, h) })
	t.Run("callback authority and cancellation", func(t *testing.T) { testPersonalCredentialCallback(t, ctx, b, first, second, budget, h) })
}

func initialSnapshot(t *testing.T, ctx context.Context, store *identitypg.Store, id string) identity.Snapshot {
	t.Helper()
	row, err := store.Get(ctx, id)
	must(t, err)
	return row
}
