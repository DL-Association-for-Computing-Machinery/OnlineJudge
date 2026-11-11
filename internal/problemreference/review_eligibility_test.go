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
