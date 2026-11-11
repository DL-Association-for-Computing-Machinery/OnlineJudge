//go:build integration && linux

package integration_test

import (
	"os"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestV6IndependentCorrectnessThroughReview(t *testing.T) {
	dir := os.Getenv("OJ_V6_INPUT_EVIDENCE_BUNDLE")
	if dir == "" {
		t.Skip("requires the verified Library Checker v6 private bundle")
	}
	lock, err := os.ReadFile("../../config/problem-sources/library-checker.json")
	must(t, err)
	p, err := problem.LoadBundle(dir, "library-checker:aplusb", lock)
	must(t, err)
	t.Cleanup(func() { must(t, p.Close()) })
	f := expectedPlanFixtureFor(t, expectedPlanFixture{referenceRequestFixture: referenceRequestFixture{draftFixture: newDraftFixture(t)}}, p)
	claims, node, selection := claimFixtureFor(t, f)
	prepareClaim(t, f, expectedPlanKey)
	claim, err := claims.ClaimReference(f.ctx, node, selection)
	must(t, err)
	// Deliberately accept a formal failure; the v6 source probes must not
	// override a later real database observation of a failed reference trial.
	must(t, claims.ReportReference(f.ctx, node, workerReportForClaim(t, f, claim, "WA")))
	item, err := f.store.SubmitReview(f.ctx, f.actor, reviewIntent(f))
	must(t, err)
	reviewer := f.actor
	reviewer.SubjectID = identityID(1101)
	_, err = f.access.SetReviewerRole(f.ctx, identity.ProblemRoleChange{SubjectID: reviewer.SubjectID, Enabled: true,
		Operator: "fixture-maintainer", Reason: "verify independent correctness read"})
	must(t, err)
	store, err := referencepg.NewReviewStore(f.b, f.access)
	must(t, err)
	_, err = store.SetAssignment(f.ctx, problemreference.ReviewAssignmentChange{ReviewID: item.ID, ReviewerSubjectID: reviewer.SubjectID,
		ExpectedReviewerRevision: 1, Operator: "fixture-maintainer", Reason: "assign synthetic non-author"})
	must(t, err)
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: reviewer.SubjectID, AuthGeneration: reviewer.AuthGeneration, RecoveryID: reviewer.RecoveryID}}
	access, err := problemreference.NewReviewAccess(sessions, store)
	must(t, err)
	got, err := access.EligibilityForSubject(f.ctx, f.credential, reviewer.SubjectID, item.ID)
	must(t, err)
	ev := got.IndependentCorrectness
	wrong := got.WrongSolution
	if got.Eligible || ev == nil || ev.Suite != "lc-small-oracles.v1" || ev.Cases != 25 ||
		!slices.Contains(got.Blockers, "formal_failed") || !slices.Contains(got.Blockers, "test_coverage_missing") ||
		slices.Contains(got.Blockers, "independent_correctness_missing") {
		t.Fatalf("review did not retain separate formal and coverage blockers: %+v", got)
	}
	if wrong == nil || wrong.Origin != "upstream" || wrong.Candidate != "wa.cpp" || wrong.RejectedCasePosition != 4 ||
		wrong.AcceptedCasePositions == nil || len(wrong.AcceptedCasePositions) != 0 || wrong.ReportSHA256 != ev.ReportSHA256 {
		t.Fatalf("assigned review did not return the v6 wrong solution witness: %+v", wrong)
	}
	rendering := got.StatementRendering
	if rendering == nil || rendering.Examples != 2 || rendering.Formulas != 6 || rendering.FormatBlocks != 2 ||
		!slices.Contains(got.Blockers, "statement_quality_missing") {
		t.Fatalf("browser evidence must remain separate from semantic review: %+v", got)
	}
}
