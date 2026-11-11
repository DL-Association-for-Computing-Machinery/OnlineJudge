//go:build integration && linux

package integration_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestReferenceClaimReadsCancellationAfterWaiting(t *testing.T) {
	f, claims, node, input := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	tx, err := f.b.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `SELECT id FROM problem_reference_requests WHERE id=$1 FOR UPDATE`, input.RequestID)
	must(t, err)
	type outcome struct {
		claim problemreference.WorkerClaim
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		got, err := claims.ClaimReference(f.ctx, node, input)
		done <- outcome{got, err}
	}()
	waitBacklogBlocked(t, f.backlogFixture, tx.Conn().PgConn().PID(), f.appA)
	_, err = tx.Exec(f.ctx, `UPDATE problem_reference_requests SET state='cancelled',cancelled_at=clock_timestamp() WHERE id=$1`, input.RequestID)
	must(t, err)
	must(t, tx.Commit(f.ctx))
	out := <-done
	noReferenceClaim(t, out.claim, out.err, problemreference.ErrClaimUnavailable)
	var rows int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_reference_worker_claim`).Scan(&rows))
	if rows != 0 {
		t.Fatal("cancelled request got a reservation")
	}
}

func TestReferenceClaimDown11WaitsForInFlightWrite(t *testing.T) {
	f, _, node, input := claimFixture(t)
	_, err := f.provider.DownTo(f.ctx, 11)
	must(t, err)
	prepareClaim(t, f, expectedPlanKey)
	tx, err := f.b.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `LOCK TABLE problem_reference_worker_claim IN ACCESS EXCLUSIVE MODE`)
	must(t, err)
	claimed := make(chan error, 1)
	go func() {
		_, err := legacyReservation(f, node, input)
		claimed <- err
	}()
	claimPID := waitBacklogBlocked(t, f.backlogFixture, tx.Conn().PgConn().PID(), f.appA)
	down := make(chan error, 1)
	go func() { _, err := f.provider.DownTo(f.ctx, 10); down <- err }()
	watch, cancel := context.WithTimeout(f.ctx, 2*time.Second)
	defer cancel()
	for {
		var blocked bool
		must(t, f.a.QueryRow(watch, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE $1::integer=ANY(pg_blocking_pids(pid))
			AND query LIKE '%LOCK TABLE worker_nodes, problem_reference_expected_plans, problem_reference_worker_claim%')`, claimPID).Scan(&blocked))
		if blocked {
			break
		}
		select {
		case err := <-down:
			t.Fatalf("Down11 ended before the in-flight claim: %v", err)
		case <-watch.Done():
			t.Fatal("Down11 lock wait not observed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	must(t, tx.Commit(f.ctx))
	must(t, <-claimed)
	var pe *pgconn.PgError
	if err := <-down; !errors.As(err, &pe) || pe.Code != "P0001" || !strings.Contains(pe.Message, "cannot discard reference claim state") {
		t.Fatalf("Down11 did not preserve claim state: %v", err)
	}
	var version int64
	must(t, f.a.QueryRow(f.ctx, `SELECT max(version_id) FROM goose_db_version WHERE is_applied`).Scan(&version))
	if version != 11 {
		t.Fatalf("Down11 changed the schema to %d", version)
	}
}

func TestReferenceClaimLostCommitDoesNotAcknowledgeLease(t *testing.T) {
	f, clean, node, input := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	faultyStore, observed := lostReferenceCommitStore(t, f.referenceRequestFixture)
	faulty, err := referencepg.NewReferenceClaimer(faultyStore, submissionpg.LockReferenceNode, node.ID, f.request.Specification.Identity.SourceLockSHA256)
	must(t, err)
	got, err := faulty.ClaimReference(f.ctx, node, input)
	noReferenceClaim(t, got, err, problemreference.ErrCommitUnknown)
	select {
	case committed := <-observed:
		if !committed {
			t.Fatal("fixture did not lose an actual COMMIT reply")
		}
	case <-f.ctx.Done():
		t.Fatal("commit-loss fixture timed out")
	}
	got, err = clean.ClaimReference(f.ctx, node, input)
	noReferenceClaim(t, got, err, problemreference.ErrClaimUnavailable)
	var count int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_reference_worker_claim WHERE request_id=$1 AND lease_until>clock_timestamp()`, input.RequestID).Scan(&count))
	if count != 1 {
		t.Fatal("unknown outcome did not retain original reservation")
	}
}
