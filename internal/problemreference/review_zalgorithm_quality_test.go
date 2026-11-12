package problemreference

import (
	"slices"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestReviewZAlgorithmQualityBindsFullPlan(t *testing.T) {
	s := ReviewEligibilitySnapshot{Evidence: ReviewEvidence{
		SourceID: "library-checker:zalgorithm", SourceLockSHA256: problemvalidation.SourceLockSHA256,
		ContentVersion:   "924bfcbc027160298dbe976d44f4d6308268510577c4031e24871150cb044cb1",
		ImportRevision:   "ecc674ef30399bb274514a269bad4c8dae2fbf9dec989a6c8263523cfb133047",
		FormalPlanSHA256: "3128a2b2ce05ef99d9d66904460ba8e578d66e85edf83f4f40e264832331628c",
		ProfileSHA256:    "32aa64eb913108ccc089a0548a98cb777f019f37cd4bc349ab87ff0ddd1a515d",
		FormalVerdict:    "AC",
	}}
	want := []string{"statement_quality_missing", "checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing"}
	check := func(t *testing.T, got ReviewEligibility, blockers []string, covered, confirmed bool) {
		t.Helper()
		rendering := got.StatementRendering
		if rendering == nil || rendering.Examples != 4 || rendering.Formulas != 8 || rendering.FormatBlocks != 0 ||
			rendering.HistoricalMatch == nil || rendering.HistoricalMatch.ObservedContentVersion != "151da5a28d0bd733fe4837547ec909137a36b0e532409c93dfd7bf3c2a7e6b41" {
			t.Fatalf("missing bound historical rendering: %+v", rendering)
		}
		if got.Eligible || got.InputValidator == nil || got.InputValidator.Inputs != 32 || got.InputValidator.Controls != 4 ||
			got.IndependentCorrectness == nil || got.IndependentCorrectness.Cases != 62 || got.IndependentCorrectness.Suite != "lc-small-oracles.v1" ||
			got.IndependentCorrectness.ReportSHA256 != got.InputValidator.ReportSHA256 || (got.TestCoverage != nil) != covered ||
			got.StatementSemanticsConfirmed != confirmed || got.WrongSolution != nil || !slices.Equal(got.Blockers, blockers) {
			t.Fatalf("unexpected Z algorithm approval prerequisites: %+v", got)
		}
	}
	check(t, assessReviewEligibility(s), want, true, false)
	confirmed := s
	confirmed.StatementConfirmed = true
	check(t, assessReviewEligibility(confirmed), want[1:], true, true)
	for _, tc := range []struct {
		name, verdict string
		returned      bool
		blocker       string
	}{
		{"returned AC", "AC", true, "materials_returned"},
		{"formal failure", "WA", false, "formal_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := s
			changed.Returned, changed.Evidence.FormalVerdict = tc.returned, tc.verdict
			changed.StatementConfirmed = tc.returned
			check(t, assessReviewEligibility(changed), []string{tc.blocker, want[0], "test_coverage_missing", want[1], want[2], want[3]}, false, false)
		})
	}
	bad := strings.Repeat("f", 64)
	for _, tc := range []struct {
		name   string
		mutate func(*ReviewEvidence)
	}{
		{"source", func(e *ReviewEvidence) { e.SourceID = "library-checker:suffixarray" }},
		{"lock", func(e *ReviewEvidence) { e.SourceLockSHA256 = bad }},
		{"previous 31-point version", func(e *ReviewEvidence) {
			e.ContentVersion = "965239246cbdbd63f1641cfcf551d9c8f3a65cd558e194668ad845cac1e39711"
		}},
		{"revision", func(e *ReviewEvidence) { e.ImportRevision = bad }},
		{"plan", func(e *ReviewEvidence) { e.FormalPlanSHA256 = bad }},
		{"profile", func(e *ReviewEvidence) { e.ProfileSHA256 = bad }},
		{"comparison plan", func(e *ReviewEvidence) { e.ComparisonPlan = bad }},
		{"comparison outcome", func(e *ReviewEvidence) { e.ComparisonOutcome = "passed" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := s
			tc.mutate(&changed.Evidence)
			got := assessReviewEligibility(changed)
			if got.Eligible || got.InputValidator != nil || got.IndependentCorrectness != nil || got.TestCoverage != nil || got.StatementRendering != nil ||
				!slices.Contains(got.Blockers, "statement_quality_missing") ||
				!slices.Contains(got.Blockers, "test_coverage_missing") ||
				!slices.Contains(got.Blockers, "input_validator_missing") || !slices.Contains(got.Blockers, "independent_correctness_missing") {
				t.Fatalf("unexpected admission on mismatched evidence: %+v", got)
			}
		})
	}
}
