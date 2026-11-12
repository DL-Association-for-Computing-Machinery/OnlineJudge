package problemreference

import (
	"encoding/json"
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

func TestReviewUnionFindCoverageRequiresCurrentCompletePlan(t *testing.T) {
	s := ReviewEligibilitySnapshot{Evidence: ReviewEvidence{
		SourceID:         "library-checker:unionfind",
		SourceLockSHA256: problemvalidation.SourceLockSHA256,
		ContentVersion:   "42e62825925bf727c394f3435c2b50f850758576c392d082fb628f7219e4f07d",
		ImportRevision:   "98e2b6c08018d8b7228e8d4f5075a16e3238132e250ac8de6134c0308881a2a4",
		FormalPlanSHA256: "a82fb61254727a07b40134b0ed30ad94f952f993cd6a742e4803c737bc2a19ae",
		ProfileSHA256:    "a3f3b1fcea5d65e79003497ee98d713e4323131fb5446e08ea6482d9486c74c2",
		FormalVerdict:    "AC",
	}}
	want := []string{"statement_quality_missing", "checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing"}
	quality := func(t *testing.T, result ReviewEligibility) {
		t.Helper()
		if result.InputValidator == nil || result.InputValidator.Inputs != 20 || result.InputValidator.Controls != 4 ||
			result.IndependentCorrectness == nil || result.IndependentCorrectness.Cases != 10 ||
			result.IndependentCorrectness.ReportSHA256 != result.InputValidator.ReportSHA256 ||
			result.StatementRendering == nil || result.StatementRendering.HistoricalMatch == nil ||
			result.StatementRendering.Examples != 1 || result.StatementRendering.Formulas != 10 || result.StatementRendering.FormatBlocks != 0 ||
			result.WrongSolution != nil || result.Eligible {
			t.Fatalf("expected the bound revised quality and rendering evidence: %+v", result)
		}
	}
	got := assessReviewEligibility(s)
	quality(t, got)
	if got.TestCoverage == nil || got.TestCoverage.LocalCPUProbe == nil || got.StatementSemanticsConfirmed || !slices.Equal(got.Blockers, want) {
		t.Fatalf("coverage should clear only its own prerequisite: %+v", got)
	}
	confirmed := s
	confirmed.StatementConfirmed = true
	ready := assessReviewEligibility(confirmed)
	quality(t, ready)
	if !ready.StatementSemanticsConfirmed || ready.TestCoverage == nil || !slices.Equal(ready.Blockers, want[1:]) {
		t.Fatalf("current confirmation must clear only the statement blocker: %+v", ready)
	}
	fields := requireCoverageJSONFields(t, got.TestCoverage, "local_cpu_probe")
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(fields["local_cpu_probe"], &probe); err != nil {
		t.Fatal(err)
	}
	keys := []string{"candidate", "small_controls_accepted", "non_sample_points", "cpu_soft_limit_ms", "candidate_outcome", "reference_outcome"}
	if len(probe) != len(keys) {
		t.Fatalf("unexpected fields in CPU summary: %v", probe)
	}
	for _, key := range keys {
		if _, ok := probe[key]; !ok {
			t.Fatalf("CPU summary omitted %s", key)
		}
	}
	for _, tc := range []struct {
		name, verdict string
		returned      bool
		blockers      []string
	}{
		{"returned", "AC", true, []string{"materials_returned"}},
		{"failed", "WA", false, []string{"formal_failed"}},
		{"returned and failed", "WA", true, []string{"materials_returned", "formal_failed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := s
			changed.Returned, changed.Evidence.FormalVerdict = tc.returned, tc.verdict
			changed.StatementConfirmed = tc.returned
			result := assessReviewEligibility(changed)
			quality(t, result)
			expected := append(slices.Clone(tc.blockers), "statement_quality_missing", "test_coverage_missing",
				"checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing")
			if result.TestCoverage != nil || result.StatementSemanticsConfirmed || !slices.Equal(result.Blockers, expected) {
				t.Fatalf("historical evidence weakened current blockers: %+v, want %v", result, expected)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*ReviewEvidence)
	}{
		{"source", func(e *ReviewEvidence) { e.SourceID = "library-checker:aplusb" }},
		{"lock", func(e *ReviewEvidence) { e.SourceLockSHA256 = strings.Repeat("1", 64) }},
		{"version", func(e *ReviewEvidence) { e.ContentVersion = strings.Repeat("2", 64) }},
		{"import revision", func(e *ReviewEvidence) { e.ImportRevision = strings.Repeat("3", 64) }},
		{"formal plan", func(e *ReviewEvidence) { e.FormalPlanSHA256 = strings.Repeat("4", 64) }},
		{"profile", func(e *ReviewEvidence) { e.ProfileSHA256 = strings.Repeat("5", 64) }},
		{"comparison plan", func(e *ReviewEvidence) { e.ComparisonPlan = strings.Repeat("6", 64) }},
		{"comparison outcome", func(e *ReviewEvidence) { e.ComparisonOutcome = "passed" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := s
			tc.change(&changed.Evidence)
			result := assessReviewEligibility(changed)
			if result.InputValidator != nil || result.IndependentCorrectness != nil || result.Eligible || result.TestCoverage != nil ||
				result.StatementRendering != nil || !slices.Contains(result.Blockers, "statement_quality_missing") ||
				!slices.Contains(result.Blockers, "test_coverage_missing") ||
				!slices.Contains(result.Blockers, "input_validator_missing") ||
				!slices.Contains(result.Blockers, "independent_correctness_missing") {
				t.Fatalf("unexpected quality evidence for a different binding: %+v", result)
			}
		})
	}
	legacy := s
	legacy.Evidence.ContentVersion = "fd871a02594fcd663790cd0530153df2ad18bf95110559ce552a121c34705bed"
	legacy.Evidence.ImportRevision = "14b95a5dcfe72e5be314ed8b7739e1330057a2bfbf85cdc3bfac30904fb557c7"
	legacy.Evidence.FormalPlanSHA256 = "d64b9c33fd865c0433ba7b932ccfd6c9184793512a271c9a618b510a05b1d37d"
	old := assessReviewEligibility(legacy)
	if old.InputValidator == nil || old.InputValidator.Inputs != 18 || old.IndependentCorrectness == nil || old.TestCoverage != nil ||
		old.StatementRendering == nil || old.StatementRendering.HistoricalMatch != nil ||
		old.IndependentCorrectness.Cases != 10 || old.InputValidator.ReportSHA256 == got.InputValidator.ReportSHA256 ||
		old.InputValidator.ReportSHA256 != old.IndependentCorrectness.ReportSHA256 {
		t.Fatalf("legacy plan no longer reads its own quality evidence: %+v", old)
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
	want := []string{"statement_quality_missing", "checker_admission_missing", "target_runtime_missing", "solution_confirmation_missing"}
	if got.Eligible || coverage == nil || !slices.Equal(got.Blockers, want) ||
		got.InputValidator == nil || got.IndependentCorrectness == nil || got.WrongSolution != nil || got.StatementRendering == nil ||
		got.StatementRendering.HistoricalMatch == nil || got.StatementSemanticsConfirmed {
		t.Fatalf("rendering evidence must not replace the human checklist: %+v", got)
	}
	confirmed := s
	confirmed.StatementConfirmed = true
	ready := assessReviewEligibility(confirmed)
	if ready.Eligible || !ready.StatementSemanticsConfirmed || !slices.Equal(ready.Blockers, want[1:]) {
		t.Fatalf("current confirmation must clear only the statement blocker: %+v", ready)
	}
	if got.InputValidator.Inputs != 17 || got.InputValidator.Controls != 4 || got.IndependentCorrectness.Cases != 25 ||
		got.InputValidator.ReportSHA256 != got.IndependentCorrectness.ReportSHA256 ||
		got.InputValidator.ReportSHA256 == coverage.ReportSHA256 {
		t.Fatalf("the two quality summaries must refer to the new probe report, not the coverage report: %+v", got)
	}
	if coverage.Points != 17 || coverage.SamplePoints != 2 || coverage.NonSampleCriteria != 7 ||
		coverage.SampleOnlyCriteria != 1 || coverage.MaximumSumSamplePosition != 2 ||
		coverage.WrongSolutions[0].RejectedPoints != 14 || coverage.WrongSolutions[3].RejectedPoints != 10 {
		t.Fatalf("coverage projection drifted from the reviewed report: %+v", coverage)
	}
	requireCoverageJSONFields(t, coverage, "maximum_sum_sample_position")
	for _, tc := range []struct {
		name              string
		change            func(*ReviewEligibilitySnapshot)
		historicalBlocker string
	}{
		{"other source", func(x *ReviewEligibilitySnapshot) { x.Evidence.SourceID = problemvalidation.ConvolutionSourceID }, ""},
		{"other source lock", func(x *ReviewEligibilitySnapshot) { x.Evidence.SourceLockSHA256 = strings.Repeat("1", 64) }, ""},
		{"other version", func(x *ReviewEligibilitySnapshot) { x.Evidence.ContentVersion = strings.Repeat("2", 64) }, ""},
		{"other import", func(x *ReviewEligibilitySnapshot) { x.Evidence.ImportRevision = strings.Repeat("3", 64) }, ""},
		{"other plan", func(x *ReviewEligibilitySnapshot) { x.Evidence.FormalPlanSHA256 = strings.Repeat("4", 64) }, ""},
		{"other profile", func(x *ReviewEligibilitySnapshot) { x.Evidence.ProfileSHA256 = strings.Repeat("5", 64) }, ""},
		{"formal failure", func(x *ReviewEligibilitySnapshot) { x.Evidence.FormalVerdict = "WA" }, "formal_failed"},
		{"unexpected comparison", func(x *ReviewEligibilitySnapshot) {
			x.Evidence.ComparisonPlan = strings.Repeat("6", 64)
			x.Evidence.ComparisonOutcome = "mismatch"
		}, ""},
		{"returned material", func(x *ReviewEligibilitySnapshot) { x.Returned = true }, "materials_returned"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := s
			tc.change(&changed)
			result := assessReviewEligibility(changed)
			if result.Eligible || result.TestCoverage != nil || !slices.Contains(result.Blockers, "test_coverage_missing") {
				t.Fatalf("coverage evidence escaped its admitted binding: %+v", result)
			}
			if tc.historicalBlocker != "" {
				if result.InputValidator == nil || result.IndependentCorrectness == nil || result.StatementRendering == nil ||
					!slices.Contains(result.Blockers, tc.historicalBlocker) {
					t.Fatalf("historical checks must not replace the current prerequisite: %+v", result)
				}
			} else if result.InputValidator != nil || result.IndependentCorrectness != nil || result.StatementRendering != nil ||
				!slices.Contains(result.Blockers, "input_validator_missing") {
				t.Fatalf("quality evidence escaped its binding: %+v", result)
			}
		})
	}
}

func requireCoverageJSONFields(t *testing.T, coverage *problemvalidation.TestCoverageEvidence, sourceSpecific string) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(coverage)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	keys := []string{"report_sha256", "points", "sample_points", "non_sample_criteria", "sample_only_criteria", "wrong_solutions", sourceSpecific}
	if len(fields) != len(keys) {
		t.Fatalf("unexpected coverage response fields: %s", raw)
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			t.Fatalf("coverage response omitted %s", key)
		}
	}
	return fields
}
