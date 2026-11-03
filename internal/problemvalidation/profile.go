// Package problemvalidation validates a complete private reference program in
// an explicitly verified sandbox. It does not publish or change database state.
package problemvalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

// SourceLockSHA256 is the reviewed checked-in LC lock for the maintenance entry.
// A changed source lock requires a reviewed admission update, never a CLI override.
const SourceLockSHA256 = "fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617"

// Profile separates the reference trial from production contestant limits.
type Profile struct {
	ID          string                           `json:"id"`
	Runtime     experiment.ExecutionProfile      `json:"runtime"`
	Files       experiment.ReferenceFilesProfile `json:"files"`
	Materials   problem.ReferenceLimits          `json:"materials"`
	Checker     string                           `json:"checker"`
	Text        judging.TextLimits               `json:"text"`
	Wall        time.Duration                    `json:"wall_ns"`
	Diagnostics int                              `json:"diagnostic_bytes,omitempty"`
}

// PinnedProfile returns fresh trial values; it never updates draft configuration.
func PinnedProfile() Profile {
	return Profile{ID: "p4-lc-reference-files-small-desktop-arm64-v1", Runtime: experiment.PinnedEnvironment(), Files: experiment.PinnedReferenceFiles(), Materials: problem.ReferenceLimits{SourceBytes: 64 << 10, InputBytes: 16 << 10, AnswerBytes: 4096, TotalBytes: 2 << 20, Cases: 64}, Checker: judging.LibraryCheckerTextVersion, Text: judging.TextLimits{Bytes: 4096, TokenBytes: 4096}, Wall: 2 * time.Minute}
}

// UnionFindProfile admits the complete unionfind maintenance trial only. Its
// resource choices do not modify draft production limits or M0 materials.
func UnionFindProfile() Profile {
	p := PinnedProfile()
	p.ID = "p4-lc-unionfind-reference-desktop-arm64-v1"
	p.Runtime = experiment.PinnedReferenceEnvironment()
	p.Materials = problem.UnionFindReferenceLimits()
	p.Text = judging.TextLimits{Bytes: 256 << 10, TokenBytes: 4096}
	p.Wall = 3 * time.Minute
	p.Diagnostics = experiment.ReferenceDiagnosticBytes
	return p
}

// ResolveSource binds the closed set of maintenance sources to exact profiles.
// Tier zero is only the legacy A+B/M0 entry; reference tier factories reject it.
func ResolveSource(sourceID string) (Profile, experiment.ReferenceTier, error) {
	switch sourceID {
	case "library-checker:aplusb":
		return PinnedProfile(), 0, nil
	case "library-checker:unionfind":
		return UnionFindProfile(), experiment.ReferenceSmall, nil
	case "library-checker:zalgorithm", "library-checker:suffixarray", "library-checker:point_add_range_sum":
		tier := experiment.ReferenceMedium
		if sourceID == "library-checker:point_add_range_sum" {
			tier = experiment.ReferenceMediumBulk
		}
		c, err := experiment.ResolveReferenceTier(tier)
		if err != nil {
			return Profile{}, 0, err
		}
		limits, err := problem.DiskReferenceLimits(sourceID)
		if err != nil {
			return Profile{}, 0, err
		}
		p := UnionFindProfile()
		p.ID = "p4-lc-zalgorithm-reference-desktop-arm64-v1"
		if sourceID == "library-checker:suffixarray" {
			p.ID = "p4-lc-suffixarray-reference-desktop-arm64-v1"
		}
		if sourceID == "library-checker:point_add_range_sum" {
			p.ID = "p4-lc-point-add-range-sum-reference-desktop-arm64-v1"
		}
		p.Runtime, p.Files, p.Materials = c.Runtime, c.Files, limits
		p.Text.Bytes = 4 << 20
		return p, tier, nil
	default:
		return Profile{}, 0, errors.New("unsupported reference source")
	}
}

func digest(value any) string {
	raw, _ := json.Marshal(value)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// Digest identifies the exact complete runtime, materials, checker and deadline.
func (p Profile) Digest() string { return digest(p) }
