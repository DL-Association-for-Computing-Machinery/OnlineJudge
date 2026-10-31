//go:build integration

package integration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func testRecoveryCandidate(t *testing.T, ctx context.Context, first, second *identitypg.Store, h *password.Hasher) {
	row, err := first.Initialize(ctx, identityID(815), true, identityRecord(t, h, "synthetic original"))
	must(t, err)
	before := openRecovery(t, ctx, first)
	budget, err := password.NewBounded(h, password.AdmissionLimits{MaxInFlight: 1})
	must(t, err)
	gate, entered, release := verificationGate(t)
	v, err := identity.NewCredentialVerifier(&verificationBarrierStore{Store: first, beforeState: gate}, budget)
	must(t, err)
	verification := startVerification(v, ctx, row.ID, "synthetic original")
	awaitVerificationGate(t, entered)
	// Recovery occurs during the VerifyPassword workflow, after its real KDF
	// and before its final State. Neither account revision changes.
	after := openRecovery(t, ctx, second)
	if before == after {
		t.Fatal("recovery epoch did not change")
	}
	release()
	verified := awaitVerification(t, verification)
	must(t, verified.err)
	if verified.result.State != row.State {
		t.Fatal("recovery unexpectedly changed account revisions")
	}
	calls := 0
	op := func(context.Context, identity.SessionState) error { calls++; return nil }
	mustCredentialError(t, second.WithSessionCredentials(ctx, row.ID, verified.result.Revision, before, op), identity.ErrConflict)
	if calls != 0 {
		t.Fatal("old verification crossed recovery despite unchanged account revisions")
	}
	// Start the entire workflow again under the new epoch, rather than attaching
	// a new epoch to the already-completed verification above.
	captured, err := second.RecoveryState(ctx)
	must(t, err)
	if !captured.Enabled || !identity.ValidRecoveryID(captured.Generation) {
		t.Fatal("password verification must start under a valid open epoch")
	}
	freshVerifier, err := identity.NewCredentialVerifier(second, budget)
	must(t, err)
	fresh, err := freshVerifier.VerifyPassword(ctx, row.ID, []byte("synthetic original"))
	must(t, err)
	must(t, first.WithSessionCredentials(ctx, row.ID, fresh.Revision, captured.Generation, op))
	if calls != 1 {
		t.Fatal("freshly restarted verification did not admit exactly once")
	}
	sameIdentity(t, ctx, first, row)
}

func testRecoveryPlans(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	tx, err := pool.Begin(ctx)
	must(t, err)
	defer tx.Rollback(ctx)
	queries := []string{
		`EXPLAIN (ANALYZE,BUFFERS) SELECT generation,enabled FROM identity_session_recovery WHERE singleton FOR SHARE`,
		`EXPLAIN (ANALYZE,BUFFERS) UPDATE identity_session_recovery SET generation=$1,enabled=false WHERE singleton RETURNING generation,enabled`,
		`EXPLAIN (ANALYZE,BUFFERS) UPDATE identity_session_recovery SET enabled=true WHERE singleton AND generation=$1 AND NOT enabled`,
	}
	for i, query := range queries {
		var args []any
		if i != 0 {
			args = []any{identityID(903)}
		}
		rows, err := tx.Query(ctx, query, args...)
		must(t, err)
		var plan strings.Builder
		for rows.Next() {
			var line string
			must(t, rows.Scan(&line))
			plan.WriteString(line + "\n")
		}
		must(t, rows.Err())
		rows.Close()
		if i == 0 && !strings.Contains(plan.String(), "LockRows") {
			t.Fatal("recovery locking plan omitted row locking")
		}
		t.Log(plan.String())
	}
	// The singleton constraint bounds this table to one row; a natural sequence
	// scan is appropriate. Do not force an index to manufacture a plan result.
}
