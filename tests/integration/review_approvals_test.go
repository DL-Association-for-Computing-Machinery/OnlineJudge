//go:build integration

package integration_test

import (
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/jackc/pgx/v5/pgconn"
)

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
