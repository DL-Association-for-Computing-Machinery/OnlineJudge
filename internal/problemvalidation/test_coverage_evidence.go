package problemvalidation

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"

// CoverageWrongSolution is a fixed candidate and its rejection count among
// non-sample points. An accepted point is not evidence of discrimination.
type CoverageWrongSolution struct {
	Candidate      string `json:"candidate"`
	RejectedPoints int    `json:"rejected_points"`
}

// CoverageLocalCPUProbe summarizes a discrimination run in a fixed local
// environment. It provides no target runtime or production verdict.
type CoverageLocalCPUProbe struct {
	Candidate             string `json:"candidate"`
	SmallControlsAccepted int    `json:"small_controls_accepted"`
	NonSamplePoints       int    `json:"non_sample_points"`
	CPUSoftLimitMS        int    `json:"cpu_soft_limit_ms"`
	CandidateOutcome      string `json:"candidate_outcome"`
	ReferenceOutcome      string `json:"reference_outcome"`
}

// TestCoverageEvidence summarizes reviewed input witnesses and discrimination
// for one frozen plan. It does not grant production or publication admission.
type TestCoverageEvidence struct {
	ReportSHA256             string                   `json:"report_sha256"`
	Points                   int                      `json:"points"`
	SamplePoints             int                      `json:"sample_points"`
	NonSampleCriteria        int                      `json:"non_sample_criteria"`
	SampleOnlyCriteria       int                      `json:"sample_only_criteria"`
	MaximumSumSamplePosition int                      `json:"maximum_sum_sample_position,omitempty"`
	WrongSolutions           [4]CoverageWrongSolution `json:"wrong_solutions"`
	LocalCPUProbe            *CoverageLocalCPUProbe   `json:"local_cpu_probe,omitempty"`
}

// AdmittedTestCoverage only admits exact reviewed plans and profiles.
// The caller must first verify the stored plan and current accepted report.
func AdmittedTestCoverage(id problem.ImportIdentity, planSHA256, profileSHA256 string) (TestCoverageEvidence, bool) {
	if matchesReviewedUnionFindPlan(id, planSHA256, profileSHA256) {
		return TestCoverageEvidence{
			ReportSHA256: "c7b67173c8fe054d44ea3e4b9267aa5618ec5c109e6c3cd6b006c35e12d7f9df",
			Points:       20, SamplePoints: 1, NonSampleCriteria: 10,
			WrongSolutions: [4]CoverageWrongSolution{
				{Candidate: "direct-edges-only", RejectedPoints: 10},
				{Candidate: "final-components-only", RejectedPoints: 10},
				{Candidate: "ignore-unions", RejectedPoints: 12},
				{Candidate: "self-query-false", RejectedPoints: 8},
			},
			LocalCPUProbe: &CoverageLocalCPUProbe{
				Candidate: "relabel-components", SmallControlsAccepted: 3, NonSamplePoints: 1, CPUSoftLimitMS: 5000,
				CandidateOutcome: "cpu_limit_exceeded", ReferenceOutcome: "accepted",
			},
		}, true
	}
	if !matchesReviewedApPlusBPlan(id, planSHA256, profileSHA256) {
		return TestCoverageEvidence{}, false
	}
	return TestCoverageEvidence{
		ReportSHA256: "240ca872c4bdc817b1fd1b6b82214b273f8fdcb4cd75d4b1866eebbfa5f440e2",
		Points:       17, SamplePoints: 2, NonSampleCriteria: 7, SampleOnlyCriteria: 1, MaximumSumSamplePosition: 2,
		WrongSolutions: [4]CoverageWrongSolution{
			{Candidate: "constant_zero", RejectedPoints: 14},
			{Candidate: "left_only", RejectedPoints: 13},
			{Candidate: "right_only", RejectedPoints: 13},
			{Candidate: "upstream_round_down_even", RejectedPoints: 10},
		},
	}, true
}
