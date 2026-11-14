//go:build integration

package integration_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestReviewApprovalRejectsBlockedSnapshotAndRecoversOriginalKey(t *testing.T) {
	f := newReviewFixture(t)
	f.grant(t, f.reader, 0, true)
	f.grant(t, f.actor, 0, true)
	e := assignReview(t, f, f.item.ID)
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: f.reader.SubjectID,
		AuthGeneration: f.reader.AuthGeneration, RecoveryID: f.reader.RecoveryID}}
	access, err := problemreference.NewReviewAccess(sessions, f.reviews)
	must(t, err)
	status, err := access.EligibilityForSubject(f.ctx, redisstore.Credential{}, f.reader.SubjectID, e.ReviewID)
	must(t, err)
	i := problemreference.ApprovalIntent{RequestKey: "approval-attempt-key", AssignmentRevision: e.AssignmentRevision,
		SnapshotSHA256: status.SnapshotSHA256, Reason: "Synthetic review"}
	_, err = f.reviews.ApproveReview(f.ctx, f.actor, e.ReviewID, i)
	requireReviewError(t, err, problemreference.ErrNotFound)
	_, err = f.reviews.ApproveReview(f.ctx, f.reader, e.ReviewID, i)
	requireReviewError(t, err, problemreference.ErrReviewApprovalBlocked)
	bad := i
	bad.SnapshotSHA256 = strings.Repeat("f", 64)
	_, err = f.reviews.ApproveReview(f.ctx, f.reader, e.ReviewID, bad)
	requireReviewError(t, err, problemreference.ErrReviewChanged)
	_, err = f.reviews.ReviewApprovalReceipt(f.ctx, f.reader, e.ReviewID, i.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
	var count int
	must(t, f.a.QueryRow(f.ctx, "SELECT count(*) FROM problem_review_approvals").Scan(&count))
	if count != 0 {
		t.Fatal("blocked approval wrote an audit row")
	}

	// The real policy still blocks new approvals. Seed a historical audit row
	// only to verify recovery and the interaction with the return transaction.
	digest := sha256.Sum256([]byte("onlinejudge/review-material/v1:" + f.actor.SubjectID +
		e.ImportRevision + e.FormalPlanSHA256 + e.ProfileSHA256 + e.EvidenceSHA256 + e.ComparisonPlan))
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_review_approvals
		(review_id, assignment_revision, reviewer_subject_id, reviewer_revision, request_key, material_sha256,
		 snapshot_sha256, policy_version, reason, auth_generation, recovery_id)
		VALUES ($1, $2, $3, 1, $4, $5, $6, $7, $8, $9, $10)`, e.ReviewID, i.AssignmentRevision, f.reader.SubjectID,
		i.RequestKey, hex.EncodeToString(digest[:]), i.SnapshotSHA256, problemreference.ReviewPolicyVersion, i.Reason,
		f.reader.AuthGeneration, f.reader.RecoveryID)
	must(t, err)
	receipt, err := f.reviews.ReviewApprovalReceipt(f.ctx, f.reader, e.ReviewID, i.RequestKey)
	must(t, err)
	again, err := f.reviews.ApproveReview(f.ctx, f.reader, e.ReviewID, i)
	must(t, err)
	if receipt != again || !receipt.Valid() || receipt.ApprovalIntent != i {
		t.Fatal("same request key did not recover the original approval")
	}
	bad = i
	bad.Reason = "Changed decision"
	_, err = f.reviews.ApproveReview(f.ctx, f.reader, e.ReviewID, bad)
	requireReviewError(t, err, problemreference.ErrConflict)
	_, err = f.reviews.ReturnReview(f.ctx, f.reader, e.ReviewID, returnIntent(e, "return-after-approval-key"))
	requireReviewError(t, err, problemreference.ErrReviewAlreadyApproved)

	a, err := f.reviews.SetAssignment(f.ctx, f.change(e.AssignmentRevision, f.reader.SubjectID, 1))
	must(t, err)
	_, err = f.reviews.ReviewApprovalReceipt(f.ctx, f.reader, e.ReviewID, i.RequestKey)
	requireReviewError(t, err, problemreference.ErrNotFound)
	e.AssignmentRevision = a.Revision
	_, err = f.reviews.ReturnReview(f.ctx, f.reader, e.ReviewID, returnIntent(e, "return-after-reassignment-key"))
	must(t, err)
}

func TestReviewApprovalLedgerBindsAssignmentAndRetainsAudit(t *testing.T) {
	f := newReviewFixture(t)
	must(t, db.CheckSchema(f.ctx, f.a))
	_, err := f.provider.DownTo(f.ctx, 25)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("startup accepted schema without the approval ledger")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))

	f.grant(t, f.reader, 0, true)
	f.role(t, f.reader.SubjectID, 0, true)
	e := assignReview(t, f, f.item.ID)
	const insert = `INSERT INTO problem_review_approvals
		(review_id, assignment_revision, reviewer_subject_id, reviewer_revision, request_key,
		 material_sha256, snapshot_sha256, policy_version, reason, auth_generation, recovery_id)
		VALUES ($1, $2, $3, $4, $5, repeat('a',64), repeat('b',64), 'test.v1', 'test audit row', 1, repeat('c',32))`
	write := func(revision int64, key string) error {
		_, err := f.a.Exec(f.ctx, insert, e.ReviewID, e.AssignmentRevision, f.reader.SubjectID, revision, key)
		return err
	}
	assertCode := func(err error, code string) {
		t.Helper()
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != code {
			t.Fatalf("expected PostgreSQL error %s, got %v", code, err)
		}
	}
	assertCode(write(2, "approval-original-key"), "23503")
	must(t, write(1, "approval-original-key"))
	assertCode(write(1, "approval-different-key"), "23505")
	_, err = f.provider.DownTo(f.ctx, 25)
	assertCode(err, "P0001")
	must(t, db.CheckSchema(f.ctx, f.a))
	var count int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_review_approvals`).Scan(&count))
	if count != 1 {
		t.Fatalf("approval ledger lost its original audit row: %d", count)
	}
}
