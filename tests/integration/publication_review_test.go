//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/jackc/pgx/v5"
)

func publicationApprovalFixture(t *testing.T) (reviewFixture, problemreference.ReviewEvidence) {
	t.Helper()
	f := newReviewFixture(t)
	f.grant(t, f.reader, 0, true)
	return f, assignReview(t, f, f.item.ID)
}

func seedPublicationApproval(t *testing.T, f reviewFixture, e problemreference.ReviewEvidence, snapshot, policy string) {
	t.Helper()
	// v14 cannot issue a passing approval. Exercise the publication gate
	// with historical rows without changing the production approval policy.
	digest := sha256.Sum256([]byte("onlinejudge/review-material/v1:" + f.actor.SubjectID +
		e.ImportRevision + e.FormalPlanSHA256 + e.ProfileSHA256 + e.EvidenceSHA256 + e.ComparisonPlan))
	_, err := f.a.Exec(f.ctx, `INSERT INTO problem_review_approvals
		(review_id, assignment_revision, reviewer_subject_id, reviewer_revision, request_key, material_sha256,
		 snapshot_sha256, policy_version, reason, auth_generation, recovery_id)
		VALUES ($1, $2, $3, 1, 'publication-approved-key', $4, $5, $6, 'fixture approval', $7, $8)`,
		e.ReviewID, e.AssignmentRevision, f.reader.SubjectID, hex.EncodeToString(digest[:]), snapshot, policy,
		f.reader.AuthGeneration, f.reader.RecoveryID)
	must(t, err)
}

func checkPublicationDenied(t *testing.T, f reviewFixture, e problemreference.ReviewEvidence, isolation pgx.TxIsoLevel, want error) {
	t.Helper()
	tx, err := f.b.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: isolation})
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	got, err := f.reviews.LockPublicationReview(f.ctx, tx, e.ReviewID, e.AssignmentRevision, e.Reviewer)
	requireReviewError(t, err, want)
	if got != (problemreference.ReviewApprovalReceipt{}) {
		t.Fatal("publication gate returned a receipt after rejection")
	}
}

func TestPublicationGateRechecksSavedApproval(t *testing.T) {
	f, e := publicationApprovalFixture(t)
	checkPublicationDenied(t, f, e, pgx.ReadCommitted, problemreference.ErrNotFound)
	seedPublicationApproval(t, f, e, currentPublicationSnapshot(t, f, e), problemreference.ReviewPolicyVersion)
	checkPublicationDenied(t, f, e, pgx.RepeatableRead, problemreference.ErrInvalid)
	wrongReviewer := e
	wrongReviewer.Reviewer = f.actor.SubjectID
	checkPublicationDenied(t, f, wrongReviewer, pgx.ReadCommitted, problemreference.ErrReviewChanged)
	checkPublicationDenied(t, f, e, pgx.ReadCommitted, problemreference.ErrReviewApprovalBlocked)

	// Even regranting the same reviewer does not restore an old approval.
	f.grant(t, f.reader, 1, false)
	checkPublicationDenied(t, f, e, pgx.ReadCommitted, identity.ErrReviewerRequired)
	f.grant(t, f.reader, 2, true)
	checkPublicationDenied(t, f, e, pgx.ReadCommitted, problemreference.ErrNotFound)
	next, err := f.reviews.SetAssignment(f.ctx, f.change(e.AssignmentRevision, f.reader.SubjectID, 3))
	must(t, err)
	checkPublicationDenied(t, f, e, pgx.ReadCommitted, problemreference.ErrReviewChanged)
	e.AssignmentRevision = next.Revision
	checkPublicationDenied(t, f, e, pgx.ReadCommitted, problemreference.ErrNotFound)
}

func currentPublicationSnapshot(t *testing.T, f reviewFixture, e problemreference.ReviewEvidence) string {
	t.Helper()
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: f.reader.SubjectID,
		AuthGeneration: f.reader.AuthGeneration, RecoveryID: f.reader.RecoveryID}}
	access, err := problemreference.NewReviewAccess(sessions, f.reviews)
	must(t, err)
	current, err := access.EligibilityForSubject(f.ctx, redisstore.Credential{}, f.reader.SubjectID, e.ReviewID)
	must(t, err)
	return current.SnapshotSHA256
}

func TestPublicationGateRejectsOldPolicyAndSnapshot(t *testing.T) {
	for _, policy := range []string{"oj.review-approval.old", problemreference.ReviewPolicyVersion} {
		t.Run(policy, func(t *testing.T) {
			f, e := publicationApprovalFixture(t)
			snapshot := strings.Repeat("f", 64)
			if policy != problemreference.ReviewPolicyVersion {
				snapshot = currentPublicationSnapshot(t, f, e)
			}
			seedPublicationApproval(t, f, e, snapshot, policy)
			checkPublicationDenied(t, f, e, pgx.ReadCommitted, problemreference.ErrReviewChanged)
		})
	}
}

func TestPublicationGateRejectsMismatchedMaterial(t *testing.T) {
	f, e := publicationApprovalFixture(t)
	seedPublicationApproval(t, f, e, currentPublicationSnapshot(t, f, e), problemreference.ReviewPolicyVersion)
	_, err := f.a.Exec(f.ctx, `UPDATE problem_review_approvals SET material_sha256=repeat('f',64)
		WHERE review_id=$1 AND assignment_revision=$2`, e.ReviewID, e.AssignmentRevision)
	must(t, err)
	checkPublicationDenied(t, f, e, pgx.ReadCommitted, problemreference.ErrStore)
}

func TestPublicationGateApprovalLookupPlan(t *testing.T) {
	f, e := publicationApprovalFixture(t)
	seedPublicationApproval(t, f, e, strings.Repeat("f", 64), problemreference.ReviewPolicyVersion)
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	// The small fixture favors a sequential scan. Verify index eligibility,
	// not planner choices or latency for a production-sized ledger.
	_, err = tx.Exec(f.ctx, "SET LOCAL enable_seqscan=off")
	must(t, err)
	rows, err := tx.Query(f.ctx, `EXPLAIN (ANALYZE, BUFFERS)
		SELECT * FROM problem_review_approvals WHERE review_id=$1 AND assignment_revision=$2`,
		e.ReviewID, e.AssignmentRevision)
	must(t, err)
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		must(t, rows.Scan(&line))
		plan.WriteString(line + "\n")
	}
	must(t, rows.Err())
	if !strings.Contains(plan.String(), "problem_review_approvals_pkey") {
		t.Fatal("exact approval lookup cannot use the existing primary key")
	}
	t.Log(plan.String())
}

func TestPublicationGateDoesNotReviveApprovalAfterRecovery(t *testing.T) {
	f, e := publicationApprovalFixture(t)
	seedPublicationApproval(t, f, e, strings.Repeat("f", 64), problemreference.ReviewPolicyVersion)
	generation := openRecovery(t, f.ctx, f.identities)
	checkPublicationDenied(t, f, e, pgx.ReadCommitted, identity.ErrConflict)
	f.reader.RecoveryID = generation
	_, err := f.reviews.ReturnReview(f.ctx, f.reader, e.ReviewID, returnIntent(e, "return-old-publication-review"))
	requireReviewError(t, err, problemreference.ErrReviewAlreadyApproved)

	next, err := f.reviews.SetAssignment(f.ctx, f.change(e.AssignmentRevision, f.reader.SubjectID, 1))
	must(t, err)
	e.AssignmentRevision = next.Revision
	checkPublicationDenied(t, f, e, pgx.ReadCommitted, problemreference.ErrNotFound)
	_, err = f.reviews.ReturnReview(f.ctx, f.reader, e.ReviewID, returnIntent(e, "return-new-publication-review"))
	must(t, err)
}

func TestPublicationGateRejectsApprovalFromOldAccountGeneration(t *testing.T) {
	f, e := publicationApprovalFixture(t)
	seedPublicationApproval(t, f, e, currentPublicationSnapshot(t, f, e), problemreference.ReviewPolicyVersion)
	state, err := f.identities.State(f.ctx, f.reader.SubjectID)
	must(t, err)
	changed, err := f.identities.ReplacePasswordCAS(f.ctx, f.reader.SubjectID, state.Revision,
		identityRecord(t, identityHasher(t, false), "synthetic replacement password"))
	must(t, err)
	if changed.AuthGeneration == f.reader.AuthGeneration {
		t.Fatal("password replacement did not change the account generation")
	}
	checkPublicationDenied(t, f, e, pgx.ReadCommitted, identity.ErrConflict)
}

func TestPublicationGateWaitsForAssignmentBeforeReadingCurrentRevision(t *testing.T) {
	f, e := publicationApprovalFixture(t)
	seedPublicationApproval(t, f, e, strings.Repeat("f", 64), problemreference.ReviewPolicyVersion)
	writer, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = writer.Rollback(f.ctx) }()
	var ownerPID uint32
	must(t, writer.QueryRow(f.ctx, `SELECT pg_backend_pid() FROM problem_review_submissions WHERE id=$1 FOR UPDATE`,
		e.ReviewID).Scan(&ownerPID))
	reader, err := f.b.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	must(t, err)
	finished := make(chan struct{})
	defer func() { _ = reader.Rollback(f.ctx) }()
	waitCtx, cancel := context.WithCancel(f.ctx)
	defer func() { cancel(); <-finished }()
	out := make(chan error, 1)
	go func() {
		defer close(finished)
		_, err := f.reviews.LockPublicationReview(waitCtx, reader, e.ReviewID, e.AssignmentRevision, e.Reviewer)
		out <- err
	}()
	waitBacklogBlocked(t, f.backlogFixture, ownerPID, f.appB)
	// Reassign the same reviewer while the publication transaction waits
	// on the parent row. Old and newly assigned subjects are identical.
	_, err = writer.Exec(f.ctx, `INSERT INTO problem_review_assignment_history
		(review_id,revision,reviewer_subject_id,reviewer_revision,maintenance_operator,reason)
		VALUES ($1,$2,$3,1,'fixture','same reviewer reassignment')`, e.ReviewID, e.AssignmentRevision+1, f.reader.SubjectID)
	must(t, err)
	must(t, writer.Commit(f.ctx))
	requireReviewError(t, <-out, problemreference.ErrReviewChanged)
}

func TestPublicationGateLeavesLocksInCallersTransaction(t *testing.T) {
	f, e := publicationApprovalFixture(t)
	seedPublicationApproval(t, f, e, currentPublicationSnapshot(t, f, e), problemreference.ReviewPolicyVersion)
	holder, err := f.a.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	must(t, err)
	defer func() { _ = holder.Rollback(f.ctx) }()
	_, err = f.reviews.LockPublicationReview(f.ctx, holder, e.ReviewID, e.AssignmentRevision, e.Reviewer)
	requireReviewError(t, err, problemreference.ErrReviewApprovalBlocked)

	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	out := make(chan error, 1)
	go func() {
		_, err := f.reviews.SetAssignment(ctx, f.change(e.AssignmentRevision, f.reader.SubjectID, 1))
		out <- err
	}()
	waitBacklogBlocked(t, f.backlogFixture, holder.Conn().PgConn().PID(), f.appB)
	must(t, holder.Rollback(f.ctx))
	must(t, <-out)
}
