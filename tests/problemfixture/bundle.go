// Package problemfixture builds bounded synthetic bundles for behavior tests.
package problemfixture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Bundle is test-owned source data, never an authorized production source lock.
type Bundle struct {
	Directory string
	Lock      []byte
	Document  map[string]any
	Files     map[string][]byte
}

// JSON uses the bundle's canonical map serialization for these ASCII fixtures.
func JSON(t *testing.T, value any) []byte {
	t.Helper()
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(value); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Hash identifies exact fixture bytes.
func Hash(raw []byte) string { s := sha256.Sum256(raw); return hex.EncodeToString(s[:]) }
func identity(raw []byte) map[string]any {
	return map[string]any{"sha256": Hash(raw), "size": len(raw)}
}

// New writes a minimal supported one-case/one-example private bundle. Its source
// policy is explicitly passed by tests, never substituted for the checked-in LC lock.
func New(t *testing.T) *Bundle {
	t.Helper()
	b := &Bundle{Directory: filepath.Join(t.TempDir(), "bundle"), Files: map[string][]byte{}}
	original := []byte("synthetic source license\n")
	input := []byte("1 2\n")
	answer := []byte("3\n")
	statement := []byte("# Synthetic A + B\n")
	ref := func(raw []byte) map[string]any {
		p := "objects/sha256/" + Hash(raw)
		b.Files[p] = raw
		v := identity(raw)
		v["path"] = p
		return v
	}
	lock := map[string]any{"schema": "oj.library-checker-source.v1", "repository": "https://github.com/yosupo06/library-checker-problems", "commit": "1111111111111111111111111111111111111111", "problems": []any{map[string]any{"path": "sample/aplusb", "title_zh": "Synthetic A + B"}}, "files": map[string]any{"LICENSE": identity(original)}}
	b.Lock = JSON(t, lock)
	b.Files["provenance/source-lock.json"] = b.Lock
	for _, p := range []string{"source-catalog", "source-validation", "material-map"} {
		b.Files["provenance/"+p+".json"] = []byte("{}\n")
	}
	s := identity(statement)
	s["path"] = "statements/aplusb.md"
	s["language"] = "en"
	b.Files["statements/aplusb.md"] = statement
	b.Document = map[string]any{
		"schema": "oj.problem-draft.v1", "adapter": "library-checker-pinned-v1", "state": "private-draft", "title": "Synthetic A + B", "statement": s,
		"test_cases": []any{map[string]any{"id": "case-0001", "input": ref(input), "answer": ref(answer), "sample": true}}, "examples": []string{"case-0001"},
		"judging":    map[string]any{"mode": "tokens", "case_sensitive": true, "numeric_equivalence": false, "token_comparison": "exact-bytes", "compatibility": "configured-not-validated", "limits": nil, "production_profile": nil},
		"provenance": map[string]any{"commit": lock["commit"], "repository": lock["repository"], "original_files": map[string]any{"source/LICENSE": ref(original)}, "original_title": "Synthetic A + B", "source_id": "library-checker:aplusb", "source_lock_sha256": Hash(b.Lock), "source_path": "sample/aplusb", "upstream_time_limit_seconds": json.Number("2.0"), "source_case_mapping": []any{map[string]any{"case_id": "case-0001", "upstream_case": "example_00"}}},
		"validation": map[string]any{"status": "not-run", "publication_blockers": []string{"statement-rendering-and-formula-validation", "production-checker-compatibility-and-resource-admission", "independent-problem-validation", "authorized-non-author-review", "authorized-atomic-publication"}},
	}
	b.Write(t)
	return b
}

// Write updates only this test's files and recomputes content/index identities.
func (b *Bundle) Write(t *testing.T) {
	t.Helper()
	delete(b.Document, "version")
	b.Document["version"] = Hash(JSON(t, b.Document))
	b.Files["drafts/aplusb.json"] = JSON(t, b.Document)
	inventory := map[string]any{}
	var total int
	for p, raw := range b.Files {
		if p != "index.json" {
			inventory[p] = identity(raw)
			total += len(raw)
		}
	}
	b.Files["index.json"] = JSON(t, map[string]any{"schema": "oj.private-draft-bundle.v1", "state": "private-unpublished", "access": "synthetic private fixture", "copied_source_bytes": total, "files": inventory, "problems": []any{map[string]any{"draft": "drafts/aplusb.json", "source_id": "library-checker:aplusb", "version": b.Document["version"], "test_cases": len(b.Document["test_cases"].([]any)), "examples": len(b.Document["examples"].([]string))}}})
	for p, raw := range b.Files {
		target := filepath.Join(b.Directory, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
