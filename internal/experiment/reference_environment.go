package experiment

// PinnedReferenceEnvironment is the unionfind maintenance trial, independently
// identified from M0 and production contestant limits. Only cached run stdout
// uses OutputBytes; compilation and stderr retain ReferenceDiagnosticBytes.
func PinnedReferenceEnvironment() ExecutionProfile {
	p := PinnedEnvironment()
	p.ID = "p4-unionfind-gnu17-desktop-arm64-v1"
	p.Run = Limits{CPU: 1000000000, Wall: 5000000000, Memory: 128 << 20, Processes: 16}
	p.OutputBytes = 256 << 10
	return p
}

// ReferenceDiagnosticBytes bounds each compile stream and run stderr. Raw run
// stdout is cached separately so control JSON retains its original 128KiB cap.
const ReferenceDiagnosticBytes = 4096
