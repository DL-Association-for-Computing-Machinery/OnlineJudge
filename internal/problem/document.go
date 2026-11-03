package problem

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

const maxMetadata = 1 << 20
const maxObject = 128 << 20
const maxBundle = 2 << 30

type fileReference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func (r fileReference) object() materialstore.Object {
	return materialstore.Object{SHA256: r.SHA256, Size: r.Size}
}

type fileIdentity struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func (r fileReference) identity() fileIdentity { return fileIdentity{r.SHA256, r.Size} }

type document struct {
	Schema    string `json:"schema"`
	Adapter   string `json:"adapter"`
	State     string `json:"state"`
	Version   string `json:"version"`
	Title     string `json:"title"`
	Statement struct {
		fileReference
		Language string                   `json:"language"`
		Images   map[string]fileReference `json:"images,omitempty"`
	} `json:"statement"`
	TestCases []struct {
		ID     string        `json:"id"`
		Input  fileReference `json:"input"`
		Answer fileReference `json:"answer"`
		Sample bool          `json:"sample"`
	} `json:"test_cases"`
	Examples []string `json:"examples"`
	Judging  struct {
		Mode               string          `json:"mode"`
		CaseSensitive      bool            `json:"case_sensitive"`
		Compatibility      string          `json:"compatibility"`
		Limits             json.RawMessage `json:"limits"`
		NumericEquivalence bool            `json:"numeric_equivalence"`
		ProductionProfile  json.RawMessage `json:"production_profile"`
		TokenComparison    string          `json:"token_comparison"`
	} `json:"judging"`
	Provenance struct {
		Commit           string                   `json:"commit"`
		Repository       string                   `json:"repository"`
		OriginalFiles    map[string]fileReference `json:"original_files"`
		OriginalTitle    string                   `json:"original_title"`
		SourceID         string                   `json:"source_id"`
		SourceLockSHA256 string                   `json:"source_lock_sha256"`
		SourcePath       string                   `json:"source_path"`
		TimeLimit        json.Number              `json:"upstream_time_limit_seconds"`
		Mapping          []struct {
			CaseID       string `json:"case_id"`
			UpstreamCase string `json:"upstream_case"`
		} `json:"source_case_mapping"`
	} `json:"provenance"`
	Validation struct {
		Status   string   `json:"status"`
		Blockers []string `json:"publication_blockers"`
	} `json:"validation"`
}

var publicationBlockers = []string{"statement-rendering-and-formula-validation", "production-checker-compatibility-and-resource-admission", "independent-problem-validation", "authorized-non-author-review", "authorized-atomic-publication"}

// strictJSON rejects unknown typed fields and trailing values. Canonical
// comparison also rejects duplicate keys, alternate escaping and extra space.
func strictJSON(raw []byte, dst any) error {
	if len(raw) == 0 || len(raw) > maxMetadata || !utf8.Valid(raw) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	d.UseNumber()
	if err := d.Decode(dst); err != nil {
		return ErrInvalid
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return ErrInvalid
	}
	// encoding/json accepts case-insensitive field aliases and defaults absent
	// fields. Compare typed and original shapes so this fixed format accepts
	// only exact declared keys, including explicit false/null fields.
	if reflect.ValueOf(dst).Elem().Kind() == reflect.Struct {
		var original any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&original); err != nil {
			return ErrInvalid
		}
		before, err := canonical(original)
		if err != nil {
			return err
		}
		after, err := canonical(dst)
		if err != nil || !bytes.Equal(before, after) {
			return ErrInvalid
		}
	}
	return nil
}

// canonical preserves number lexemes from the supported source format. JSON
// escaping follows Python ensure_ascii=False, including U+2028/U+2029. Skip
// escaped backslash pairs so literal "\\u2028" is never rewritten.
func canonical(value any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(value); err != nil {
		return nil, ErrInvalid
	}
	// Sort object keys even when value was a typed struct rather than a map.
	var normalized any
	decoder := json.NewDecoder(bytes.NewReader(b.Bytes()))
	decoder.UseNumber()
	if err := decoder.Decode(&normalized); err != nil {
		return nil, ErrInvalid
	}
	b.Reset()
	if err := e.Encode(normalized); err != nil {
		return nil, ErrInvalid
	}
	raw := b.Bytes()
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			if bytes.HasPrefix(raw[i:], []byte(`\u2028`)) {
				out = append(out, []byte("\u2028")...)
				i += 5
				continue
			}
			if bytes.HasPrefix(raw[i:], []byte(`\u2029`)) {
				out = append(out, []byte("\u2029")...)
				i += 5
				continue
			}
			out = append(out, raw[i], raw[i+1])
			i++
			continue
		}
		out = append(out, raw[i])
	}
	return out, nil
}

func canonicalMap(raw []byte) (map[string]any, error) {
	var v map[string]any
	if err := strictJSON(raw, &v); err != nil || v == nil {
		return nil, ErrInvalid
	}
	encoded, err := canonical(v)
	if err != nil || !bytes.Equal(raw, encoded) {
		return nil, ErrInvalid
	}
	return v, nil
}
func hash(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func validHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func validPath(s string) bool {
	if s == "" || len(s) > 240 || path.Clean(s) != s || strings.HasPrefix(s, "/") || strings.Contains(s, "\\") {
		return false
	}
	for _, c := range s {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	for _, part := range strings.Split(s, "/") {
		if part == ".." || part == "." || part == "" {
			return false
		}
	}
	return true
}
func validFile(r fileReference) bool {
	return validPath(r.Path) && validHash(r.SHA256) && r.Size >= 0 && r.Size <= maxObject
}

func parseDocument(raw []byte, sourceID string) (document, error) {
	var d document
	m, err := canonicalMap(raw)
	if err != nil {
		return d, err
	}
	if err = strictJSON(raw, &d); err != nil {
		return d, err
	}
	delete(m, "version")
	versioned, err := canonical(m)
	if err != nil || !validHash(d.Version) || hash(versioned) != d.Version || d.Schema != "oj.problem-draft.v1" || d.Adapter != "library-checker-pinned-v1" || d.State != "private-draft" || d.Title == "" || len(d.Title) > 512 {
		return d, ErrInvalid
	}
	p := d.Provenance
	if p.SourceID != sourceID || !validPath(p.SourcePath) || sourceID != "library-checker:"+path.Base(p.SourcePath) || p.Repository != "https://github.com/yosupo06/library-checker-problems" || len(p.Commit) != 40 || !validHash(p.SourceLockSHA256) || p.OriginalTitle == "" {
		return d, ErrInvalid
	}
	timeLimit, e := p.TimeLimit.Float64()
	if e != nil || timeLimit <= 0 || timeLimit > 3600 {
		return d, ErrInvalid
	}
	j := d.Judging
	if j.Mode != "tokens" || !j.CaseSensitive || j.NumericEquivalence || j.TokenComparison != "exact-bytes" || j.Compatibility != "configured-not-validated" || string(j.Limits) != "null" || string(j.ProductionProfile) != "null" || d.Validation.Status != "not-run" || !reflect.DeepEqual(d.Validation.Blockers, publicationBlockers) {
		return d, ErrInvalid
	}
	if !validFile(d.Statement.fileReference) || d.Statement.Language != "en" || len(d.TestCases) < 1 || len(d.TestCases) > 1024 || len(p.Mapping) != len(d.TestCases) || len(p.OriginalFiles) < 1 || len(p.OriginalFiles) > 2048 {
		return d, ErrInvalid
	}
	var examples []string
	seen := map[string]bool{}
	for i, c := range d.TestCases {
		if c.ID != fmt.Sprintf("case-%04d", i+1) || !validFile(c.Input) || !validFile(c.Answer) || c.Input.Path != "objects/sha256/"+c.Input.SHA256 || c.Answer.Path != "objects/sha256/"+c.Answer.SHA256 || p.Mapping[i].CaseID != c.ID || !validPath(p.Mapping[i].UpstreamCase) || strings.Contains(p.Mapping[i].UpstreamCase, "/") || seen[p.Mapping[i].UpstreamCase] {
			return d, ErrInvalid
		}
		seen[p.Mapping[i].UpstreamCase] = true
		if c.Sample {
			examples = append(examples, c.ID)
		}
	}
	if !reflect.DeepEqual(examples, d.Examples) {
		return d, ErrInvalid
	}
	for name, r := range p.OriginalFiles {
		if !validPath(name) || !strings.HasPrefix(name, "source/") || !validFile(r) || r.Path != "objects/sha256/"+r.SHA256 {
			return d, ErrInvalid
		}
	}
	if len(d.Statement.Images) > MaxPreviewImages {
		return d, ErrInvalid
	}
	for source, r := range d.Statement.Images {
		original, ok := p.OriginalFiles["source/"+p.SourcePath+"/"+source]
		if !validImageSource(source) || !validFile(r) || !ok || r != original || r.Size < 1 || r.Size > MaxPreviewImageBytes {
			return d, ErrInvalid
		}
	}
	return d, nil
}
