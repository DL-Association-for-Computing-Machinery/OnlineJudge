package execution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

type manifest struct {
	Version       string                      `json:"version"`
	Origin        string                      `json:"origin"`
	Language      string                      `json:"language"`
	Checker       string                      `json:"checker"`
	CaseSensitive bool                        `json:"case_sensitive"`
	Environment   experiment.ExecutionProfile `json:"execution_environment"`
	Tests         []test                      `json:"tests"`
}
type test struct {
	ID     string `json:"id"`
	Input  string `json:"input_sha256"`
	Answer string `json:"answer_sha256"`
}

var testID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func prepare(d materials.Descriptor, blobs map[string][]byte) (manifest, error) {
	var m manifest
	bad := errors.New("unsupported or corrupt execution material")
	_, fixed, _ := experiment.SelectFixture(experiment.PinnedVersion)
	if len(d.Manifest) > 16384 || len(d.Source) == 0 || len(d.Source) > submission.MaxSourceBytes || !utf8.ValidString(d.Source) ||
		d.Version != experiment.PinnedVersion || d.Language != "gnu++17" || !bytes.Equal(d.Manifest, fixed) || digest(d.Manifest) != d.ManifestSHA256 || digest([]byte(d.Source)) != d.SourceSHA256 {
		return m, bad
	}
	decoder := json.NewDecoder(bytes.NewReader(d.Manifest))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&m) != nil || decoder.Decode(new(any)) != io.EOF || m.Version != d.Version || m.Language != d.Language ||
		m.Origin != "synthetic-project-fixture" || m.Checker != "ascii-tokens-v1" || len(m.Tests) == 0 || len(m.Tests) > 64 ||
		!reflect.DeepEqual(m.Environment, experiment.PinnedEnvironment()) {
		return m, bad
	}
	seen := map[string]bool{}
	total := 0
	for _, t := range m.Tests {
		if !testID.MatchString(t.ID) || seen[t.ID] {
			return m, bad
		}
		seen[t.ID] = true
		for _, hash := range []string{t.Input, t.Answer} {
			data, ok := blobs[hash]
			if !ok || len(data) > judging.TextBudget || digest(data) != hash {
				return m, bad
			}
			total += len(data)
			if total > 4<<20 {
				return m, bad
			}
		}
		// Corrupt later reference material must not be hidden by an earlier WA.
		if _, err := judging.CompareText(blobs[t.Answer], nil, m.CaseSensitive); err != nil {
			return m, bad
		}
	}
	return m, nil
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
