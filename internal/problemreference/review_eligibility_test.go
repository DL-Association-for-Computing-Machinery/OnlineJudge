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
	if got.Eligible || got.InputValidator == nil || got.InputValidator.Inputs != 53 || !slices.Equal(got.Blockers, want) {
		t.Fatalf("expected only the validator prerequisite to clear: %+v", got)
	}
	s.Evidence.ProfileSHA256 = strings.Repeat("f", 64)
	stale := assessReviewEligibility(s)
	if stale.InputValidator != nil || !slices.Contains(stale.Blockers, "policy_unavailable") ||
		!slices.Contains(stale.Blockers, "input_validator_missing") {
		t.Fatalf("old profile borrowed current policy evidence: %+v", stale)
	}
}
