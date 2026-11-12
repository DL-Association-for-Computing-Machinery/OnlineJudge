//go:build integration && linux && live_reference

package integration_test

import (
	"context"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func verifyLiveZAlgorithmQuality(t *testing.T, ctx context.Context, f draftFixture, requests *referencepg.Store, objects problem.ReferenceObjectReader,
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
	want := []string{"statement_quality_missing", "checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing"}
	const report = "19e3ca6ab1c5a22337993ce1ff8c324fb39b060978555b5020ea27f9b8f05176"
	input, independent, coverage := got.InputValidator, got.IndependentCorrectness, got.TestCoverage
	if got.Eligible || !slices.Equal(got.Blockers, want) || got.WrongSolution != nil ||
		coverage == nil || coverage.Points != 32 || coverage.SamplePoints != 4 || coverage.NonSampleCriteria != 6 || coverage.LocalCPUProbe == nil ||
		input == nil || input.Inputs != 32 || input.Controls != 4 || input.ReportSHA256 != report ||
		independent == nil || independent.Cases != 62 || independent.ReportSHA256 != report {
		t.Fatalf("unexpected evidence from the reviewed 32-point plan: %+v", got)
	}
	rendering := got.StatementRendering
	if rendering == nil || rendering.Examples != 4 || rendering.Formulas != 8 || rendering.FormatBlocks != 0 ||
		rendering.HistoricalMatch == nil || rendering.HistoricalMatch.ReportSHA256 != "b2319f18ba9b65eec5aa720d2867795acdde450a14d0a101d6d4291bda499d3e" {
		t.Fatalf("reviewer did not receive bound historical rendering: %+v", rendering)
	}
	views := map[string]any{"current": got}
	const payloadSHA256 = "d5f6202ba5df73e0eb1b7b9e377340db6b63ffe47f047f1efb98496ec003cb11"
	got = verifyLiveRenderingConfirmation(t, ctx, reviews, objects, credential, reviewer, read, got, views, payloadSHA256)
	_, err = reviews.ReturnReview(ctx, reviewer, item.ID, problemreference.ReturnIntent{
		RequestKey: "live-zalgorithm-return", AssignmentRevision: got.Evidence.AssignmentRevision,
		ImportRevision: intent.ImportRevision, EvidenceSHA256: got.Evidence.EvidenceSHA256, Reason: "fixture return",
	})
	must(t, err)
	returned, err := read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, item.ID)
	must(t, err)
	if returned.Eligible || returned.TestCoverage != nil || returned.StatementSemanticsConfirmed ||
		!slices.Equal(returned.Blockers, []string{"materials_returned", want[0], "test_coverage_missing", want[1], want[2], want[3]}) ||
		returned.StatementRendering == nil || returned.StatementRendering.HistoricalMatch == nil ||
		*returned.StatementRendering.HistoricalMatch != *rendering.HistoricalMatch ||
		returned.InputValidator == nil || *returned.InputValidator != *input ||
		returned.IndependentCorrectness == nil || *returned.IndependentCorrectness != *independent {
		t.Fatalf("return did not preserve historical evidence and its blocker: %+v", returned)
	}
	views["returned"] = returned
	writeLiveReferenceJSON(t, out, "quality-zalgorithm", views)
}
