//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

type recoveryBeginResult struct {
	generation string
	err        error
}

func awaitRecoveryBegin(t *testing.T, done <-chan recoveryBeginResult) recoveryBeginResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("recovery begin did not finish")
		return recoveryBeginResult{}
	}
}

func sameRecovery(t *testing.T, ctx context.Context, store *identitypg.Store, want identity.RecoveryState) {
	t.Helper()
	got, err := store.RecoveryState(ctx)
	must(t, err)
	if got != want {
		t.Fatal("complete recovery state changed unexpectedly")
	}
}

func openRecovery(t *testing.T, ctx context.Context, store *identitypg.Store) string {
	t.Helper()
	generation, err := store.BeginRecovery(ctx)
	must(t, err)
	if !identity.ValidRecoveryID(generation) {
		t.Fatal("begin returned invalid recovery generation")
	}
	must(t, store.FinishRecovery(ctx, generation))
	return generation
}

func testSessionRecoveryAuthority(t *testing.T, ctx context.Context, a, b *pgxpool.Pool) {
	h := identityHasher(t, false)
	first, err := identitypg.New(a, h)
	must(t, err)
	second, err := identitypg.New(b, h)
	must(t, err)
	t.Run("explicit lifecycle rejects stale and repeated completion", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(800), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		old, err := first.BeginRecovery(ctx)
		must(t, err)
		current, err := second.BeginRecovery(ctx)
		must(t, err)
		if !identity.ValidRecoveryID(old) || !identity.ValidRecoveryID(current) || old == current {
			t.Fatal("independent recovery begins did not create different valid epochs")
		}
		closed := identity.RecoveryState{Generation: current}
		sameRecovery(t, ctx, first, closed)
		mustCredentialError(t, first.FinishRecovery(ctx, old), identity.ErrConflict)
		sameRecovery(t, ctx, second, closed)
		calls := 0
		op := func(context.Context, identity.SessionState) error { calls++; return nil }
		mustCredentialError(t, first.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: 1, RecoveryID: current}, op), identity.ErrRecovering)
		mustCredentialError(t, first.WithSessionCredentials(ctx, row.ID, row.Revision, current, op), identity.ErrRecovering)
		for _, invalid := range []string{"", "invalid", "0000000000000000000000000000000A"} {
			mustCredentialError(t, first.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: 1, RecoveryID: invalid}, op), identity.ErrInvalid)
			mustCredentialError(t, first.WithSessionCredentials(ctx, row.ID, row.Revision, invalid, op), identity.ErrInvalid)
			mustCredentialError(t, first.FinishRecovery(ctx, invalid), identity.ErrInvalid)
		}
		mustCredentialError(t, first.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: 1, RecoveryID: current}, nil), identity.ErrInvalid)
		if calls != 0 {
			t.Fatal("closed/invalid recovery invoked operation")
		}
		must(t, first.FinishRecovery(ctx, current))
		mustCredentialError(t, second.FinishRecovery(ctx, current), identity.ErrConflict)
		sameRecovery(t, ctx, second, identity.RecoveryState{Generation: current, Enabled: true})
		mustCredentialError(t, first.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: 1, RecoveryID: old}, op), identity.ErrConflict)
		mustCredentialError(t, first.WithSessionCredentials(ctx, row.ID, row.Revision, old, op), identity.ErrConflict)
		op = func(_ context.Context, state identity.SessionState) error {
			calls++
			if state != (identity.SessionState{State: row.State, RecoveryID: current}) {
				return errors.New("session operation received wrong authorities")
			}
			return nil
		}
		must(t, first.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: 1, RecoveryID: current}, op))
		must(t, second.WithSessionCredentials(ctx, row.ID, row.Revision, current, op))
		if calls != 2 {
			t.Fatal("current recovery did not admit exactly two valid operations")
		}
		sameIdentity(t, ctx, first, row)
	})
	t.Run("session account generation survives only rehash", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(801), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		epoch := openRecovery(t, ctx, first)
		fresh, err := second.RehashCAS(ctx, row.ID, row.Revision, identityRecord(t, h, "synthetic original"))
		must(t, err)
		calls := 0
		op := func(context.Context, identity.SessionState) error { calls++; return nil }
		mustCredentialError(t, first.WithSessionCredentials(ctx, row.ID, row.Revision, epoch, op), identity.ErrConflict)
		must(t, first.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: row.AuthGeneration, RecoveryID: epoch}, op))
		fresh, err = second.ReplacePasswordCAS(ctx, row.ID, fresh.Revision, identityRecord(t, h, "synthetic replacement"))
		must(t, err)
		mustCredentialError(t, first.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: row.AuthGeneration, RecoveryID: epoch}, op), identity.ErrConflict)
		rev, err := second.DisableCAS(ctx, row.ID, fresh.Revision)
		must(t, err)
		mustCredentialError(t, first.WithSession(ctx, row.ID, identity.SessionVersion{AuthGeneration: rev.AuthGeneration, RecoveryID: epoch}, op), identity.ErrConflict)
		mustCredentialError(t, first.WithSessionCredentials(ctx, row.ID, rev, epoch, op), identity.ErrConflict)
		if calls != 1 {
			t.Fatal("session account guard skipped revision/enable checks")
		}
	})
	t.Run("missing authority does not recreate itself", func(t *testing.T) {
		before, err := first.RecoveryState(ctx)
		must(t, err)
		_, err = a.Exec(ctx, `DELETE FROM identity_session_recovery`)
		must(t, err)
		_, err = first.RecoveryState(ctx)
		mustCredentialError(t, err, identity.ErrRecovering)
		generation, err := first.BeginRecovery(ctx)
		if generation != "" || !errors.Is(err, identity.ErrRecovering) {
			t.Fatal("missing authority initialized itself")
		}
		calls := 0
		mustCredentialError(t, first.WithSession(ctx, identityID(800), identity.SessionVersion{AuthGeneration: 1, RecoveryID: before.Generation}, func(context.Context, identity.SessionState) error { calls++; return nil }), identity.ErrRecovering)
		if calls != 0 {
			t.Fatal("missing authority admitted an operation")
		}
		// Explicit fixture repair preserves the exact prior row; no runtime API
		// has permission to recreate a missing singleton.
		_, err = a.Exec(ctx, `INSERT INTO identity_session_recovery(singleton,generation,enabled) VALUES(true,$1,$2)`, before.Generation, before.Enabled)
		must(t, err)
	})
	t.Run("update failure preserves the whole closed row", func(t *testing.T) {
		epoch, err := first.BeginRecovery(ctx)
		must(t, err)
		before := identity.RecoveryState{Generation: epoch}
		_, err = a.Exec(ctx, `CREATE FUNCTION reject_recovery_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic private recovery detail' USING ERRCODE='P0001'; END $$;
            CREATE TRIGGER reject_recovery_update AFTER UPDATE ON identity_session_recovery FOR EACH ROW EXECUTE FUNCTION reject_recovery_update()`)
		must(t, err)
		defer func() {
			_, err := a.Exec(ctx, `DROP TRIGGER reject_recovery_update ON identity_session_recovery; DROP FUNCTION reject_recovery_update()`)
			must(t, err)
		}()
		generation, err := first.BeginRecovery(ctx)
		if generation != "" || !errors.Is(err, identity.ErrStore) || err.Error() != identity.ErrStore.Error() {
			t.Fatal("failed begin leaked details or returned an epoch")
		}
		sameRecovery(t, ctx, second, before)
		err = second.FinishRecovery(ctx, epoch)
		if !errors.Is(err, identity.ErrStore) || err.Error() != identity.ErrStore.Error() {
			t.Fatal("failed finish hid storage error or exposed details")
		}
		sameRecovery(t, ctx, first, before)
	})
	t.Run("global and account ordering", func(t *testing.T) { testSessionRecoveryLocks(t, ctx, a, b, first, second, h) })
}
