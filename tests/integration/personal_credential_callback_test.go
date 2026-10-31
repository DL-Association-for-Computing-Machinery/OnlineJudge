//go:build integration

package integration_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func testPersonalCredentialCallback(t *testing.T, ctx context.Context, b *pgxpool.Pool, first, second *identitypg.Store, budget *password.BoundedHasher, h *password.Hasher) {
	row, err := first.InitializePersonal(ctx, identityID(2060), personalEmail(t, "credential-callback@example.invalid"), true, identityRecord(t, h, "synthetic personal password"))
	must(t, err)
	g, err := identity.NewPersonalCredentialGate(first, budget)
	must(t, err)
	t.Run("callback holds recovery authority until it finishes", func(t *testing.T) {
		epoch := openRecovery(t, ctx, first)
		barrier, entered, release := verificationGate(t)
		var calls atomic.Int32
		out := startPersonalCredentials(g, ctx, row.Email, func(call context.Context, state identity.SessionState) error {
			calls.Add(1)
			if state.State != row.State || state.RecoveryID != epoch {
				return errors.New("wrong locked callback state")
			}
			return barrier(call)
		})
		awaitVerificationGate(t, entered)
		backend := authorityBackend(t, ctx, b)
		begin := make(chan recoveryBeginResult, 1)
		go func() { generation, err := second.BeginRecovery(ctx); begin <- recoveryBeginResult{generation, err} }()
		waitIdentityBlocked(t, ctx, b, backend.pid)
		sameRecovery(t, ctx, second, identity.RecoveryState{Generation: epoch, Enabled: true})
		release()
		must(t, awaitAuthority(t, out))
		result := awaitRecoveryBegin(t, begin)
		must(t, result.err)
		if result.generation == epoch || !identity.ValidRecoveryID(result.generation) || calls.Load() != 1 {
			t.Fatal("recovery did not wait for the actual credential callback")
		}
	})
	t.Run("callback errors preserve guarded effects and release locks", func(t *testing.T) {
		openRecovery(t, ctx, first)
		private := errors.New("synthetic private operation detail")
		calls := 0
		err := g.WithPersonalCredentials(ctx, row.Email, []byte("synthetic personal password"), func(context.Context, identity.SessionState) error { calls++; return private })
		if !errors.Is(err, identity.ErrGuardedOperation) || !errors.Is(err, private) || err.Error() != identity.ErrGuardedOperation.Error() || calls != 1 {
			t.Fatal("callback outcome was retried, leaked or reclassified")
		}
		_, err = second.BeginRecovery(ctx)
		must(t, err)
	})
	t.Run("cancellation before guard cannot start operation", func(t *testing.T) {
		openRecovery(t, ctx, first)
		call, cancel := context.WithCancel(ctx)
		defer cancel()
		store := &personalCredentialBarrierStore{Store: first, beforeGuard: func(context.Context) error { cancel(); return nil }}
		gate, err := identity.NewPersonalCredentialGate(store, budget)
		must(t, err)
		calls := 0
		err = gate.WithPersonalCredentials(call, row.Email, []byte("synthetic personal password"), func(context.Context, identity.SessionState) error { calls++; return nil })
		if !errors.Is(err, context.Canceled) || errors.Is(err, identity.ErrGuardedOperation) || calls != 0 {
			t.Fatal("pre-guard cancellation invoked callback")
		}
	})
	t.Run("cancellation inside callback keeps guarded classification", func(t *testing.T) {
		openRecovery(t, ctx, first)
		call, cancel := context.WithCancel(ctx)
		defer cancel()
		calls := 0
		err := g.WithPersonalCredentials(call, row.Email, []byte("synthetic personal password"), func(context.Context, identity.SessionState) error { calls++; cancel(); return nil })
		if !errors.Is(err, context.Canceled) || !errors.Is(err, identity.ErrGuardedOperation) || calls != 1 {
			t.Fatal("callback cancellation lost uncertain effects")
		}
		_, err = second.BeginRecovery(ctx)
		must(t, err)
	})
}
