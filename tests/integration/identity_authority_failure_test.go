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

func testAuthorityFailures(t *testing.T, ctx context.Context, a, b *pgxpool.Pool, first, second *identitypg.Store, h *password.Hasher) {
	t.Run("canceled lock waiter never starts operation", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(630), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		holder, err := a.Begin(ctx)
		must(t, err)
		defer holder.Rollback(ctx)
		var pid int32
		must(t, holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
		_, err = holder.Exec(ctx, `SELECT id FROM identity_credentials WHERE id=$1 FOR UPDATE`, row.ID)
		must(t, err)
		call, cancel := context.WithCancel(ctx)
		defer cancel()
		var calls atomic.Int32
		done := make(chan error, 1)
		go func() {
			done <- second.WithAccount(call, row.ID, row.AuthGeneration, func(context.Context, identity.State) error { calls.Add(1); return nil })
		}()
		backend := blockedIdentityBackend(t, ctx, b, pid)
		cancel()
		err = awaitAuthority(t, done)
		if !errors.Is(err, context.Canceled) || errors.Is(err, identity.ErrGuardedOperation) || calls.Load() != 0 {
			t.Fatal("canceled lock waiter started operation or lost cancellation")
		}
		// Keep the holder until this exact canceled backend is gone, rather than
		// treating a client cancellation as proof of server-side cleanup.
		waitIdentityBackendGone(t, ctx, b, backend)
		must(t, holder.Rollback(ctx))
		sameIdentity(t, ctx, first, row)
	})
	t.Run("running callback cancellation is cooperative", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(631), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		call, cancel := context.WithCancel(ctx)
		defer cancel()
		gate, entered, release := verificationGate(t)
		var calls atomic.Int32
		done := make(chan error, 1)
		go func() {
			done <- first.WithAccount(call, row.ID, row.AuthGeneration, func(context.Context, identity.State) error {
				calls.Add(1)
				// Deliberately ignore the provided context to expose the API's
				// cooperative limit; production callbacks must not do this.
				return gate(context.Background())
			})
		}()
		awaitVerificationGate(t, entered)
		backend := authorityBackend(t, ctx, b)
		cancel()
		changed := make(chan error, 1)
		go func() { _, err := second.DisableCAS(ctx, row.ID, row.Revision); changed <- err }()
		waitIdentityBlocked(t, ctx, b, backend.pid)
		select {
		case <-done:
			t.Fatal("cancellation pretended to stop a running callback")
		default:
		}
		release()
		err = awaitAuthority(t, done)
		if !errors.Is(err, identity.ErrGuardedOperation) || !errors.Is(err, context.Canceled) || calls.Load() != 1 {
			t.Fatal("canceled running callback lost uncertain-outcome classification")
		}
		must(t, awaitAuthority(t, changed))
		want := row
		want.Enabled, want.AuthGeneration = false, 2
		sameIdentity(t, ctx, first, want)
	})
	t.Run("lost backend permits late callback after replacement", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(640), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		gate, entered, release := verificationGate(t)
		var calls, effects atomic.Int32
		done := make(chan error, 1)
		go func() {
			done <- first.WithCredentials(ctx, row.ID, row.Revision, func(call context.Context, state identity.State) error {
				calls.Add(1)
				if state != row.State {
					return errors.New("unexpected authority callback state")
				}
				if err := gate(call); err != nil {
					return err
				}
				// A controlled external-effect marker, not a Redis implementation.
				effects.Add(1)
				return nil
			})
		}()
		awaitVerificationGate(t, entered)
		backend := authorityBackend(t, ctx, b)
		var killed bool
		must(t, b.QueryRow(ctx, `SELECT pg_terminate_backend($1)`, backend.pid).Scan(&killed))
		if !killed {
			t.Fatal("guard backend was not terminated")
		}
		waitIdentityBackendGone(t, ctx, b, backend)
		fresh, err := second.ReplacePasswordCAS(ctx, row.ID, row.Revision, identityRecord(t, h, "synthetic replacement"))
		must(t, err)
		select {
		case <-done:
			t.Fatal("backend loss incorrectly stopped a blocked callback")
		default:
		}
		if effects.Load() != 0 {
			t.Fatal("effect marker ran before replacement")
		}
		release()
		err = awaitAuthority(t, done)
		if !errors.Is(err, identity.ErrGuardedOperation) || !errors.Is(err, identity.ErrStore) ||
			err.Error() != identity.ErrGuardedOperation.Error() || calls.Load() != 1 || effects.Load() != 1 {
			t.Fatal("lost backend hid or retried the late external effect")
		}
		staleCalls := 0
		mustCredentialError(t, first.WithAccount(ctx, row.ID, row.AuthGeneration, func(context.Context, identity.State) error { staleCalls++; return nil }), identity.ErrConflict)
		if staleCalls != 0 {
			t.Fatal("old auth generation was usable after replacement")
		}
		sameIdentity(t, ctx, first, fresh)
	})
}
