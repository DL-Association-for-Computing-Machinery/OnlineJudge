// Package experiment contains controlled M0 inputs, never real training accounts.
package experiment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

// Version names the synthetic A+B fixture; changing material requires a new version.
const Version = "m0-sum-v1"

// PinnedVersion retains the original inputs while fixing the execution profile.
const PinnedVersion = "m0-sum-v3"

// UbuntuVersion names the distinct fixture for the isolated Ubuntu arm64 VM.
const UbuntuVersion = "m0-sum-ubuntu-arm64-v1"

// WorkerFixture is a closed selection of executable M0 fixtures. The zero
// value preserves the existing Desktop fixture.
type WorkerFixture struct {
	ubuntu        bool
	aplusb        string
	aplusbVersion string
	aplusbTier    ReferenceTier
	localText     bool
	java21        bool
	python3       bool
}

// IsLocalText reports that the worker owns an admitted bounded text fixture.
func (f WorkerFixture) IsLocalText() bool { return f.localText }

// MaterialLimits keeps the legacy transport budgets separate from local text.
func (f WorkerFixture) MaterialLimits() (file, total int) {
	if f.localText {
		return materials.LocalTextFileBytes, materials.LocalTextTotalBytes
	}
	return materials.MaxFileBytes, 4 << 20
}

// APlusBFixture enables only the explicit, independently pinned transport
// manifest. It cannot be selected by a task or a server response.
func APlusBFixture(raw []byte, expected string) (WorkerFixture, error) {
	return aplusbFixtureForTier(raw, expected, ReferenceSmall)
}

func aplusbFixtureForTier(raw []byte, expected string, tier ReferenceTier) (WorkerFixture, error) {
	m, err := materials.ParseAPlusB(raw, expected)
	if err != nil {
		return WorkerFixture{}, err
	}
	runtimeSHA := APlusBRuntimeSHA256ForTier(tier)
	if runtimeSHA == "" || m.RuntimeSHA256 != runtimeSHA {
		return WorkerFixture{}, errors.New("A+B runtime binding mismatch")
	}
	return WorkerFixture{aplusb: string(raw), aplusbVersion: m.Version, aplusbTier: tier}, nil
}

// APlusBRuntimeSHA256 binds both the execution and cached-file budgets.
func APlusBRuntimeSHA256() string {
	return APlusBRuntimeSHA256ForTier(ReferenceSmall)
}

// APlusBRuntimeSHA256ForTier binds a supported A+B runtime and cache budget.
func APlusBRuntimeSHA256ForTier(tier ReferenceTier) string {
	if tier != ReferenceSmall && tier != ReferenceAPlusBUbuntu {
		return ""
	}
	config, err := ResolveReferenceTier(tier)
	if err != nil {
		return ""
	}
	return aplusbRuntimeDigest(config)
}

// APlusBTrialAMD64CandidateSHA256 binds offline transport without allowing
// the candidate to be chosen by the executable fixture loader.
func APlusBTrialAMD64CandidateSHA256() string {
	return aplusbRuntimeDigest(APlusBTrialAMD64Candidate())
}

func aplusbRuntimeDigest(config ReferenceConfiguration) string {
	raw, _ := json.Marshal(config)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// IsAPlusB identifies the original isolated A+B execution profile.
func (f WorkerFixture) IsAPlusB() bool { return f.UsesCachedText() && !f.IsLocalText() }

// UsesCachedText identifies a text fixture using the bounded cached executor.
func (f WorkerFixture) UsesCachedText() bool { return f.aplusb != "" }

// APlusBTier returns zero for fixtures outside the A+B lease experiment.
func (f WorkerFixture) APlusBTier() ReferenceTier { return f.aplusbTier }

// ParseWorkerFixture refuses historical and arbitrary manifests for execution.
func ParseWorkerFixture(version string) (WorkerFixture, error) {
	switch version {
	case "", PinnedVersion:
		return WorkerFixture{}, nil
	case UbuntuVersion:
		return WorkerFixture{ubuntu: true}, nil
	default:
		return WorkerFixture{}, errors.New("unsupported M0 Worker fixture")
	}
}

// Version is the version on the controller's queue and material record.
func (f WorkerFixture) Version() string {
	if f.UsesCachedText() {
		return f.aplusbVersion
	}
	if f.ubuntu {
		return UbuntuVersion
	}
	return PinnedVersion
}

// Profile returns a fresh copy of the built-in runtime definition.
func (f WorkerFixture) Profile() ExecutionProfile {
	if f.UsesCachedText() {
		config, _ := ResolveReferenceTier(f.aplusbTier)
		if f.python3 {
			return PythonTextBase(config.Runtime)
		}
		if f.java21 {
			return JavaTextBase(config.Runtime)
		}
		return config.Runtime
	}
	p := PinnedEnvironment()
	if f.ubuntu {
		p.ID = "m0-gnu17-ubuntu-arm64-v1"
		p.KernelRelease = "6.8.0-106-generic"
	}
	return p
}

// Manifest returns the canonical bytes for this execution fixture.
func (f WorkerFixture) Manifest() []byte {
	if f.UsesCachedText() {
		return []byte(f.aplusb)
	}
	return manifest(f.Version(), f.Profile())
}

// SelectFixture accepts only built-in fixtures; empty preserves the legacy default.
// This is explicit process configuration, never a caller-supplied image or command.
func SelectFixture(version string) (string, []byte, error) {
	switch version {
	case "", Version:
		return Version, Manifest(), nil
	case PinnedVersion, UbuntuVersion:
		f, err := ParseWorkerFixture(version)
		if err != nil {
			return "", nil, err
		}
		return f.Version(), f.Manifest(), nil
	case "m0-sum-v2":
		return version, manifest(version, legacyEnvironment()), nil
	default:
		return "", nil, errors.New("unknown M0 material version")
	}
}

// Manifest pins the two ordered test inputs and answers by SHA-256.
// Legacy acceptance-test material remains explicitly unvalidated forever.
func Manifest() []byte {
	return manifest(Version, "unvalidated")
}

func manifest(version string, environment any) []byte {
	hash := func(value string) string { sum := sha256.Sum256([]byte(value)); return hex.EncodeToString(sum[:]) }
	data, _ := json.Marshal(map[string]any{
		"version": version, "origin": "synthetic-project-fixture", "language": "gnu++17",
		"checker": "ascii-tokens-v1", "case_sensitive": false,
		"execution_environment": environment,
		"tests": []map[string]string{
			{"id": "01", "input_sha256": hash("1 2\n"), "answer_sha256": hash("3\n")},
			{"id": "02", "input_sha256": hash("-7 4\n"), "answer_sha256": hash("-3\n")},
		},
	})
	// Match the persisted canonical JSON: struct field order must not change the
	// digest when PostgreSQL JSONB is decoded and its object keys are sorted.
	var canonical map[string]any
	_ = json.Unmarshal(data, &canonical)
	data, _ = json.Marshal(canonical)
	return data
}
