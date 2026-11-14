package materials

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

const APlusBOrigin = "isolated-aplusb-gnu17-v1"
const APlusBOutputBytes = 262144

// TextTransportManifest is the registered transport projection of an independently
// reviewed judge manifest. Its own digest pins every transported object.
type TextTransportManifest struct {
	Checker         string                 `json:"checker"`
	CheckerArtifact string                 `json:"checker_artifact_sha256,omitempty"`
	Decimal         *judging.DecimalPolicy `json:"decimal,omitempty"`
	JudgeSHA256     string                 `json:"judge_sha256"`
	Language        string                 `json:"language"`
	Origin          string                 `json:"origin"`
	RuntimeSHA256   string                 `json:"runtime_sha256"`
	Tests           []TextTransportPoint   `json:"tests"`
	TokenBytes      int                    `json:"token_bytes"`
	Version         string                 `json:"version"`
}

// TextTransportPoint contains two private object identities without their bytes.
type TextTransportPoint struct {
	Answer string `json:"answer_sha256"`
	ID     string `json:"id"`
	Input  string `json:"input_sha256"`
}

// APlusBManifest retains the name of the original fixed text transport.
type APlusBManifest = TextTransportManifest

// APlusBPoint retains the original name for its private object references.
type APlusBPoint = TextTransportPoint

var aplusbVersion = regexp.MustCompile(`^aplusb-[a-f0-9]{64}$`)

// Canonical matches the key ordering of the JSONB readback encoder.
func (m TextTransportManifest) Canonical() []byte {
	raw, _ := json.Marshal(m)
	return raw
}

// ParseAPlusB accepts only canonical bounded metadata matching the trusted pin.
func ParseAPlusB(raw []byte, expected string) (APlusBManifest, error) {
	var m APlusBManifest
	if len(raw) > 16384 || !digestPattern.MatchString(expected) || checksum(raw) != expected ||
		json.Unmarshal(raw, &m) != nil || !bytes.Equal(raw, m.Canonical()) ||
		!aplusbVersion.MatchString(m.Version) || m.Version != "aplusb-"+m.JudgeSHA256 ||
		m.Checker != judging.LibraryCheckerTextVersion || m.CheckerArtifact != "" || m.Decimal != nil || !digestPattern.MatchString(m.RuntimeSHA256) ||
		m.Origin != APlusBOrigin || m.Language != "gnu++17" || m.TokenBytes < 1 || m.TokenBytes > 65536 ||
		len(m.Tests) != 17 {
		return APlusBManifest{}, ErrUnavailable
	}
	seen := make(map[string]bool)
	for _, point := range m.Tests {
		if !testIDPattern.MatchString(point.ID) || !digestPattern.MatchString(point.Input) ||
			!digestPattern.MatchString(point.Answer) || seen[point.ID] {
			return APlusBManifest{}, ErrUnavailable
		}
		seen[point.ID] = true
	}
	return m, nil
}

// LoadAPlusB uses explicit owner-private configuration with no fallback.
func LoadAPlusB(getenv func(string) string) (m APlusBManifest, raw []byte, enabled bool, err error) {
	path, digest := getenv("OJ_APLUSB_MANIFEST_FILE"), getenv("OJ_APLUSB_MANIFEST_SHA256")
	if path == "" && digest == "" {
		return m, nil, false, nil
	}
	if getenv("OJ_SANDBOX_EXPERIMENT") != "1" || getenv("OJ_EXPERIMENT_MATERIAL_VERSION") != "" {
		return m, nil, true, ErrUnavailable
	}
	raw, err = checkedfile.ReadSecret(path, 16384)
	if err != nil {
		return m, nil, true, err
	}
	m, err = ParseAPlusB(raw, digest)
	return m, raw, true, err
}

// ConfiguredAPlusB loads the operator's explicit environment.
func ConfiguredAPlusB() (APlusBManifest, []byte, bool, error) { return LoadAPlusB(os.Getenv) }
