package execution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strings"
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
	tokenBytes    int
}
type test struct {
	ID     string `json:"id"`
	Input  string `json:"input_sha256"`
	Answer string `json:"answer_sha256"`
}

var testID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func prepare(ctx context.Context, d materials.Descriptor, blobs map[string][]byte, fixture experiment.WorkerFixture) (manifest, error) {
	var m manifest
	bad := errors.New("unsupported or corrupt execution material")
	if len(d.Manifest) > 16384 || len(d.Source) == 0 || len(d.Source) > submission.MaxSourceBytes || !utf8.ValidString(d.Source) || strings.ContainsRune(d.Source, 0) ||
		d.Version != fixture.Version() || !fixture.SupportsLanguage(d.Language) || !bytes.Equal(d.Manifest, fixture.Manifest()) || digest(d.Manifest) != d.ManifestSHA256 || digest([]byte(d.Source)) != d.SourceSHA256 {
		return m, bad
	}
	if fixture.UsesCachedText() {
		parse := materials.ParseAPlusB
		if fixture.IsLocalText() {
			parse = materials.ParseLocalText
		}
		p, err := parse(d.Manifest, d.ManifestSHA256)
		if err != nil {
			return m, bad
		}
		m = manifest{Version: p.Version, Language: p.Language, Origin: p.Origin, Checker: p.Checker, Environment: fixture.ProfileForLanguage(d.Language), tokenBytes: p.TokenBytes}
		for _, point := range p.Tests {
			m.Tests = append(m.Tests, test{ID: point.ID, Input: point.Input, Answer: point.Answer})
		}
	} else {
		decoder := json.NewDecoder(bytes.NewReader(d.Manifest))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&m) != nil || decoder.Decode(new(any)) != io.EOF || m.Version != d.Version || m.Language != d.Language ||
			m.Origin != "synthetic-project-fixture" || m.Checker != "ascii-tokens-v1" || len(m.Tests) == 0 || len(m.Tests) > 64 ||
			!reflect.DeepEqual(m.Environment, fixture.Profile()) {
			return m, bad
		}
	}
	seen := map[string]bool{}
	total := 0
	fileLimit, totalLimit := fixture.MaterialLimits()
	for _, t := range m.Tests {
		if !testID.MatchString(t.ID) || seen[t.ID] {
			return m, bad
		}
		seen[t.ID] = true
		for _, hash := range []string{t.Input, t.Answer} {
			data, ok := blobs[hash]
			if !ok || len(data) > fileLimit || digest(data) != hash {
				return m, bad
			}
			total += len(data)
			if total > totalLimit {
				return m, bad
			}
		}
		// Corrupt later reference material must not be hidden by an earlier WA.
		if m.Checker == materials.LocalSPJChecker {
			if len(blobs[t.Input]) > experiment.SPJObjectBytes || len(blobs[t.Answer]) > experiment.SPJObjectBytes {
				return m, bad
			}
		} else if m.tokenBytes != 0 {
			answer := blobs[t.Answer]
			if (!fixture.IsLocalText() && (len(answer) > 4096 || len(blobs[t.Input]) > 16<<10)) ||
				judging.ValidateLibraryCheckerReferenceAt(ctx, judging.FrozenText{Reader: bytes.NewReader(answer), Size: int64(len(answer))},
					judging.TextLimits{Bytes: materials.APlusBOutputBytes, TokenBytes: m.tokenBytes}) != nil {
				return m, bad
			}
		} else if _, err := judging.CompareText(blobs[t.Answer], nil, m.CaseSensitive); err != nil {
			return m, bad
		}
	}
	return m, nil
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
