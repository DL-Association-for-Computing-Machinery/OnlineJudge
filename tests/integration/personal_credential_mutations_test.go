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

func testPersonalCredentialMutations(t *testing.T, ctx context.Context, pool *pgxpool.Pool, first, second *identitypg.Store, budget *password.BoundedHasher, h *password.Hasher) {
	old := identityHasher(t, true)
	writer, err := identitypg.New(pool, old)
	must(t, err)
	initialize := func(n int) identity.Personal {
		row, err := writer.InitializePersonal(ctx, identityID(n), personalEmail(t, fmt.Sprintf("credential-legacy-%d@example.invalid", n)), true, identityRecord(t, old, "synthetic personal password"))
		must(t, err)
		return row
	}
	t.Run("legacy upgrade uses the new credential version", func(t *testing.T) {
		row := initialize(2030)
		epoch := openRecovery(t, ctx, first)
		g, err := identity.NewPersonalCredentialGate(second, budget)
		must(t, err)
		calls := 0
		must(t, g.WithPersonalCredentials(ctx, row.Email, []byte("synthetic personal password"), func(_ context.Context, got identity.SessionState) error {
			calls++
			want := row.State
			want.CredentialVersion++
			if got.State != want || got.RecoveryID != epoch {
				return errors.New("upgraded credential gate used stale mapping revisions")
			}
			return nil
		}))
		snapshot := initialSnapshot(t, ctx, first, row.ID)
		class, err := h.CheckRecord(snapshot.Record)
		must(t, err)
		if class != password.CurrentRecord || snapshot.CredentialVersion != 2 || snapshot.AuthGeneration != 1 || calls != 1 {
			t.Fatal("same-password upgrade did not preserve authority and exact gate revision")
		}
	})
	for i, phase := range []string{"before upgrade", "after upgrade", "before guard"} {
		for j, disable := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "/replace", true: "/disable"}[disable], func(t *testing.T) {
				row := initialize(2040 + 2*i + j)
				openRecovery(t, ctx, first)
				barrier, entered, release := verificationGate(t)
				store := &personalCredentialBarrierStore{Store: first}
				switch phase {
				case "before upgrade":
					store.beforeRehash = barrier
				case "after upgrade":
					store.beforeState = barrier
				case "before guard":
					store.beforeGuard = barrier
				}
				g, err := identity.NewPersonalCredentialGate(store, budget)
				must(t, err)
				var calls atomic.Int32
				out := startPersonalCredentials(g, ctx, row.Email, func(context.Context, identity.SessionState) error { calls.Add(1); return nil })
				awaitVerificationGate(t, entered)
				before := initialSnapshot(t, ctx, second, row.ID)
				if disable {
					_, err = second.DisableCAS(ctx, row.ID, before.Revision)
				} else {
					_, err = second.ReplacePasswordCAS(ctx, row.ID, before.Revision, identityRecord(t, h, "synthetic changed password"))
				}
				must(t, err)
				changed := initialSnapshot(t, ctx, second, row.ID)
				release()
				mustCredentialError(t, awaitAuthority(t, out), identity.ErrConflict)
				if calls.Load() != 0 || store.recoveryReads.Load() != 1 {
					t.Fatal("changed credentials were retried or invoked")
				}
				sameIdentity(t, ctx, first, changed)
			})
		}
	}
	t.Run("unknown committed upgrade is not retried or compensated", func(t *testing.T) {
		row := initialize(2050)
		openRecovery(t, ctx, first)
		call, cancel := context.WithCancel(ctx)
		defer cancel()
		store := &personalCredentialBarrierStore{Store: first, afterRehash: cancel}
		g, err := identity.NewPersonalCredentialGate(store, budget)
		must(t, err)
		calls := 0
		err = g.WithPersonalCredentials(call, row.Email, []byte("synthetic personal password"), func(context.Context, identity.SessionState) error { calls++; return nil })
		if !errors.Is(err, identity.ErrStore) || !errors.Is(err, context.Canceled) || errors.Is(err, identity.ErrGuardedOperation) || calls != 0 || store.rehashCalls.Load() != 1 {
			t.Fatal("unknown upgrade lost classification, retried, or invoked callback")
		}
		snapshot := initialSnapshot(t, ctx, second, row.ID)
		if snapshot.CredentialVersion != 2 || snapshot.AuthGeneration != 1 {
			t.Fatal("committed upgrade was undone or repeated")
		}
	})
}
