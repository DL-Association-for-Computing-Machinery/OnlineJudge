package problemvalidation

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"

func matchesReviewedZAlgorithmPlan(id problem.ImportIdentity, planSHA256, profileSHA256 string) bool {
	return id.SourceID == "library-checker:zalgorithm" &&
		id.SourceLockSHA256 == "fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617" &&
		id.Version == "924bfcbc027160298dbe976d44f4d6308268510577c4031e24871150cb044cb1" &&
		id.ImportRevision == "ecc674ef30399bb274514a269bad4c8dae2fbf9dec989a6c8263523cfb133047" &&
		planSHA256 == "3128a2b2ce05ef99d9d66904460ba8e578d66e85edf83f4f40e264832331628c" &&
		profileSHA256 == "32aa64eb913108ccc089a0548a98cb777f019f37cd4bc349ab87ff0ddd1a515d"
}

// AdmittedZAlgorithmQuality returns both summaries for the reviewed 32-point
// revision. The caller must verify the stored plan and accepted report first.
func AdmittedZAlgorithmQuality(id problem.ImportIdentity, planSHA256, profileSHA256 string) (InputValidatorEvidence, IndependentCorrectnessEvidence, bool) {
	if !matchesReviewedZAlgorithmPlan(id, planSHA256, profileSHA256) {
		return InputValidatorEvidence{}, IndependentCorrectnessEvidence{}, false
	}
	const report = "19e3ca6ab1c5a22337993ce1ff8c324fb39b060978555b5020ea27f9b8f05176"
	return InputValidatorEvidence{ReportSHA256: report, Inputs: 32, Controls: 4},
		IndependentCorrectnessEvidence{ReportSHA256: report, Suite: admittedOracleSuite, Cases: 62}, true
}
