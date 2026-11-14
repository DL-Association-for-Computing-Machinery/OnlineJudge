//go:build integration

package integration_test

import (
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

func TestReviewDiscoveryOnlyShowsCurrentNonAuthorAssignments(t *testing.T) {
	f := newReviewFixture(t)
	var start problemreference.ReviewDiscoveryPosition
	_, err := f.reviews.DiscoverReviewAssignments(f.ctx, f.reader, start)
	requireReviewError(t, err, identity.ErrReviewerRequired)
	f.grant(t, f.reader, 0, true)
	f.grant(t, f.actor, 0, true)
	other := f.reader
	other.SubjectID = identityID(1102)
	_, err = f.identities.Initialize(f.ctx, other.SubjectID, true, identityRecord(t, identityHasher(t, false), "synthetic reviewer for discovery"))
	must(t, err)
	f.grant(t, other, 0, true)
	// Keep the real item outside the fixture's low ID range, then copy only the
	// immutable projection shape to create many historical candidates cheaply.
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_review_submissions
		(id,owner_subject_id,request_key,draft_id,source_id,content_version,import_revision,source_lock_sha256,
		reference_request_id,formal_plan_sha256,profile_sha256,evidence_sha256,formal_verdict,
		differential_plan_sha256,differential_outcome,auth_generation,recovery_id,author_revision)
		SELECT lpad(to_hex(n),32,'0'), owner_subject_id, 'discovery-test-case-'||n, draft_id, source_id, content_version,
		import_revision, source_lock_sha256, reference_request_id, formal_plan_sha256, profile_sha256,
		evidence_sha256, formal_verdict, differential_plan_sha256, differential_outcome, auth_generation, recovery_id, author_revision
		FROM problem_review_submissions CROSS JOIN generate_series(1,24) n WHERE id=$1`, f.item.ID)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_review_assignment_history
		(review_id,revision,reviewer_subject_id,reviewer_revision,maintenance_operator,reason)
		SELECT lpad(to_hex(n),32,'0'),v,$1,1,'synthetic','same item repeated' FROM generate_series(1,24) n CROSS JOIN generate_series(1,30) v`, f.reader.SubjectID)
	must(t, err)
	for n := 1; n <= 21; n++ {
		change := f.change(30, "", 0)
		change.ReviewID = identityID(n)
		if n >= 20 {
			change.ReviewerSubjectID, change.ExpectedReviewerRevision = other.SubjectID, 1
		}
		_, err := f.reviews.SetAssignment(f.ctx, change)
		must(t, err)
	}
	// The store also excludes self-authored rows defensively, even if such a
	// row was introduced by direct maintenance SQL instead of SetAssignment.
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_review_assignment_history
		(review_id,revision,reviewer_subject_id,reviewer_revision,maintenance_operator,reason)
		VALUES($1,1,$2,1,'synthetic','invalid historical attribution')`, f.item.ID, f.actor.SubjectID)
	must(t, err)
	first, err := f.reviews.DiscoverReviewAssignments(f.ctx, f.reader, start)
	must(t, err)
	if first.Items == nil || len(first.Items) != 0 || first.Next.ID != identityID(20) || first.Next.RoleRevision != 1 {
		t.Fatalf("stale first page did not advance exactly once: %+v", first)
	}
	next, err := f.reviews.DiscoverReviewAssignments(f.ctx, f.reader, first.Next)
	must(t, err)
	if len(next.Items) != 3 || next.Next != start || next.Items[0].ReviewID != identityID(22) ||
		next.Items[2].ReviewID != identityID(24) || next.Items[0].ImportRevision != f.draft.ImportRevision {
		t.Fatalf("stale or wrong assignments in second page: %+v", next)
	}
	otherPage, err := f.reviews.DiscoverReviewAssignments(f.ctx, other, start)
	must(t, err)
	if len(otherPage.Items) != 2 || otherPage.Items[0].ReviewID != identityID(20) {
		t.Fatalf("other reviewer did not get only the reassigned items: %+v", otherPage)
	}
	author, err := f.reviews.DiscoverReviewAssignments(f.ctx, f.actor, start)
	must(t, err)
	if len(author.Items) != 0 {
		t.Fatal("self-authored item appeared")
	}
	f.reject(t, f.reader, identityID(20), problemreference.ErrNotFound)
	f.grant(t, f.reader, 1, false)
	_, err = f.reviews.DiscoverReviewAssignments(f.ctx, f.reader, first.Next)
	requireReviewError(t, err, identity.ErrReviewerRequired)
	f.grant(t, f.reader, 2, true)
	_, err = f.reviews.DiscoverReviewAssignments(f.ctx, f.reader, first.Next)
	requireReviewError(t, err, problemreference.ErrReviewCursor)
	regranted, err := f.reviews.DiscoverReviewAssignments(f.ctx, f.reader, start)
	must(t, err)
	if len(regranted.Items) != 0 || regranted.Next != start {
		t.Fatal("a new grant inherited the previous assignments")
	}
	_, err = f.provider.DownTo(f.ctx, 24)
	must(t, err)
	latest, err := f.reviews.Assignment(f.ctx, identityID(20))
	must(t, err)
	if latest.Revision != 31 || latest.ReviewerSubjectID != other.SubjectID {
		t.Fatal("schema rollback removed assignment history")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
}
