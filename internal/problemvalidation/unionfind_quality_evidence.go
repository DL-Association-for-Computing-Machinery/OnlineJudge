package problemvalidation

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"

func matchesReviewedUnionFindPlan(id problem.ImportIdentity, planSHA256, profileSHA256 string) bool {
	return id.SourceID == "library-checker:unionfind" &&
		id.SourceLockSHA256 == "fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617" &&
		id.Version == "42e62825925bf727c394f3435c2b50f850758576c392d082fb628f7219e4f07d" &&
		id.ImportRevision == "98e2b6c08018d8b7228e8d4f5075a16e3238132e250ac8de6134c0308881a2a4" &&
		planSHA256 == "a82fb61254727a07b40134b0ed30ad94f952f993cd6a742e4803c737bc2a19ae" &&
		profileSHA256 == "a3f3b1fcea5d65e79003497ee98d713e4323131fb5446e08ea6482d9486c74c2"
}

// AdmittedUnionFindQuality returns both summaries from the reviewed 20-point
// run. The caller must first verify the stored plan and accepted report.
func AdmittedUnionFindQuality(id problem.ImportIdentity, planSHA256, profileSHA256 string) (InputValidatorEvidence, IndependentCorrectnessEvidence, bool) {
	if !matchesReviewedUnionFindPlan(id, planSHA256, profileSHA256) {
		return InputValidatorEvidence{}, IndependentCorrectnessEvidence{}, false
	}
	const report = "f6937f8e62284bd3514ae593126c63a8a0f57b139bdafc63437819287e6bf9db"
	return InputValidatorEvidence{ReportSHA256: report, Inputs: 20, Controls: 4},
		IndependentCorrectnessEvidence{ReportSHA256: report, Suite: admittedOracleSuite, Cases: 10}, true
}
