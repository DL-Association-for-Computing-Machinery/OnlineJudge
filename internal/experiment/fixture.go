// Package experiment contains controlled M0 inputs, never real training accounts.
package experiment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Version names the synthetic A+B fixture; changing material requires a new version.
const Version = "m0-sum-v1"

// PinnedVersion retains the original inputs while fixing the execution profile.
const PinnedVersion = "m0-sum-v3"

// UbuntuVersion names the distinct fixture for the isolated Ubuntu arm64 VM.
const UbuntuVersion = "m0-sum-ubuntu-arm64-v1"

// WorkerFixture is a closed selection of executable M0 fixtures. The zero
// value preserves the existing Desktop fixture.
type WorkerFixture struct{ ubuntu bool }

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
	if f.ubuntu {
		return UbuntuVersion
	}
	return PinnedVersion
}

// Profile returns a fresh copy of the built-in runtime definition.
func (f WorkerFixture) Profile() ExecutionProfile {
	p := PinnedEnvironment()
	if f.ubuntu {
		p.ID = "m0-gnu17-ubuntu-arm64-v1"
		p.KernelRelease = "6.8.0-106-generic"
	}
	return p
}

// Manifest returns the canonical bytes for this execution fixture.
func (f WorkerFixture) Manifest() []byte { return manifest(f.Version(), f.Profile()) }

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
