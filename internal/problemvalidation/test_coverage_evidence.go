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
	const lock = "fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617"
	if id.SourceID != "library-checker:aplusb" || id.SourceLockSHA256 != lock ||
		id.Version != "74c7797f5484daf9df01b1be2e5a537eba7b05b1ef20efc08110c61e3c1490fd" ||
		id.ImportRevision != "e96d280c89a6e5d3da76ee5347c0581d4153d7052e9afceddffb2fdbb3d2036d" ||
		planSHA256 != "fe4e1968ae66dbfbe23afb96c03ed62dba8957d06e3b88602ebd653faf1c80f5" ||
		profileSHA256 != "658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437" {
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
