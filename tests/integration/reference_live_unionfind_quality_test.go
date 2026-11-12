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

func verifyLiveUnionFindQuality(t *testing.T, ctx context.Context, f draftFixture, requests *referencepg.Store, objects problem.ReferenceObjectReader,
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
	want := []string{"statement_quality_missing", "checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing"}
	const report = "f6937f8e62284bd3514ae593126c63a8a0f57b139bdafc63437819287e6bf9db"
	input, independent := got.InputValidator, got.IndependentCorrectness
	if got.Eligible || !slices.Equal(got.Blockers, want) || got.WrongSolution != nil ||
		input == nil || input.Inputs != 20 || input.Controls != 4 || input.ReportSHA256 != report ||
		independent == nil || independent.Cases != 10 || independent.ReportSHA256 != report {
		t.Fatalf("unexpected evidence from the reviewed 20-point plan: %+v", got)
	}
	rendering := got.StatementRendering
	if rendering == nil || rendering.Examples != 1 || rendering.Formulas != 10 || rendering.FormatBlocks != 0 ||
		rendering.HistoricalMatch == nil || rendering.HistoricalMatch.ReportSHA256 != "3dd77d0fa9bc501ae23a3c352e0c83486b71d6b8d619de4451b27534db86d453" {
		t.Fatalf("reviewer did not receive the bound historical rendering: %+v", rendering)
	}
	coverage := got.TestCoverage
	if coverage == nil || coverage.ReportSHA256 != "c7b67173c8fe054d44ea3e4b9267aa5618ec5c109e6c3cd6b006c35e12d7f9df" ||
		coverage.LocalCPUProbe == nil || coverage.LocalCPUProbe.CandidateOutcome != "cpu_limit_exceeded" {
		t.Fatalf("reviewer did not receive the bound coverage report: %+v", coverage)
	}
	views := map[string]any{"current": got}
	const payloadSHA256 = "961fcb88a3ac2ff44149d08f6415ca8a87a723deb92d63e4ca789e6f4f0d0e0f"
	got = verifyLiveRenderingConfirmation(t, ctx, reviews, objects, credential, reviewer, read, got, views, payloadSHA256)
	_, err = reviews.ReturnReview(ctx, reviewer, item.ID, problemreference.ReturnIntent{
		RequestKey: "live-unionfind-return", AssignmentRevision: got.Evidence.AssignmentRevision,
		ImportRevision: intent.ImportRevision, EvidenceSHA256: got.Evidence.EvidenceSHA256, Reason: "fixture return",
	})
	must(t, err)
	returned, err := read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, item.ID)
	must(t, err)
	wantReturned := []string{"materials_returned", "statement_quality_missing", "test_coverage_missing", "checker_admission_missing",
		"target_runtime_missing", "solution_confirmation_missing"}
	if returned.Eligible || returned.TestCoverage != nil || returned.StatementSemanticsConfirmed || !slices.Equal(returned.Blockers, wantReturned) ||
		returned.StatementRendering == nil || returned.StatementRendering.HistoricalMatch == nil ||
		*returned.StatementRendering.HistoricalMatch != *rendering.HistoricalMatch ||
		returned.InputValidator == nil || *returned.InputValidator != *input ||
		returned.IndependentCorrectness == nil || *returned.IndependentCorrectness != *independent {
		t.Fatalf("return did not restore the statement and coverage prerequisites: %+v", returned)
	}
	views["returned"] = returned
	writeLiveReferenceJSON(t, out, "quality-unionfind", views)
}
