//go:build integration && linux && live_reference

package integration_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func verifyLiveCoverageReview(t *testing.T, ctx context.Context, f draftFixture, requests *referencepg.Store, objects problem.ReferenceObjectReader,
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
	validator, small := got.InputValidator, got.IndependentCorrectness
	if validator == nil || validator.Controls != 4 || small == nil || small.Cases != 25 ||
		validator.ReportSHA256 != small.ReportSHA256 ||
		slices.Contains(got.Blockers, "input_validator_missing") || slices.Contains(got.Blockers, "independent_correctness_missing") {
		t.Fatalf("incorrect input or oracle evidence: %+v", got)
	}
	if local {
		c := got.TestCoverage
		if c.Points != 17 || c.SamplePoints != 2 || c.NonSampleCriteria != 7 || c.SampleOnlyCriteria != 1 ||
			c.MaximumSumSamplePosition != 2 || c.WrongSolutions[3].RejectedPoints != 10 ||
			validator.Inputs != 17 || validator.ReportSHA256 != "70c7834df6a86220da0cb8a9e6b30f7015832ff082cdeaa45eced15579107bc0" ||
			got.StatementRendering == nil || got.StatementRendering.HistoricalMatch == nil || got.WrongSolution != nil ||
			!slices.Contains(got.Blockers, "checker_admission_missing") || !slices.Contains(got.Blockers, "target_runtime_missing") {
			t.Fatalf("incorrect coverage summary or unrelated prerequisite changed: %+v", got)
		}
	} else if validator.Inputs != 12 || validator.ReportSHA256 != "cc6907675c87d93e6c0dd08fc637d13bcd34ff6b4ba9e497fbce4c6681578a9d" ||
		got.StatementRendering == nil || got.StatementRendering.HistoricalMatch != nil || got.WrongSolution == nil {
		t.Fatalf("original v6 evidence changed: %+v", got)
	}
	views := map[string]any{"current": got}
	if local {
		got = verifyLiveRenderingConfirmation(t, ctx, reviews, objects, credential, reviewer, read, got, views)
	}
	_, err = reviews.ReturnReview(ctx, reviewer, item.ID, problemreference.ReturnIntent{
		RequestKey: "live-coverage-return", AssignmentRevision: got.Evidence.AssignmentRevision,
		ImportRevision: intent.ImportRevision, EvidenceSHA256: got.Evidence.EvidenceSHA256, Reason: "fixture return",
	})
	must(t, err)
	returned, err := read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, item.ID)
	must(t, err)
	if returned.TestCoverage != nil || returned.Eligible || returned.StatementSemanticsConfirmed ||
		!slices.Contains(returned.Blockers, "statement_quality_missing") || !slices.Contains(returned.Blockers, "materials_returned") ||
		!slices.Contains(returned.Blockers, "test_coverage_missing") || returned.InputValidator == nil ||
		returned.IndependentCorrectness == nil || *returned.InputValidator != *validator || *returned.IndependentCorrectness != *small {
		t.Fatalf("returned material retained coverage admission: %+v", returned)
	}
	views["returned"] = returned
	writeLiveReferenceJSON(t, out, "coverage-aplusb", views)
}

func verifyLiveRenderingConfirmation(t *testing.T, ctx context.Context, reviews *referencepg.ReviewStore, objects problem.ReferenceObjectReader,
	credential redisstore.Credential, reviewer identity.ProblemActor, read *problemreference.ReviewAccess,
	initial problemreference.ReviewEligibility, views map[string]any,
) problemreference.ReviewEligibility {
	t.Helper()
	id := initial.Evidence.ReviewID
	if initial.StatementSemanticsConfirmed || !slices.Contains(initial.Blockers, "statement_quality_missing") {
		t.Fatal("historical render bypassed the current semantic checklist")
	}
	materials, err := referencepg.NewReviewMaterialStore(reviews, problempg.ReadBoundReviewDraft)
	must(t, err)
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: reviewer.SubjectID,
		AuthGeneration: reviewer.AuthGeneration, RecoveryID: reviewer.RecoveryID}}
	acquire := func() (func(), error) { return func() {}, nil }
	render, err := problemreference.NewReviewMaterialReader(sessions, materials, objects, acquire)
	must(t, err)
	got, err := render.MaterialsForSubject(ctx, credential, reviewer.SubjectID, id)
	must(t, err)
	preview := got.Statement
	if preview.Version != initial.Evidence.ContentVersion || preview.DraftID != initial.Evidence.DraftID {
		t.Fatal("review materials do not belong to the local revision")
	}
	preview.Version, preview.DraftID = "", ""
	raw, err := json.Marshal(preview)
	must(t, err)
	if liveReferenceDigest(raw) != "4edd12c6982688522fce113cfe129f620159229d19ee7a4f11896ea8413f5baf" {
		t.Fatal("current review preview does not match the historical render inputs")
	}
	checklist, err := problemreference.NewStatementConfirmations(sessions, materials, objects, acquire)
	must(t, err)
	confirm := func(e problemreference.ReviewEvidence, key string) problemreference.ReviewEligibility {
		t.Helper()
		_, err := checklist.ConfirmForSubject(ctx, credential, reviewer.SubjectID, id, statementIntent(e, key))
		must(t, err)
		status, err := read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, id)
		must(t, err)
		want := slices.DeleteFunc(slices.Clone(initial.Blockers), func(s string) bool { return s == "statement_quality_missing" })
		if !status.StatementSemanticsConfirmed || status.Eligible || !slices.Equal(status.Blockers, want) {
			t.Fatalf("confirmation must clear only statement quality: %+v", status)
		}
		return status
	}
	confirmed := confirm(initial.Evidence, "live-rendering-confirm")
	views["confirmed"] = confirmed
	removed, err := reviews.SetAssignment(ctx, problemreference.ReviewAssignmentChange{ReviewID: id,
		ExpectedRevision: confirmed.Evidence.AssignmentRevision, Operator: "fixture-maintainer", Reason: "test withdrawal"})
	must(t, err)
	_, err = read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, id)
	requireReviewError(t, err, problemreference.ErrNotFound)
	_, err = reviews.SetAssignment(ctx, problemreference.ReviewAssignmentChange{ReviewID: id, ExpectedRevision: removed.Revision,
		ReviewerSubjectID: reviewer.SubjectID, ExpectedReviewerRevision: 1, Operator: "fixture-maintainer", Reason: "assign a fresh review"})
	must(t, err)
	reassigned, err := read.EligibilityForSubject(ctx, credential, reviewer.SubjectID, id)
	must(t, err)
	if reassigned.StatementSemanticsConfirmed || reassigned.Eligible || reassigned.StatementRendering == nil ||
		!slices.Equal(reassigned.Blockers, initial.Blockers) {
		t.Fatalf("fresh assignment reused a previous human confirmation: %+v", reassigned)
	}
	views["reassigned"] = reassigned
	return confirm(reassigned.Evidence, "live-rendering-reconfirm")
}
