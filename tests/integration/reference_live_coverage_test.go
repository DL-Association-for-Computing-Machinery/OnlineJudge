//go:build integration && linux && live_reference

package integration_test

import (
	"context"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func verifyLiveCoverageReview(t *testing.T, ctx context.Context, f draftFixture, requests *referencepg.Store,
	credential redisstore.Credential, reviewer identity.ProblemActor, out string, local bool, intent problemreference.ReviewIntent,
) {
	t.Helper()
	item, err := requests.SubmitReview(ctx, f.actor, intent)
	must(t, err)
	reviews, err := referencepg.NewReviewStore(f.b, f.access)
	must(t, err)
	for _, actor := range []identity.ProblemActor{f.actor, reviewer} {
		_, err := f.access.SetReviewerRole(ctx, identity.ProblemRoleChange{SubjectID: actor.SubjectID, Enabled: true,
			Operator: "fixture-maintainer", Reason: "verify coverage review access"})
		must(t, err)
	}
	viewFor := func(actor identity.ProblemActor) *problemreference.ReviewAccess {
		sessions := fixedDraftSession{view: redisstore.View{SubjectID: actor.SubjectID,
			AuthGeneration: actor.AuthGeneration, RecoveryID: actor.RecoveryID}}
		s, err := problemreference.NewReviewAccess(sessions, reviews)
		must(t, err)
		return s
	}
	read := viewFor(reviewer)
	_, err = read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, item.ID)
	requireReviewError(t, err, problemreference.ErrNotFound)
	_, err = reviews.SetAssignment(ctx, problemreference.ReviewAssignmentChange{ReviewID: item.ID,
		ReviewerSubjectID: reviewer.SubjectID, ExpectedReviewerRevision: 1, Operator: "fixture-maintainer", Reason: "assign coverage review"})
	must(t, err)
	_, err = viewFor(f.actor).EligibilityForSubject(ctx, credential, f.actor.SubjectID, item.ID)
	requireReviewError(t, err, problemreference.ErrNotFound)
	got, err := read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, item.ID)
	must(t, err)
	if got.Eligible || (got.TestCoverage != nil) != local ||
		slices.Contains(got.Blockers, "test_coverage_missing") == local {
		t.Fatalf("unexpected coverage admission: %+v", got)
	}
	if local {
		c := got.TestCoverage
		if c.Points != 17 || c.SamplePoints != 2 || c.NonSampleCriteria != 7 || c.SampleOnlyCriteria != 1 ||
			c.MaximumSumSamplePosition != 2 || c.WrongSolutions[3].RejectedPoints != 10 ||
			got.InputValidator != nil || got.IndependentCorrectness != nil || got.StatementRendering != nil ||
			!slices.Contains(got.Blockers, "checker_admission_missing") || !slices.Contains(got.Blockers, "target_runtime_missing") {
			t.Fatalf("incorrect coverage summary or unrelated prerequisite changed: %+v", got)
		}
	}
	_, err = reviews.ReturnReview(ctx, reviewer, item.ID, problemreference.ReturnIntent{
		RequestKey: "live-coverage-return", AssignmentRevision: got.Evidence.AssignmentRevision,
		ImportRevision: intent.ImportRevision, EvidenceSHA256: got.Evidence.EvidenceSHA256, Reason: "fixture return",
	})
	must(t, err)
	returned, err := read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, item.ID)
	must(t, err)
	if returned.TestCoverage != nil || returned.Eligible || !slices.Contains(returned.Blockers, "materials_returned") ||
		!slices.Contains(returned.Blockers, "test_coverage_missing") {
		t.Fatalf("returned material retained coverage admission: %+v", returned)
	}
	writeLiveReferenceJSON(t, out, "coverage-aplusb", map[string]any{"current": got, "returned": returned})
}
