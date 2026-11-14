package experiment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
)

const SPJObjectBytes = 256 << 10
const SPJArtifactBytes = 1 << 20
const SPJHeaderSHA256 = "529f92008bc444628ac657da3e5245ebf2bbfdf37c24a88d80114dec14728015"

// SPJLimits returns the fixed checker budget separate from contestant execution.
func SPJLimits() Limits {
	return Limits{CPU: 1000000000, Wall: 4000000000, Memory: 128 << 20, Processes: 8}
}

// SPJFiles bounds the separate checker ledger. The contestant ledger never
// stores these objects. Sixteen points need at most 210 checker operations.
func SPJFiles() ReferenceFilesProfile {
	return ReferenceFilesProfile{ID: "p7-spj-files-v1", FileBytes: SPJArtifactBytes,
		LiveBytes:    SPJArtifactBytes + 3*SPJObjectBytes,
		PayloadBytes: 56 << 20, Operations: 224, Objects: 4}
}

// LocalSPJRuntimeSHA256 binds the executor image and the checker invocation.
func LocalSPJRuntimeSHA256() string {
	tier, _ := ResolveReferenceTier(ReferenceAPlusBUbuntu)
	tier.Runtime = PythonTextBase(tier.Runtime)
	raw, _ := json.Marshal(struct {
		Tier                         ReferenceConfiguration
		Run                          Limits
		Args                         [4]string
		Header                       string
		Files                        ReferenceFilesProfile
		Input, Artifact, Diagnostics int
	}{tier, SPJLimits(), SPJArgs(), SPJHeaderSHA256, SPJFiles(), SPJObjectBytes, SPJArtifactBytes, ReferenceDiagnosticBytes})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// SPJArgs names only the current point's three private materials.
func SPJArgs() [4]string { return [4]string{"./checker", "input", "output", "answer"} }

// WithCheckerArtifact admits only a matching operator-provisioned local artifact.
func (f WorkerFixture) WithCheckerArtifact(path string) (WorkerFixture, error) {
	if !f.UsesSPJ() || !f.Python3Enabled() {
		return WorkerFixture{}, errors.New("SPJ requires a fixed combined sandbox and manifest")
	}
	raw, err := checkedfile.ReadSecret(path, SPJArtifactBytes)
	if err != nil {
		return WorkerFixture{}, err
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != f.checkerSHA {
		return WorkerFixture{}, errors.New("checker artifact digest mismatch")
	}
	f.checker = string(raw)
	return f, nil
}
