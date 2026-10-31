//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func testSessionRecoveryLocks(t *testing.T, ctx context.Context, a, b *pgxpool.Pool, first, second *identitypg.Store, h *password.Hasher) {
	t.Run("two session guards share both authorities", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(810), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		epoch := openRecovery(t, ctx, first)
		gate, entered, release := verificationGate(t)
		op := func(call context.Context, state identity.SessionState) error {
			if state != (identity.SessionState{State: row.State, RecoveryID: epoch}) {
				return errors.New("wrong shared session authorities")
			}
			return gate(call)
		}
		one, two := make(chan error, 1), make(chan error, 1)
		go func() { one <- first.WithSessionCredentials(ctx, row.ID, row.Revision, epoch, op) }()
		go func() {
			two <- second.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}, op)
		}()
		awaitVerificationGate(t, entered)
		awaitVerificationGate(t, entered)
		release()
		must(t, awaitAuthority(t, one))
		must(t, awaitAuthority(t, two))
	})
	t.Run("global lock is retained while waiting for account", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(811), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		epoch := openRecovery(t, ctx, first)
		holder, err := a.Begin(ctx)
		must(t, err)
		defer holder.Rollback(ctx)
		var pid int32
		must(t, holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
		_, err = holder.Exec(ctx, `SELECT id FROM identity_credentials WHERE id=$1 FOR UPDATE`, row.ID)
		must(t, err)
		gate, entered, release := verificationGate(t)
		guard := make(chan error, 1)
		go func() {
			guard <- second.WithSessionCredentials(ctx, row.ID, row.Revision, epoch, func(call context.Context, _ identity.SessionState) error { return gate(call) })
		}()
		waitingGuard := blockedIdentityBackend(t, ctx, b, pid)
		begin := make(chan recoveryBeginResult, 1)
		go func() { generation, err := first.BeginRecovery(ctx); begin <- recoveryBeginResult{generation, err} }()
		waitIdentityBlocked(t, ctx, b, waitingGuard.pid)
		must(t, holder.Rollback(ctx))
		awaitVerificationGate(t, entered)
		sameRecovery(t, ctx, first, identity.RecoveryState{Generation: epoch, Enabled: true})
		release()
		must(t, awaitAuthority(t, guard))
		result := awaitRecoveryBegin(t, begin)
		must(t, result.err)
		if !identity.ValidRecoveryID(result.generation) || result.generation == epoch {
			t.Fatal("recovery begin failed to rotate after waiting for both locks")
		}
		sameRecovery(t, ctx, second, identity.RecoveryState{Generation: result.generation})
		sameIdentity(t, ctx, first, row)
	})
	for i, open := range []bool{false, true} {
		t.Run(map[bool]string{false: "guard waits for closed epoch", true: "guard waits for changed open epoch"}[open], func(t *testing.T) {
			row, err := first.Initialize(ctx, identityID(812+i), true, identityRecord(t, h, "synthetic original"))
			must(t, err)
			old := openRecovery(t, ctx, first)
			// The fixture directly holds an authority update to expose a real
			// waiter; it does not pretend to invoke Begin/Finish inside this Tx.
			next := identityID(900 + i)
			holder, err := a.Begin(ctx)
			must(t, err)
			defer holder.Rollback(ctx)
			var pid int32
			must(t, holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
			_, err = holder.Exec(ctx, `UPDATE identity_session_recovery SET generation=$1,enabled=$2 WHERE singleton`, next, open)
			must(t, err)
			calls := 0
			done := make(chan error, 1)
			go func() {
				done <- second.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: 1, RecoveryID: old}, func(context.Context, identity.SessionState) error { calls++; return nil })
			}()
			waitIdentityBlocked(t, ctx, b, pid)
			must(t, holder.Commit(ctx))
			want := identity.ErrRecovering
			if open {
				want = identity.ErrConflict
			}
			mustCredentialError(t, awaitAuthority(t, done), want)
			if calls != 0 {
				t.Fatal("recovery waiter used the old global snapshot")
			}
			sameRecovery(t, ctx, first, identity.RecoveryState{Generation: next, Enabled: open})
		})
	}
	t.Run("finish rechecks generation after a pending begin", func(t *testing.T) {
		old, err := first.BeginRecovery(ctx)
		must(t, err)
		next := identityID(902)
		holder, err := a.Begin(ctx)
		must(t, err)
		defer holder.Rollback(ctx)
		var pid int32
		must(t, holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
		_, err = holder.Exec(ctx, `UPDATE identity_session_recovery SET generation=$1,enabled=false WHERE singleton`, next)
		must(t, err)
		done := make(chan error, 1)
		go func() { done <- second.FinishRecovery(ctx, old) }()
		waitIdentityBlocked(t, ctx, b, pid)
		must(t, holder.Commit(ctx))
		mustCredentialError(t, awaitAuthority(t, done), identity.ErrConflict)
		sameRecovery(t, ctx, first, identity.RecoveryState{Generation: next})
	})
	t.Run("two finishes have one winner", func(t *testing.T) {
		epoch, err := first.BeginRecovery(ctx)
		must(t, err)
		done := make(chan error, 2)
		for _, store := range []*identitypg.Store{first, second} {
			go func() { done <- store.FinishRecovery(ctx, epoch) }()
		}
		wins := 0
		for range 2 {
			err := awaitAuthority(t, done)
			if err == nil {
				wins++
			} else {
				mustCredentialError(t, err, identity.ErrConflict)
			}
		}
		if wins != 1 {
			t.Fatal("recovery completion was not conditional and single-use")
		}
		sameRecovery(t, ctx, first, identity.RecoveryState{Generation: epoch, Enabled: true})
	})
	t.Run("canceled begin does not retry after backend cleanup", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(814), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		epoch := openRecovery(t, ctx, first)
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		generation, err := first.BeginRecovery(canceled)
		if generation != "" || !errors.Is(err, context.Canceled) {
			t.Fatal("pre-canceled begin returned an epoch")
		}
		mustCredentialError(t, first.FinishRecovery(canceled, epoch), context.Canceled)
		gate, entered, release := verificationGate(t)
		guard := make(chan error, 1)
		go func() {
			guard <- first.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}, func(call context.Context, _ identity.SessionState) error { return gate(call) })
		}()
		awaitVerificationGate(t, entered)
		owner := authorityBackend(t, ctx, b)
		call, stop := context.WithCancel(ctx)
		defer stop()
		begin := make(chan recoveryBeginResult, 1)
		go func() { generation, err := second.BeginRecovery(call); begin <- recoveryBeginResult{generation, err} }()
		backend := blockedIdentityBackend(t, ctx, b, owner.pid)
		stop()
		result := awaitRecoveryBegin(t, begin)
		if result.generation != "" || !errors.Is(result.err, identity.ErrStore) || !errors.Is(result.err, context.Canceled) {
			t.Fatal("canceled begin lost uncertain storage classification")
		}
		// Only this exact backend-exit barrier proves no delayed UPDATE can run
		// when the global lock is released; cancellation alone would not.
		waitIdentityBackendGone(t, ctx, b, backend)
		release()
		must(t, awaitAuthority(t, guard))
		sameRecovery(t, ctx, first, identity.RecoveryState{Generation: epoch, Enabled: true})
	})
	t.Run("epoch captured before verification cannot be relabeled", func(t *testing.T) {
		testRecoveryCandidate(t, ctx, first, second, h)
	})
	t.Run("single-row recovery query plans", func(t *testing.T) { testRecoveryPlans(t, ctx, a) })
}
