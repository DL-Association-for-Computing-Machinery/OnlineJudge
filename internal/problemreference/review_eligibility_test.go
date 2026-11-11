package problemreference

import (
	"slices"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestReviewApprovalRequiresMoreThanReferenceAC(t *testing.T) {
	for _, tc := range []struct {
		name, source, formal, comparison string
		plan, returned, staleProfile     bool
		want                             []string
	}{
		{name: "ordinary formal AC", source: "library-checker:aplusb", formal: "AC",
			want: []string{"independent_correctness_missing"}},
		{name: "legacy convolution", source: problemvalidation.ConvolutionSourceID, formal: "AC", plan: true,
			want: []string{"comparison_missing"}},
		{name: "convolution with no plan", source: problemvalidation.ConvolutionSourceID, formal: "AC",
			want: []string{"comparison_missing"}},
		{name: "formal AC with differing output", source: problemvalidation.ConvolutionSourceID, formal: "AC", plan: true,
			comparison: "mismatch", want: []string{"comparison_mismatch"}},
		{name: "all reference trials passed", source: problemvalidation.ConvolutionSourceID, formal: "AC", plan: true, comparison: "passed"},
		{name: "old profile", source: problemvalidation.ConvolutionSourceID, formal: "AC", plan: true,
			comparison: "passed", staleProfile: true, want: []string{"policy_unavailable"}},
		{name: "unknown source", source: "unregistered:example", formal: "AC",
			want: []string{"policy_unavailable", "independent_correctness_missing"}},
		{name: "returned with formal failure", source: problemvalidation.ConvolutionSourceID, formal: "WA", returned: true,
			want: []string{"materials_returned", "formal_failed", "comparison_missing"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _, _ := problemvalidation.ResolveSource(tc.source)
			e := ReviewEvidence{SourceID: tc.source, SourceLockSHA256: problemvalidation.SourceLockSHA256,
				ProfileSHA256: p.Digest(), FormalVerdict: tc.formal, ComparisonOutcome: tc.comparison}
			if tc.plan {
				e.ComparisonPlan = strings.Repeat("a", 64)
			}
			if tc.staleProfile {
				e.ProfileSHA256 = strings.Repeat("b", 64)
			}
			got := assessReviewEligibility(ReviewEligibilitySnapshot{Evidence: e, Returned: tc.returned})
			want := append(slices.Clone(tc.want), "statement_quality_missing", "test_coverage_missing",
				"input_validator_missing", "checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing")
			if got.Eligible || got.PolicyVersion != ReviewPolicyVersion || got.Evidence != e || !slices.Equal(got.Blockers, want) {
				t.Fatalf("unexpected policy result: eligible=%v, blockers=%v, want %v", got.Eligible, got.Blockers, want)
			}
		})
	}
}

func TestReviewApprovalWithTrustedValidatorKeepsOtherBlockers(t *testing.T) {
	const source = problemvalidation.ConvolutionSourceID
	profile, _, err := problemvalidation.ResolveSource(source)
	if err != nil {
		t.Fatal(err)
	}
	e := ReviewEvidence{SourceID: source, SourceLockSHA256: problemvalidation.SourceLockSHA256,
		ContentVersion:   "7a1079bd0d8be8d2cb57619780d4f519de096fd3bb9655eaa37a9bfe49e19e80",
		ImportRevision:   "e1c721627cd97dec382d90bfe559e766b7f3811d9257ce89946404befe009975",
		FormalPlanSHA256: "8a0d5909fce14750a809d59cece1bc0dede3f41c8dce6fba0f1ad92e371471d9",
		ProfileSHA256:    profile.Digest(), FormalVerdict: "WA", ComparisonOutcome: "mismatch"}
	s := ReviewEligibilitySnapshot{Evidence: e, Returned: true}
	got := assessReviewEligibility(s)
	want := []string{"materials_returned", "formal_failed", "comparison_mismatch", "statement_quality_missing",
		"test_coverage_missing", "checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing"}
	if got.Eligible || got.InputValidator == nil || got.IndependentCorrectness != nil || got.InputValidator.Inputs != 53 ||
		got.WrongSolution == nil || got.WrongSolution.RejectedCasePosition != 8 ||
		got.StatementRendering == nil || got.StatementRendering.FormatBlocks != 2 || !slices.Equal(got.Blockers, want) {
		t.Fatalf("expected only the validator prerequisite to clear: %+v", got)
	}
	s.Evidence.ComparisonOutcome = ""
	missing := assessReviewEligibility(s)
	if missing.IndependentCorrectness != nil || !slices.Contains(missing.Blockers, "comparison_missing") {
		t.Fatalf("convolution missed mandatory station comparison: %+v", missing)
	}
	s.Evidence.ProfileSHA256 = strings.Repeat("f", 64)
	stale := assessReviewEligibility(s)
	if stale.InputValidator != nil || stale.WrongSolution != nil || stale.StatementRendering != nil ||
		!slices.Contains(stale.Blockers, "policy_unavailable") ||
		!slices.Contains(stale.Blockers, "input_validator_missing") {
		t.Fatalf("old profile borrowed current policy evidence: %+v", stale)
	}
}

func TestReviewApprovalWithTrustedIndependentCorrectnessKeepsOtherBlockers(t *testing.T) {
	const source = "library-checker:aplusb"
	profile, _, err := problemvalidation.ResolveSource(source)
	if err != nil {
		t.Fatal(err)
	}
	e := ReviewEvidence{SourceID: source, SourceLockSHA256: problemvalidation.SourceLockSHA256,
		ContentVersion:   "87ebb9680aa3abe190b61d4a5ccac1dc83e48b29656364e445e36fabe7467cc9",
		ImportRevision:   "d19c436bc1631cb2e42b38f0036de45c100ea2f76c953df502b960287c248dd5",
		FormalPlanSHA256: "4632b8970118f8a75ac39857d31d2afb8244683892b9edc49357f9f5aefeba2a",
		ProfileSHA256:    profile.Digest(), FormalVerdict: "WA"}
	s := ReviewEligibilitySnapshot{Evidence: e, Returned: true}
	got := assessReviewEligibility(s)
	want := []string{"materials_returned", "formal_failed", "statement_quality_missing", "test_coverage_missing",
		"checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing"}
	if got.Eligible || got.IndependentCorrectness == nil || got.IndependentCorrectness.Cases != 25 || !slices.Equal(got.Blockers, want) {
		t.Fatalf("trusted source probes weakened another prerequisite: %+v", got)
	}
	s.Evidence.ProfileSHA256 = strings.Repeat("f", 64)
	stale := assessReviewEligibility(s)
	if stale.IndependentCorrectness != nil || !slices.Contains(stale.Blockers, "policy_unavailable") ||
		!slices.Contains(stale.Blockers, "independent_correctness_missing") {
		t.Fatalf("old profile borrowed current policy evidence: %+v", stale)
	}
}

func TestReviewCoverageRequiresCurrentCompleteApPlusBPlan(t *testing.T) {
	e := ReviewEvidence{
		SourceID:         "library-checker:aplusb",
		SourceLockSHA256: problemvalidation.SourceLockSHA256,
		ContentVersion:   "74c7797f5484daf9df01b1be2e5a537eba7b05b1ef20efc08110c61e3c1490fd",
		ImportRevision:   "e96d280c89a6e5d3da76ee5347c0581d4153d7052e9afceddffb2fdbb3d2036d",
		FormalPlanSHA256: "fe4e1968ae66dbfbe23afb96c03ed62dba8957d06e3b88602ebd653faf1c80f5",
		ProfileSHA256:    "658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437",
		FormalVerdict:    "AC",
	}
	s := ReviewEligibilitySnapshot{Evidence: e}
	got := assessReviewEligibility(s)
	coverage := got.TestCoverage
	want := []string{"independent_correctness_missing", "statement_quality_missing", "input_validator_missing",
		"checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing"}
	if got.Eligible || coverage == nil || !slices.Equal(got.Blockers, want) ||
		got.InputValidator != nil || got.IndependentCorrectness != nil || got.WrongSolution != nil || got.StatementRendering != nil {
		t.Fatalf("coverage must clear only its own blocker: %+v", got)
	}
	if coverage.Points != 17 || coverage.SamplePoints != 2 || coverage.NonSampleCriteria != 7 ||
		coverage.SampleOnlyCriteria != 1 || coverage.MaximumSumSamplePosition != 2 ||
		coverage.WrongSolutions[0].RejectedPoints != 14 || coverage.WrongSolutions[3].RejectedPoints != 10 {
		t.Fatalf("coverage projection drifted from the reviewed report: %+v", coverage)
	}
	for _, tc := range []struct {
		name   string
		change func(*ReviewEligibilitySnapshot)
	}{
		{"other source", func(x *ReviewEligibilitySnapshot) { x.Evidence.SourceID = problemvalidation.ConvolutionSourceID }},
		{"other source lock", func(x *ReviewEligibilitySnapshot) { x.Evidence.SourceLockSHA256 = strings.Repeat("1", 64) }},
		{"other version", func(x *ReviewEligibilitySnapshot) { x.Evidence.ContentVersion = strings.Repeat("2", 64) }},
		{"other import", func(x *ReviewEligibilitySnapshot) { x.Evidence.ImportRevision = strings.Repeat("3", 64) }},
		{"other plan", func(x *ReviewEligibilitySnapshot) { x.Evidence.FormalPlanSHA256 = strings.Repeat("4", 64) }},
		{"other profile", func(x *ReviewEligibilitySnapshot) { x.Evidence.ProfileSHA256 = strings.Repeat("5", 64) }},
		{"formal failure", func(x *ReviewEligibilitySnapshot) { x.Evidence.FormalVerdict = "WA" }},
		{"unexpected comparison", func(x *ReviewEligibilitySnapshot) {
			x.Evidence.ComparisonPlan = strings.Repeat("6", 64)
			x.Evidence.ComparisonOutcome = "mismatch"
		}},
		{"returned material", func(x *ReviewEligibilitySnapshot) { x.Returned = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := s
			tc.change(&changed)
			result := assessReviewEligibility(changed)
			if result.Eligible || result.TestCoverage != nil || !slices.Contains(result.Blockers, "test_coverage_missing") {
				t.Fatalf("coverage evidence escaped its admitted binding: %+v", result)
			}
		})
	}
}
