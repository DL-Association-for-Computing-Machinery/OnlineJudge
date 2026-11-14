//go:build integration

package integration_test

import (
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/jackc/pgx/v5"
)

func publisherChange(subject string, revision int64, enabled bool) identity.ProblemRoleChange {
	return identity.ProblemRoleChange{SubjectID: subject, ExpectedRevision: revision, Enabled: enabled,
		Operator: "publisher-fixture", Reason: "controlled publisher grant"}
}

func checkPublisher(t *testing.T, f draftFixture, actor identity.ProblemActor, other string, revision int64, isolation pgx.TxIsoLevel, want error) {
	t.Helper()
	tx, err := f.b.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: isolation})
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	mustCredentialError(t, f.access.LockPublisher(f.ctx, tx, actor, revision, other), want)
}

func TestPublisherAuthorityIndependentAndRevisionBound(t *testing.T) {
	f := newDraftFixture(t)
	reviewer := identityID(1101)
	f.role(t, f.actor.SubjectID, 0, true)
	_, err := f.access.SetReviewerRole(f.ctx, publisherChange(f.actor.SubjectID, 0, true))
	must(t, err)
	checkPublisher(t, f, f.actor, reviewer, 1, pgx.ReadCommitted, identity.ErrPublisherRequired)
	first := publisherChange(f.actor.SubjectID, 0, true)
	_, err = f.access.SetPublisherRole(f.ctx, first)
	must(t, err)
	_, err = f.access.SetPublisherRole(f.ctx, first)
	mustCredentialError(t, err, identity.ErrConflict)
	checkPublisher(t, f, f.actor, reviewer, 1, pgx.ReadCommitted, nil)
	checkPublisher(t, f, f.actor, f.actor.SubjectID, 1, pgx.ReadCommitted, nil)
	checkPublisher(t, f, f.actor, reviewer, 1, pgx.RepeatableRead, identity.ErrInvalid)
	stale := f.actor
	stale.AuthGeneration++
	checkPublisher(t, f, stale, reviewer, 1, pgx.ReadCommitted, identity.ErrConflict)
	_, err = f.access.SetPublisherRole(f.ctx, publisherChange(f.actor.SubjectID, 1, false))
	must(t, err)
	checkPublisher(t, f, f.actor, reviewer, 1, pgx.ReadCommitted, identity.ErrPublisherRequired)
	_, err = f.access.SetPublisherRole(f.ctx, publisherChange(f.actor.SubjectID, 2, true))
	must(t, err)
	checkPublisher(t, f, f.actor, reviewer, 1, pgx.ReadCommitted, identity.ErrConflict)
	checkPublisher(t, f, f.actor, reviewer, 3, pgx.ReadCommitted, nil)
	role, err := f.access.PublisherRole(f.ctx, f.actor.SubjectID)
	must(t, err)
	if role.SubjectID != f.actor.SubjectID || !role.Enabled || role.Revision != 3 {
		t.Fatal("unexpected grant readback")
	}
	row, err := f.identities.State(f.ctx, reviewer)
	must(t, err)
	_, err = f.identities.DisableCAS(f.ctx, reviewer, row.Revision)
	must(t, err)
	checkPublisher(t, f, f.actor, reviewer, 3, pgx.ReadCommitted, identity.ErrConflict)
	_, err = f.access.SetPublisherRole(f.ctx, publisherChange(reviewer, 0, true))
	mustCredentialError(t, err, identity.ErrPublisherRequired)
	old := f.actor
	f.actor.RecoveryID = openRecovery(t, f.ctx, f.identities)
	checkPublisher(t, f, old, old.SubjectID, 3, pgx.ReadCommitted, identity.ErrConflict)
	checkPublisher(t, f, f.actor, f.actor.SubjectID, 3, pgx.ReadCommitted, nil)
}

func TestPublisherWaitsForAccountBeforeReadingLatestGrant(t *testing.T) {
	f := newDraftFixture(t)
	_, err := f.access.SetPublisherRole(f.ctx, publisherChange(f.actor.SubjectID, 0, true))
	must(t, err)
	holder, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = holder.Rollback(f.ctx) }()
	var subject string
	must(t, holder.QueryRow(f.ctx, "SELECT id FROM identity_credentials WHERE id=$1 FOR UPDATE", f.actor.SubjectID).Scan(&subject))
	out := make(chan error, 1)
	go func() {
		tx, err := f.b.Begin(f.ctx)
		if err == nil {
			err = f.access.LockPublisher(f.ctx, tx, f.actor, 1, identityID(1101))
			_ = tx.Rollback(f.ctx)
		}
		out <- err
	}()
	waitBacklogBlocked(t, f.backlogFixture, holder.Conn().PgConn().PID(), f.appB)
	_, err = holder.Exec(f.ctx, `INSERT INTO identity_problem_publisher_history
		(subject_id,revision,enabled,maintenance_operator,reason) VALUES ($1,2,false,'fixture','revoke before read')`, subject)
	must(t, err)
	must(t, holder.Commit(f.ctx))
	mustCredentialError(t, <-out, identity.ErrPublisherRequired)
}

func TestPublisherAndReviewGatesShareCallerLocks(t *testing.T) {
	f, evidence := publicationApprovalFixture(t)
	seedPublicationApproval(t, f, evidence, currentPublicationSnapshot(t, f, evidence), problemreference.ReviewPolicyVersion)
	// Use a publisher that sorts after the assigned reviewer.
	publisher := f.actor
	publisher.SubjectID = identityID(1102)
	h := identityHasher(t, false)
	_, err := f.identities.Initialize(f.ctx, publisher.SubjectID, true, identityRecord(t, h, "synthetic publisher password"))
	must(t, err)
	_, err = f.access.SetPublisherRole(f.ctx, publisherChange(publisher.SubjectID, 0, true))
	must(t, err)
	holder, err := f.a.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	must(t, err)
	defer func() { _ = holder.Rollback(f.ctx) }()
	must(t, f.access.LockPublisher(f.ctx, holder, publisher, 1, evidence.Reviewer))

	// Both tests run before the review gate takes any additional locks.
	publisherChangeDone := make(chan error, 1)
	go func() {
		_, err := f.access.SetPublisherRole(f.ctx, publisherChange(publisher.SubjectID, 1, false))
		publisherChangeDone <- err
	}()
	waitBacklogBlocked(t, f.backlogFixture, holder.Conn().PgConn().PID(), f.appA)
	state, err := f.identities.State(f.ctx, f.reader.SubjectID)
	must(t, err)
	reviewerIdentity, err := identitypg.New(f.b, h)
	must(t, err)
	reviewerChangeDone := make(chan error, 1)
	go func() {
		_, err := reviewerIdentity.DisableCAS(f.ctx, f.reader.SubjectID, state.Revision)
		reviewerChangeDone <- err
	}()
	waitBacklogBlocked(t, f.backlogFixture, holder.Conn().PgConn().PID(), f.appB)
	_, err = f.reviews.LockPublicationReview(f.ctx, holder, evidence.ReviewID, evidence.AssignmentRevision, evidence.Reviewer)
	requireReviewError(t, err, problemreference.ErrReviewApprovalBlocked)
	must(t, holder.Rollback(f.ctx))
	must(t, <-publisherChangeDone)
	must(t, <-reviewerChangeDone)
}

func TestPublisherMigrationProtectsAuditAndLookupUsesPrimaryKey(t *testing.T) {
	f := newDraftFixture(t)
	_, err := f.provider.DownTo(f.ctx, 27)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("startup accepted previous schema")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	_, err = f.access.SetPublisherRole(f.ctx, publisherChange(f.actor.SubjectID, 0, true))
	must(t, err)
	_, err = f.provider.DownTo(f.ctx, 27)
	if err == nil || !strings.Contains(err.Error(), "cannot discard publisher history") {
		t.Fatalf("rollback did not protect publisher audit: %v", err)
	}
	// Empty newer migrations can be removed before Down28 refuses.
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	// Only inspect index eligibility on a tiny fixture.
	_, err = tx.Exec(f.ctx, "SET LOCAL enable_seqscan=off")
	must(t, err)
	rows, err := tx.Query(f.ctx, `EXPLAIN (ANALYZE, BUFFERS)
		SELECT subject_id, revision, enabled FROM identity_problem_publisher_history
		WHERE subject_id=$1 ORDER BY revision DESC LIMIT 1`, f.actor.SubjectID)
	must(t, err)
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		must(t, rows.Scan(&line))
		plan.WriteString(line + "\n")
	}
	must(t, rows.Err())
	if !strings.Contains(plan.String(), "identity_problem_publisher_history_pkey") {
		t.Fatal("publisher grant lookup has no primary key plan")
	}
	t.Log(plan.String())
}
