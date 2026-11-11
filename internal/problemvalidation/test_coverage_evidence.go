package problemvalidation

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"

// CoverageWrongSolution is a fixed candidate and its rejection count among
// non-sample points. An accepted point is not evidence of discrimination.
type CoverageWrongSolution struct {
	Candidate      string `json:"candidate"`
	RejectedPoints int    `json:"rejected_points"`
}

// TestCoverageEvidence summarizes reviewed input witnesses and discrimination
// for one frozen plan. It does not grant production or publication admission.
type TestCoverageEvidence struct {
	ReportSHA256             string                   `json:"report_sha256"`
	Points                   int                      `json:"points"`
	SamplePoints             int                      `json:"sample_points"`
	NonSampleCriteria        int                      `json:"non_sample_criteria"`
	SampleOnlyCriteria       int                      `json:"sample_only_criteria"`
	MaximumSumSamplePosition int                      `json:"maximum_sum_sample_position"`
	WrongSolutions           [4]CoverageWrongSolution `json:"wrong_solutions"`
}

// AdmittedTestCoverage only admits the reviewed A+B supplement and profile.
// The caller must first verify the stored plan and accepted report.
func AdmittedTestCoverage(id problem.ImportIdentity, planSHA256, profileSHA256 string) (TestCoverageEvidence, bool) {
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
