package experiment

import "errors"

// ReferenceTier selects a reviewed maintenance configuration, never arbitrary
// runtime budgets. Zero is not a reference tier; M0 remains a separate entry.
type ReferenceTier uint8

const (
	// ReferenceSmall preserves the original unionfind configuration.
	ReferenceSmall ReferenceTier = 1
	// ReferenceMedium is the reviewed medium-material reference trial.
	ReferenceMedium ReferenceTier = 2
	// ReferencePrimality admits the fixed dependency-layout trial.
	ReferencePrimality ReferenceTier = 4
)

// ReferenceConfiguration binds runtime and both file-transport budget ledgers.
// ResolveReferenceTier always returns fresh slices, safe to detach for reports.
type ReferenceConfiguration struct {
	Runtime ExecutionProfile
	Files   ReferenceFilesProfile
	Entry   string
}

// ResolveReferenceTier accepts only the closed set of reviewed configurations.
func ResolveReferenceTier(tier ReferenceTier) (ReferenceConfiguration, error) {
	switch tier {
	case ReferenceSmall:
		return ReferenceConfiguration{Runtime: PinnedReferenceEnvironment(), Files: PinnedReferenceFiles()}, nil
	case ReferenceMedium, ReferencePrimality:
		p := PinnedReferenceEnvironment()
		p.ID = "p4-reference-medium-gnu17-desktop-arm64-v1"
		p.OutputBytes = 4 << 20
		// The immutable image stores flags in Entrypoint. This new slot uses an
		// explicitly overridden entrypoint and a separately verified fixed Cmd.
		p.SandboxCmd = append([]string(nil), p.SandboxArgs[1:]...)
		for i := 0; i+1 < len(p.SandboxCmd); i++ {
			if p.SandboxCmd[i] == "-output-limit" {
				p.SandboxCmd[i+1] = "8388608"
			}
		}
		p.SandboxArgs = []string{"/opt/go-judge"}
		f := PinnedReferenceFiles()
		f.ID = "p4-gojudge-cached-files-medium-v1"
		f.PayloadBytes = 256 << 20
		if tier == ReferencePrimality {
			p.ID = "p4-primality-gnu17-desktop-arm64-v1"
			p.OutputBytes = 512 << 10
			p.CompileArgs = []string{"/usr/local/bin/g++", "-std=gnu++17", "-O2", "-pipe", "sol/correct.cpp", "-Icommon", "-o", "main"}
			f.ID = "p4-gojudge-cached-dependencies-v1"
			f.Objects = 6 // five compilation files and the reserved executable
			return ReferenceConfiguration{Runtime: p, Files: f, Entry: "sol/correct.cpp"}, nil
		}
		return ReferenceConfiguration{Runtime: p, Files: f}, nil
	default:
		return ReferenceConfiguration{}, errors.New("unsupported reference tier")
	}
}
