package experiment

import "errors"

// ReferenceTier selects a reviewed maintenance configuration, never arbitrary
// runtime budgets. Zero is not a reference tier; M0 remains a separate entry.
type ReferenceTier uint8

// ConvolutionComparisonOutputBytes bounds the fixed small-corpus solver output.
const ConvolutionComparisonOutputBytes = 512

const (
	// ReferenceSmall preserves the original unionfind configuration.
	ReferenceSmall ReferenceTier = 1
	// ReferenceMedium is the reviewed medium-material reference trial.
	ReferenceMedium ReferenceTier = 2
	// ReferenceMediumBulk retains Medium execution with a larger transfer ledger.
	ReferenceMediumBulk ReferenceTier = 3
	// ReferencePrimality admits the fixed dependency-layout trial.
	ReferencePrimality ReferenceTier = 4
	// ReferenceStaticRMQ admits one complete source with larger input transport.
	ReferenceStaticRMQ ReferenceTier = 5
	// ReferenceStaticRangeSum admits one complete source with larger raw output.
	ReferenceStaticRangeSum ReferenceTier = 6
	// ReferenceAssociativeArray admits the fixed complete large-input source.
	ReferenceAssociativeArray ReferenceTier = 7
	// ReferenceConvolution admits the fixed complete source on its own output slot.
	ReferenceConvolution ReferenceTier = 8
	// ReferenceAPlusBUbuntu admits only the standalone A+B guest experiment.
	ReferenceAPlusBUbuntu ReferenceTier = 9
	// ReferenceAPlusBTrialAMD64 is for offline material preparation only.
	// ResolveReferenceTier deliberately does not admit it for execution.
	ReferenceAPlusBTrialAMD64 ReferenceTier = 10
)

// APlusBTrialAMD64Candidate returns the proposed trial binding. This does not
// assert that the target kernel, Engine or slot has passed admission.
func APlusBTrialAMD64Candidate() ReferenceConfiguration {
	p := PinnedReferenceEnvironment()
	p.ID = "p4-aplusb-gnu17-trial-amd64-v1"
	p.Platform = "linux/amd64"
	p.KernelRelease = "6.8.0-124-generic"
	p.ImageID = "sha256:bfb42a2daf3f86c8636e3db883179852f385df1a31fd2b9f4a42d01e7e9f502c"
	p.DockerfileSHA256 = "4ed13710221459edc73bad8a52d84369dcfb4da17c5848b5f52ed1509bf20f2d"
	return ReferenceConfiguration{Runtime: p, Files: PinnedReferenceFiles()}
}

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
	case ReferenceAPlusBUbuntu:
		p := PinnedReferenceEnvironment()
		p.ID = "p4-aplusb-gnu17-ubuntu-arm64-v1"
		p.KernelRelease = "6.8.0-106-generic"
		return ReferenceConfiguration{Runtime: p, Files: PinnedReferenceFiles()}, nil
	case ReferenceMedium, ReferenceMediumBulk, ReferencePrimality, ReferenceStaticRMQ, ReferenceStaticRangeSum, ReferenceAssociativeArray, ReferenceConvolution:
		p := PinnedReferenceEnvironment()
		p.ID = "p4-reference-medium-gnu17-desktop-arm64-v1"
		p.OutputBytes = 4 << 20
		// The immutable image stores flags in Entrypoint. This new slot uses an
		// explicitly overridden entrypoint and a separately verified fixed Cmd.
		p.SandboxCmd = append([]string(nil), p.SandboxArgs[1:]...)
		for i := 0; i+1 < len(p.SandboxCmd); i++ {
			if p.SandboxCmd[i] == "-output-limit" {
				p.SandboxCmd[i+1] = "8388608"
				if tier == ReferenceConvolution {
					p.SandboxCmd[i+1] = "16777216"
				}
			}
		}
		p.SandboxArgs = []string{"/opt/go-judge"}
		f := PinnedReferenceFiles()
		f.ID = "p4-gojudge-cached-files-medium-v1"
		f.PayloadBytes = 256 << 20
		if tier == ReferenceConvolution {
			p.ID = "p4-convolution-gnu17-desktop-arm64-v1"
			p.OutputBytes = 16 << 20
			p.Run = Limits{CPU: 5000000000, Wall: 10000000000, Memory: 128 << 20, Processes: 16}
			f.ID = "p4-gojudge-cached-convolution-v1"
			f.PayloadBytes = 2 << 30
		}
		if tier == ReferenceStaticRMQ {
			p.ID = "p4-staticrmq-gnu17-desktop-arm64-v1"
			f.ID = "p4-gojudge-cached-staticrmq-v1"
			f.PayloadBytes = 512 << 20
		}
		if tier == ReferenceMediumBulk {
			f.ID = "p4-gojudge-cached-files-medium-bulk-v1"
			f.PayloadBytes = 512 << 20
		}
		if tier == ReferenceStaticRangeSum {
			p.ID = "p4-static-range-sum-gnu17-desktop-arm64-v1"
			p.OutputBytes = 8 << 20
			f.ID = "p4-gojudge-cached-static-range-sum-v1"
			f.PayloadBytes = 512 << 20
		}
		if tier == ReferenceAssociativeArray {
			p.ID = "p4-associative-array-gnu17-desktop-arm64-v1"
			p.OutputBytes = 8 << 20
			p.Run = Limits{CPU: 5000000000, Wall: 10000000000, Memory: 128 << 20, Processes: 16}
			f.ID = "p4-gojudge-cached-associative-array-v1"
			f.PayloadBytes = 1 << 30
		}
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
