package problem

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

type checkingMaterials struct {
	puts    int
	fail    int
	after   func()
	objects map[string][]byte
}

func (s *checkingMaterials) Put(ctx context.Context, o materialstore.Object, r io.Reader) (materialstore.Object, error) {
	s.puts++
	if s.puts == s.fail {
		return materialstore.Object{}, materialstore.ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(r, o.Size+1))
	if err != nil || int64(len(raw)) != o.Size || hash(raw) != o.SHA256 {
		return materialstore.Object{}, materialstore.ErrIntegrity
	}
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	s.objects[o.SHA256] = raw
	if s.after != nil {
		s.after()
	}
	return o, nil
}
func (s *checkingMaterials) Open(context.Context, materialstore.Object) (*materialstore.Snapshot, error) {
	return nil, errors.New("unused fixture method")
}

func TestPreparedBundle(t *testing.T) {
	b := problemfixture.New(t)
	p, err := LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer closePrepared(t, p)
	c := p.Content()
	if c.SourceID != "library-checker:aplusb" || c.Version != b.Document["version"] || len(c.References) != 10 {
		t.Fatalf("incomplete metadata: references=%d", len(c.References))
	}
	c.Document[0] = 'x'
	c.References[0].Name = "mutated"
	if p.Content().Document[0] == 'x' || p.Content().References[0].Name == "mutated" {
		t.Fatal("mutable prepared metadata escaped")
	}
	materials := &checkingMaterials{}
	if err = p.persist(context.Background(), materials); err != nil {
		t.Fatal(err)
	}
	if materials.puts != 10 {
		t.Fatal("not all logical references persisted")
	}
	if err = p.acquire(); err != nil {
		t.Fatal(err)
	}
	if err = p.Close(); !errors.Is(err, ErrBusy) {
		t.Fatal("active source closed")
	}
	p.release()
	if err = p.Close(); err != nil {
		t.Fatal(err)
	}
	if err = p.acquire(); !errors.Is(err, ErrInvalid) {
		t.Fatal("closed prepared source reused")
	}
}

func TestBundleRejectsInvalidInput(t *testing.T) {
	for _, name := range []string{"wrong-lock", "publication", "limits", "unknown-field", "field-alias", "missing-explicit-false", "bad-version", "path-escape", "wrong-role", "missing-source", "duplicate-json", "unlisted-file", "incomplete", "file-link", "directory-link", "fifo", "public-file", "payload-corruption"} {
		t.Run(name, func(t *testing.T) {
			b := problemfixture.New(t)
			lock := b.Lock
			wantPersistFailure := false
			switch name {
			case "wrong-lock":
				lock = []byte("{}\n")
			case "publication":
				b.Document["state"] = "published"
				b.Write(t)
			case "limits":
				b.Document["judging"].(map[string]any)["limits"] = map[string]any{"cpu": 1}
				b.Write(t)
			case "unknown-field":
				b.Document["owner"] = "caller"
				b.Write(t)
			case "field-alias":
				b.Document["STATE"] = b.Document["state"]
				delete(b.Document, "state")
				b.Write(t)
			case "missing-explicit-false":
				delete(b.Document["judging"].(map[string]any), "numeric_equivalence")
				b.Write(t)
			case "bad-version":
				b.Document["version"] = strings.Repeat("0", 64)
				raw := problemfixture.JSON(t, b.Document)
				if err := os.WriteFile(filepath.Join(b.Directory, "drafts/aplusb.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "path-escape":
				b.Document["statement"].(map[string]any)["path"] = "../outside"
				b.Write(t)
			case "wrong-role":
				b.Document["examples"] = []string{}
				b.Write(t)
			case "missing-source":
				b.Document["provenance"].(map[string]any)["original_files"] = map[string]any{}
				b.Write(t)
			case "duplicate-json":
				raw := b.Files["index.json"]
				raw = append([]byte(`{"state":"private-unpublished",`), raw[1:]...)
				if err := os.WriteFile(filepath.Join(b.Directory, "index.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "unlisted-file":
				if err := os.WriteFile(filepath.Join(b.Directory, "extra"), []byte("private"), 0600); err != nil {
					t.Fatal(err)
				}
			case "incomplete":
				if err := os.WriteFile(filepath.Join(filepath.Dir(b.Directory), "INCOMPLETE"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "file-link":
				target := filepath.Join(b.Directory, "statements/aplusb.md")
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("../provenance/material-map.json", target); err != nil {
					t.Fatal(err)
				}
			case "directory-link":
				target := filepath.Join(b.Directory, "statements")
				if err := os.Rename(target, target+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("statements-old", target); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				makeBundleFIFO(t, filepath.Join(b.Directory, "statements/aplusb.md"))
			case "public-file":
				if err := os.Chmod(filepath.Join(b.Directory, "statements/aplusb.md"), 0644); err != nil {
					t.Fatal(err)
				}
			case "payload-corruption":
				target := filepath.Join(b.Directory, "statements/aplusb.md")
				raw := bytes.Clone(b.Files["statements/aplusb.md"])
				raw[0] = 'X'
				if err := os.WriteFile(target, raw, 0600); err != nil {
					t.Fatal(err)
				}
				wantPersistFailure = true
			}
			p, err := LoadBundle(b.Directory, "library-checker:aplusb", lock)
			if wantPersistFailure {
				if err != nil {
					t.Fatal(err)
				}
				defer closePrepared(t, p)
				if err = p.persist(context.Background(), &checkingMaterials{}); !errors.Is(err, materialstore.ErrIntegrity) {
					t.Fatal("corrupt material accepted")
				}
				return
			}
			if err == nil {
				closePrepared(t, p)
				t.Fatal("invalid bundle accepted")
			}
		})
	}
}

func closePrepared(t *testing.T, p *Prepared) {
	t.Helper()
	if err := p.Close(); err != nil {
		t.Error(err)
	}
}

func TestCanonicalUnicodeAndEscapedLiterals(t *testing.T) {
	for _, s := range []string{"中文<>&\u2028\u2029", `literal\u2028\u2029`, "mixed\\\u2028", "control\t\n"} {
		raw, err := canonical(map[string]any{"s": s})
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]string
		if err = json.Unmarshal(raw, &decoded); err != nil || decoded["s"] != s {
			t.Fatal("canonical escaping changed content")
		}
		if _, err = canonicalMap(raw); err != nil {
			t.Fatal(err)
		}
	}
}
