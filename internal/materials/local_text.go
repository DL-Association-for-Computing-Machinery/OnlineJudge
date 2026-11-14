package materials

import (
	"bytes"
	"encoding/json"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

const (
	LocalTextOrigin     = "isolated-local-text-gnu17-v1"
	LocalTextFileBytes  = 4 << 20
	LocalTextTotalBytes = 48 << 20
)

// ParseLocalText binds the bounded text queue to an operator-pinned manifest.
// Runtime admission is checked separately by the executable fixture loader.
func ParseLocalText(raw []byte, expected string) (TextTransportManifest, error) {
	var m TextTransportManifest
	if len(raw) > 16384 || !digestPattern.MatchString(expected) || checksum(raw) != expected ||
		json.Unmarshal(raw, &m) != nil || !bytes.Equal(raw, m.Canonical()) ||
		m.Origin != LocalTextOrigin || m.Version != "local-text-"+m.JudgeSHA256 ||
		!digestPattern.MatchString(m.JudgeSHA256) || !digestPattern.MatchString(m.RuntimeSHA256) ||
		m.Language != "gnu++17" || m.Checker != judging.LibraryCheckerTextVersion ||
		!(judging.TextLimits{Bytes: APlusBOutputBytes, TokenBytes: m.TokenBytes}).Valid() ||
		len(m.Tests) == 0 || len(m.Tests) > 64 {
		return TextTransportManifest{}, ErrUnavailable
	}
	seen := make(map[string]bool, len(m.Tests))
	for _, point := range m.Tests {
		if !testIDPattern.MatchString(point.ID) || !digestPattern.MatchString(point.Input) ||
			!digestPattern.MatchString(point.Answer) || seen[point.ID] {
			return TextTransportManifest{}, ErrUnavailable
		}
		seen[point.ID] = true
	}
	return m, nil
}
