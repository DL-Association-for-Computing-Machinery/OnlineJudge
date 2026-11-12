//go:build integration && linux

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
)

func TestRestoredReferenceMaintenanceFromArchive(t *testing.T) {
	f, claims, node, report := acceptedReportFixture(t, "AC")
	must(t, claims.ReportReference(f.ctx, node, report))
	f.limits(t, 23, 23)
	const selectedKey = "reference-restored-selected"
	selected, err := f.store.Enqueue(f.ctx, f.actor, selectedKey, f.draft.ID, f.catalog)
	must(t, err)
	prepareClaim(t, f, selectedKey)
	lease, err := claims.ClaimReference(f.ctx, node, problemreference.ClaimInput{
		RequestID: selected.ID, SourceID: f.draft.SourceID, PlanSHA256: f.want.SHA256,
	})
	must(t, err)
	oldIDs := []string{selected.ID}
	for i := range 21 {
		r, err := f.store.Enqueue(f.ctx, f.actor, fmt.Sprintf("reference-restored-other-%02d", i), f.draft.ID, f.catalog)
		must(t, err)
		oldIDs = append(oldIDs, r.ID)
	}
	current, err := f.store.Enqueue(f.ctx, f.actor, "reference-restored-current", f.draft.ID, f.catalog)
	must(t, err)
	slices.Sort(oldIDs)
	archive, schema := dumpSubmissionFixture(t, f.backlogFixture, t.TempDir())
	target := newSubmissionRestoreTarget(t, f.ctx, schema, archive)
	database := target.Config().ConnConfig.Database
	identities, err := identitypg.New(target, identityHasher(t, false))
	must(t, err)
	const wrongGate = "restored reference maintenance requires the confirmed closed recovery epoch"
	call := func(args ...string) (string, error) {
		return runRestoredReferenceCLI(t, f.ctx, target, schema, "OJ_RESTORED_DATABASE_URL", args...)
	}
	fail := func(want string, args ...string) {
		t.Helper()
		if got, err := call(args...); err == nil || got != want {
			t.Fatalf("expected maintenance refusal %q, got %q (%v)", want, got, err)
		}
	}
	fail(wrongGate, "list", database, f.actor.RecoveryID)
	epoch, err := identities.BeginRecovery(f.ctx)
	must(t, err)
	// An entry from the confirmed epoch is ineligible even if present while
	// closed. Normal admission cannot produce this fixture while closed.
	_, err = target.Exec(f.ctx, `UPDATE problem_reference_requests SET recovery_id=$1 WHERE id=$2`, epoch, current.ID)
	must(t, err)
	fail(wrongGate, "list", database, f.actor.RecoveryID)
	fail(referencepg.ErrRestoreDatabase.Error(), "list", "wrong-database", epoch)
	if got, err := runRestoredReferenceCLI(t, f.ctx, target, schema, "OJ_DATABASE_URL", "list", database, epoch); err == nil || got != "OJ_RESTORED_DATABASE_URL is required" {
		t.Fatal("maintenance fell back to the runtime DSN")
	}
	if got, err := call("cancel", database, epoch, selected.ID); err == nil || !strings.HasPrefix(got, "usage:") {
		t.Fatal("cancellation did not require explicit confirmation")
	}
	fail(problemreference.ErrNotFound.Error(), "cancel", database, epoch, current.ID, "--confirm-cancel")
	fail(referencepg.ErrRestoreTerminal.Error(), "cancel", database, epoch, f.request.ID, "--confirm-cancel")

	var listed []string
	after := ""
	for page := range 2 {
		args := []string{"list", database, epoch}
		if after != "" {
			args = append(args, after)
		}
		body, err := call(args...)
		must(t, err)
		var got referencepg.RestoredReferencePage
		must(t, json.Unmarshal([]byte(body), &got))
		wantCount := []int{20, 2}[page]
		if len(got.Requests) != wantCount || (page == 0 && got.NextAfter != got.Requests[19].ID) || (page == 1 && got.NextAfter != "") {
			t.Fatal("unexpected continuation for stale queued references")
		}
		for _, row := range got.Requests {
			if row.RecoveryID != f.actor.RecoveryID || row.State != problemreference.Queued {
				t.Fatal("list returned an ineligible reference")
			}
			listed = append(listed, row.ID)
		}
		after = got.NextAfter
		if strings.Contains(body, selectedKey) || strings.Contains(body, lease.Token) {
			t.Fatal("list exposed request or lease credentials")
		}
	}
	if !slices.Equal(listed, oldIDs) {
		t.Fatal("pagination did not visit the expected stale queued requests")
	}
	explainRestoredReferenceMaintenance(t, f.ctx, target, epoch, selected.ID)
	before := restoredReferenceEvidence(t, f.ctx, target, selected.ID, report.RequestID)
	queued, err := call("status", database, epoch, selected.ID)
	must(t, err)
	var row referencepg.RestoredReference
	must(t, json.Unmarshal([]byte(queued), &row))
	if row.ID != selected.ID || row.State != problemreference.Queued || row.CancelledAt != nil || row.RecoveryID != f.actor.RecoveryID {
		t.Fatal("dry inspection did not preserve the selected queued entry")
	}

	hold, err := target.Begin(f.ctx)
	must(t, err)
	defer func() { _ = hold.Rollback(f.ctx) }()
	_, err = hold.Exec(f.ctx, `SELECT singleton FROM problem_reference_request_policy FOR UPDATE`)
	must(t, err)
	var holder int32
	must(t, hold.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&holder))
	type outcome struct {
		body string
		err  error
	}
	cancelled, reopened := make(chan outcome, 1), make(chan error, 1)
	// Build the command on the test goroutine, then observe real lock order.
	cmd := restoredReferenceCLI(t, f.ctx, target, schema, "OJ_RESTORED_DATABASE_URL", "cancel", database, epoch, selected.ID, "--confirm-cancel")
	go func() {
		body, err := cmd.CombinedOutput()
		cancelled <- outcome{strings.TrimSpace(string(body)), err}
	}()
	backend := blockedIdentityBackend(t, f.ctx, target, holder)
	go func() { reopened <- identities.FinishRecovery(f.ctx, epoch) }()
	_ = blockedIdentityBackend(t, f.ctx, target, backend.pid)
	must(t, hold.Rollback(f.ctx))
	result := <-cancelled
	must(t, result.err)
	must(t, <-reopened)
	must(t, json.Unmarshal([]byte(result.body), &row))
	if row.ID != selected.ID || row.RecoveryID != f.actor.RecoveryID || row.State != problemreference.Cancelled || row.CancelledAt == nil {
		t.Fatal("maintenance did not cancel exactly the old request")
	}
	if got := restoredReferenceEvidence(t, f.ctx, target, selected.ID, report.RequestID); got != before {
		t.Fatal("cancellation altered the immutable request, plan, reservation, or historical report")
	}
	fail(wrongGate, "cancel", database, epoch, oldIDs[0], "--confirm-cancel")
	access, err := identitypg.NewProblemAccess(target)
	must(t, err)
	drafts, err := problempg.NewReferenceAccess(access)
	must(t, err)
	store, err := referencepg.New(target, drafts)
	must(t, err)
	actor := f.actor
	actor.RecoveryID = epoch
	fresh, err := store.Enqueue(f.ctx, actor, "reference-restored-new-epoch", f.draft.ID, f.catalog)
	must(t, err)
	if fresh.Authorization.Actor.RecoveryID != epoch || fresh.State != problemreference.Queued || fresh.ID == selected.ID {
		t.Fatal("freed slot did not admit a new request under the opened authority")
	}

	// Revisit the cancelled receipt in a new closed window. The original
	// cancellation time, not the time of the retry, remains authoritative.
	next, err := identities.BeginRecovery(f.ctx)
	must(t, err)
	for _, args := range [][]string{{"status", database, next, selected.ID}, {"cancel", database, next, selected.ID, "--confirm-cancel"}} {
		got, err := call(args...)
		must(t, err)
		if got != result.body {
			t.Fatal("read or retry changed the committed cancellation receipt")
		}
	}
	var total, active int
	must(t, target.QueryRow(f.ctx, `SELECT count(*),count(*) FILTER (WHERE state='queued') FROM problem_reference_requests`).Scan(&total, &active))
	if total != 25 || active != 23 {
		t.Fatal("maintenance did not preserve history or release exactly one queue slot")
	}
}

func TestRestoredReferenceMaintenanceRechecksGateAfterLockWait(t *testing.T) {
	f := newExpectedPlanFixture(t)
	epoch, err := f.identities.BeginRecovery(f.ctx)
	must(t, err)
	database := f.a.Config().ConnConfig.Database
	opening, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = opening.Rollback(f.ctx) }()
	var holder int32
	must(t, opening.QueryRow(f.ctx, `UPDATE identity_session_recovery SET enabled=true
		WHERE singleton AND generation=$1 AND NOT enabled RETURNING pg_backend_pid()`, epoch).Scan(&holder))
	result := make(chan error, 1)
	go func() {
		_, err := referencepg.CancelRestoredReference(f.ctx, f.b, database, epoch, f.request.ID)
		result <- err
	}()
	_ = blockedIdentityBackend(t, f.ctx, f.a, holder)
	must(t, opening.Commit(f.ctx))
	if err := <-result; !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("cancellation crossed an already-committed open: %v", err)
	}
	f.counts(t, 1, 1)

	epoch, err = f.identities.BeginRecovery(f.ctx)
	must(t, err)
	type cancellation struct {
		row referencepg.RestoredReference
		err error
	}
	done := make(chan cancellation, 2)
	for _, pool := range []*pgxpool.Pool{f.a, f.b} {
		go func() {
			row, err := referencepg.CancelRestoredReference(f.ctx, pool, database, epoch, f.request.ID)
			done <- cancellation{row, err}
		}()
	}
	a, b := <-done, <-done
	must(t, a.err)
	must(t, b.err)
	if a.row.State != problemreference.Cancelled || b.row.State != problemreference.Cancelled ||
		a.row.CancelledAt == nil || b.row.CancelledAt == nil || !a.row.CancelledAt.Equal(*b.row.CancelledAt) {
		t.Fatal("concurrent cancellation did not replay the same receipt")
	}
	f.counts(t, 1, 0)
}

func explainRestoredReferenceMaintenance(t *testing.T, ctx context.Context, pool *pgxpool.Pool, epoch, id string) {
	t.Helper()
	rows, err := pool.Query(ctx, `EXPLAIN (ANALYZE, BUFFERS) SELECT id, owner_subject_id, draft_id, source_id,
		recovery_id, state, created_at, cancelled_at FROM problem_reference_requests
		WHERE state='queued' AND recovery_id<>$1 AND id COLLATE "C">$2 ORDER BY id COLLATE "C" LIMIT $3`, epoch, "", 21)
	must(t, err)
	plan, err := pgx.CollectRows(rows, pgx.RowTo[string])
	must(t, err)
	t.Logf("restored reference listing:\n%s", strings.Join(plan, "\n"))
	rows, err = pool.Query(ctx, `EXPLAIN (ANALYZE, BUFFERS) SELECT id, owner_subject_id, draft_id, source_id,
		recovery_id, state, created_at, cancelled_at FROM problem_reference_requests WHERE id=$1 AND recovery_id<>$2 FOR UPDATE`, id, epoch)
	must(t, err)
	plan, err = pgx.CollectRows(rows, pgx.RowTo[string])
	must(t, err)
	t.Logf("restored reference selected lock:\n%s", strings.Join(plan, "\n"))
}

func restoredReferenceEvidence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, selected, reported string) string {
	t.Helper()
	var digest string
	must(t, pool.QueryRow(ctx, `SELECT encode(sha256(convert_to(jsonb_build_array(
		(SELECT to_jsonb(r)-'state'-'cancelled_at' FROM problem_reference_requests r WHERE r.id=$1),
		(SELECT to_jsonb(p) FROM problem_reference_expected_plans p WHERE p.request_id=$1),
		(SELECT to_jsonb(h) FROM problem_reference_dispatch_history h WHERE h.request_id=$1),
		(SELECT to_jsonb(c) FROM problem_reference_worker_claim c WHERE c.singleton),
		(SELECT to_jsonb(r) FROM problem_reference_worker_reports r WHERE r.request_id=$2)
	)::text,'UTF8')),'hex')`, selected, reported).Scan(&digest))
	return digest
}

func runRestoredReferenceCLI(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema, dsnVariable string, args ...string) (string, error) {
	t.Helper()
	output, err := restoredReferenceCLI(t, ctx, pool, schema, dsnVariable, args...).CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func restoredReferenceCLI(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schema, dsnVariable string, args ...string) *exec.Cmd {
	t.Helper()
	path, err := filepath.Abs("../../.local/bin/migrate")
	must(t, err)
	cmd := exec.CommandContext(ctx, path, append([]string{"restored-reference"}, args...)...)
	cmd.Env = []string{dsnVariable + "=" + pool.Config().ConnString() + " search_path=" + schema}
	return cmd
}
