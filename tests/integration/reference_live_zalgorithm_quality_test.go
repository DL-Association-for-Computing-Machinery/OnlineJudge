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

func verifyLiveZAlgorithmQuality(t *testing.T, ctx context.Context, f draftFixture, requests *referencepg.Store,
	credential redisstore.Credential, reviewer identity.ProblemActor, out string, intent problemreference.ReviewIntent,
) {
	t.Helper()
	item, err := requests.SubmitReview(ctx, f.actor, intent)
	must(t, err)
	reviews, err := referencepg.NewReviewStore(f.b, f.access)
	must(t, err)
	_, err = f.access.SetReviewerRole(ctx, identity.ProblemRoleChange{SubjectID: reviewer.SubjectID, Enabled: true,
		Operator: "fixture-maintainer", Reason: "verify z-algorithm quality access"})
	must(t, err)
	_, err = reviews.SetAssignment(ctx, problemreference.ReviewAssignmentChange{ReviewID: item.ID,
		ReviewerSubjectID: reviewer.SubjectID, ExpectedReviewerRevision: 1, Operator: "fixture-maintainer", Reason: "assign z-algorithm review"})
	must(t, err)
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: reviewer.SubjectID,
		AuthGeneration: reviewer.AuthGeneration, RecoveryID: reviewer.RecoveryID}}
	read, err := problemreference.NewReviewAccess(sessions, reviews)
	must(t, err)
	got, err := read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, item.ID)
	must(t, err)
	want := []string{"statement_quality_missing", "test_coverage_missing", "checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing"}
	const report = "19e3ca6ab1c5a22337993ce1ff8c324fb39b060978555b5020ea27f9b8f05176"
	input, independent := got.InputValidator, got.IndependentCorrectness
	if got.Eligible || !slices.Equal(got.Blockers, want) || got.TestCoverage != nil || got.StatementRendering != nil || got.WrongSolution != nil ||
		input == nil || input.Inputs != 32 || input.Controls != 4 || input.ReportSHA256 != report ||
		independent == nil || independent.Cases != 62 || independent.ReportSHA256 != report {
		t.Fatalf("unexpected evidence from the reviewed 32-point plan: %+v", got)
	}
	_, err = reviews.ReturnReview(ctx, reviewer, item.ID, problemreference.ReturnIntent{
		RequestKey: "live-zalgorithm-return", AssignmentRevision: got.Evidence.AssignmentRevision,
		ImportRevision: intent.ImportRevision, EvidenceSHA256: got.Evidence.EvidenceSHA256, Reason: "fixture return",
	})
	must(t, err)
	returned, err := read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, item.ID)
	must(t, err)
	if returned.Eligible || !slices.Equal(returned.Blockers, append([]string{"materials_returned"}, want...)) ||
		returned.InputValidator == nil || *returned.InputValidator != *input ||
		returned.IndependentCorrectness == nil || *returned.IndependentCorrectness != *independent {
		t.Fatalf("return did not preserve historical evidence and its blocker: %+v", returned)
	}
	writeLiveReferenceJSON(t, out, "quality-zalgorithm", map[string]any{"current": got, "returned": returned})
}
