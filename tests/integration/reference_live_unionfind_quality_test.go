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

func verifyLiveUnionFindQuality(t *testing.T, ctx context.Context, f draftFixture, requests *referencepg.Store,
	credential redisstore.Credential, reviewer identity.ProblemActor, out string, intent problemreference.ReviewIntent,
) {
	t.Helper()
	item, err := requests.SubmitReview(ctx, f.actor, intent)
	must(t, err)
	reviews, err := referencepg.NewReviewStore(f.b, f.access)
	must(t, err)
	_, err = f.access.SetReviewerRole(ctx, identity.ProblemRoleChange{SubjectID: reviewer.SubjectID, Enabled: true,
		Operator: "fixture-maintainer", Reason: "verify unionfind quality access"})
	must(t, err)
	_, err = reviews.SetAssignment(ctx, problemreference.ReviewAssignmentChange{ReviewID: item.ID,
		ReviewerSubjectID: reviewer.SubjectID, ExpectedReviewerRevision: 1, Operator: "fixture-maintainer", Reason: "assign unionfind review"})
	must(t, err)
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: reviewer.SubjectID,
		AuthGeneration: reviewer.AuthGeneration, RecoveryID: reviewer.RecoveryID}}
	read, err := problemreference.NewReviewAccess(sessions, reviews)
	must(t, err)
	got, err := read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, item.ID)
	must(t, err)
	want := []string{"statement_quality_missing", "test_coverage_missing", "checker_admission_missing",
		"target_runtime_missing", "solution_confirmation_missing"}
	const report = "f6937f8e62284bd3514ae593126c63a8a0f57b139bdafc63437819287e6bf9db"
	input, independent := got.InputValidator, got.IndependentCorrectness
	if got.Eligible || !slices.Equal(got.Blockers, want) || got.StatementRendering != nil || got.TestCoverage != nil || got.WrongSolution != nil ||
		input == nil || input.Inputs != 20 || input.Controls != 4 || input.ReportSHA256 != report ||
		independent == nil || independent.Cases != 10 || independent.ReportSHA256 != report {
		t.Fatalf("unexpected evidence from the reviewed 20-point plan: %+v", got)
	}
	writeLiveReferenceJSON(t, out, "quality-unionfind", got)
}
