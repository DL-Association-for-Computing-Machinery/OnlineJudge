package problemvalidation

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"

func matchesReviewedApPlusBPlan(id problem.ImportIdentity, planSHA256, profileSHA256 string) bool {
	return id.SourceID == "library-checker:aplusb" &&
		id.SourceLockSHA256 == "fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617" &&
		id.Version == "74c7797f5484daf9df01b1be2e5a537eba7b05b1ef20efc08110c61e3c1490fd" &&
		id.ImportRevision == "e96d280c89a6e5d3da76ee5347c0581d4153d7052e9afceddffb2fdbb3d2036d" &&
		planSHA256 == ReviewedAPlusBPlan &&
		profileSHA256 == "658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437"
}

// AdmittedApPlusBQuality returns the input checks and small-case results from
// the same reviewed run. It does not grant evidence from the original v6 bundle.
// The caller must first verify the stored plan and accepted report.
func AdmittedApPlusBQuality(id problem.ImportIdentity, planSHA256, profileSHA256 string) (InputValidatorEvidence, IndependentCorrectnessEvidence, bool) {
	if !matchesReviewedApPlusBPlan(id, planSHA256, profileSHA256) {
		return InputValidatorEvidence{}, IndependentCorrectnessEvidence{}, false
	}
	const report = "70c7834df6a86220da0cb8a9e6b30f7015832ff082cdeaa45eced15579107bc0"
	return InputValidatorEvidence{ReportSHA256: report, Inputs: 17, Controls: 4},
		IndependentCorrectnessEvidence{ReportSHA256: report, Suite: admittedOracleSuite, Cases: 25}, true
}
