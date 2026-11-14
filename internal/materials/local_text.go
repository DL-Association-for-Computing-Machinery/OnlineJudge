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
	LocalSPJChecker     = "testlib-0.9.45-oj-binary-v1"
	LocalSPJPoints      = 16
)

// ParseLocalText binds the bounded text queue to an operator-pinned manifest.
// Runtime admission is checked separately by the executable fixture loader.
func ParseLocalText(raw []byte, expected string) (TextTransportManifest, error) {
	var m TextTransportManifest
	if len(raw) > 16384 || !digestPattern.MatchString(expected) || checksum(raw) != expected ||
		json.Unmarshal(raw, &m) != nil || !bytes.Equal(raw, m.Canonical()) ||
		m.Origin != LocalTextOrigin || m.Version != "local-text-"+m.JudgeSHA256 ||
		!digestPattern.MatchString(m.JudgeSHA256) || !digestPattern.MatchString(m.RuntimeSHA256) ||
		m.Language != "gnu++17" || !m.validLocalChecker() ||
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

func (m TextTransportManifest) validLocalChecker() bool {
	switch m.Checker {
	case judging.LibraryCheckerTextVersion:
		return m.Decimal == nil && m.CheckerArtifact == "" && (judging.TextLimits{Bytes: APlusBOutputBytes, TokenBytes: m.TokenBytes}).Valid()
	case judging.DecimalVersion:
		return m.Decimal != nil && m.Decimal.Valid() && m.TokenBytes == 0 && m.CheckerArtifact == ""
	case LocalSPJChecker:
		return m.Decimal == nil && m.TokenBytes == 0 && digestPattern.MatchString(m.CheckerArtifact) && len(m.Tests) <= LocalSPJPoints
	default:
		return false
	}
}
