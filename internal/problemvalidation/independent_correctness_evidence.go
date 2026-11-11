package problemvalidation

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"

// IndependentCorrectnessEvidence identifies the reviewed small-case oracle run.
// It does not establish full coverage or production readiness.
type IndependentCorrectnessEvidence struct {
	ReportSHA256 string `json:"report_sha256"`
	Suite        string `json:"suite"`
	Cases        int    `json:"cases"`
}

const admittedOracleSuite = "lc-small-oracles.v1"

// AdmittedIndependentCorrectness requires the exact reviewed v6 binding.
// Callers must first verify the binding in trusted stored evidence.
func AdmittedIndependentCorrectness(identity problem.ImportIdentity, planSHA256 string) (IndependentCorrectnessEvidence, bool) {
	row, ok := admittedV6Binding(identity, planSHA256)
	// Convolution has a separate station differential requirement. Its source
	// preparation probes cannot satisfy that requirement.
	if !ok || identity.SourceID == ConvolutionSourceID || row.oracleCases < 1 {
		return IndependentCorrectnessEvidence{}, false
	}
	return IndependentCorrectnessEvidence{ReportSHA256: admittedInputValidatorReport, Suite: admittedOracleSuite, Cases: row.oracleCases}, true
}
