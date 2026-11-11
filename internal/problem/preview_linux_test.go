//go:build linux

package problem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestPreviewPreservesTextAndExcludesFormalMaterials(t *testing.T) {
	input := "\ufeff 1\r\n2\t\x00\n"
	f := newPreviewFixture(t, func(b *problemfixture.Bundle) {
		cases := b.Document["test_cases"].([]any)
		cases[0].(map[string]any)["input"] = previewTextRef(b, []byte(input))
		cases[0].(map[string]any)["answer"] = previewTextRef(b, nil)
		b.Document["test_cases"] = append(cases,
			map[string]any{"id": "case-0002", "input": previewTextRef(b, []byte("private formal input")), "answer": previewTextRef(b, []byte("private formal answer")), "sample": false},
			map[string]any{"id": "case-0003", "input": previewTextRef(b, []byte(" 7\n")), "answer": previewTextRef(b, []byte("8\n")), "sample": true})
		b.Document["examples"] = []string{"case-0001", "case-0003"}
		provenance := b.Document["provenance"].(map[string]any)
		provenance["source_case_mapping"] = append(provenance["source_case_mapping"].([]any),
			map[string]any{"case_id": "case-0002", "upstream_case": "formal_00"}, map[string]any{"case_id": "case-0003", "upstream_case": "example_01"})
	})
	// Persisted preview must survive the import catalog being rotated away.
	f.importer.sources = nil
	got, err := f.importer.Preview(t.Context(), f.sessions.credential, previewID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Markdown != "# Synthetic A + B\n" || got.Version != f.repository.draft.Version || got.Language != "en" ||
		!reflect.DeepEqual(got.Examples, []Example{{ID: "case-0001", Input: input, Output: ""}, {ID: "case-0003", Input: " 7\n", Output: "8\n"}}) {
		t.Fatal("preview changed text bytes, order or immutable version")
	}
	if len(f.materials.opened) != 5 || f.repository.gets != 2 {
		t.Fatal("preview read extra roles or skipped final authorization")
	}
	doc, _, err := previewFiles(f.repository.draft, f.sessions.view.SubjectID, previewID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{doc.Statement.SHA256, doc.TestCases[0].Input.SHA256, doc.TestCases[0].Answer.SHA256, doc.TestCases[2].Input.SHA256, doc.TestCases[2].Answer.SHA256}
	for n, object := range f.materials.opened {
		if object.SHA256 != want[n] {
			t.Fatal("preview opened a formal test or original source")
		}
	}
}

func TestPreviewRejectsBeforeMaterialIO(t *testing.T) {
	for _, kind := range []string{"stale-session", "foreign", "missing", "revoked", "late-reference", "document-reference", "duplicate-role", "version", "budget"} {
		t.Run(kind, func(t *testing.T) {
			var change func(*problemfixture.Bundle)
			if kind == "budget" {
				change = func(b *problemfixture.Bundle) {
					// One shared object appears twice; count both fields, not hashes.
					c := b.Document["test_cases"].([]any)[0].(map[string]any)
					c["input"] = previewTextRef(b, []byte(strings.Repeat("a", int(MaxPreviewBytes/2))))
					c["answer"] = c["input"]
				}
			}
			f := newPreviewFixture(t, change)
			id := previewID
			want := error(ErrStore)
			switch kind {
			case "stale-session":
				f.sessions.err = redisstore.ErrStale
				want = redisstore.ErrStale
			case "foreign":
				f.sessions.view.SubjectID = "44444444444444444444444444444444"
				want = ErrNotFound
			case "missing":
				id = "55555555555555555555555555555555"
				want = ErrNotFound
			case "revoked":
				f.repository.first = identity.ErrForbidden
				want = identity.ErrForbidden
			case "late-reference", "document-reference":
				name := "tests/case-0001/answer"
				if kind == "document-reference" {
					name = "draft.json"
				}
				for n := range f.repository.draft.References {
					if f.repository.draft.References[n].Name == name {
						f.repository.draft.References[n].Object.SHA256 = strings.Repeat("0", 64)
					}
				}
			case "duplicate-role":
				f.repository.draft.References = append(f.repository.draft.References, f.repository.draft.References[0])
			case "version":
				f.repository.draft.Version = strings.Repeat("0", 64)
			}
			got, err := f.importer.Preview(t.Context(), f.sessions.credential, id)
			if !errors.Is(err, want) || !reflect.DeepEqual(got, Preview{}) || len(f.materials.opened) != 0 {
				t.Fatalf("preflight %s: error=%v material reads=%d", kind, err, len(f.materials.opened))
			}
		})
	}
}

func TestPreviewFailureNeverReturnsPartialText(t *testing.T) {
	for _, kind := range []string{"invalid-utf8", "later-open-failure", "missing-object", "corrupt-object", "closed-snapshot", "logout", "actor-change", "role-revoked", "metadata-change", "source-change", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			var change func(*problemfixture.Bundle)
			if kind == "invalid-utf8" {
				change = func(b *problemfixture.Bundle) {
					b.Document["test_cases"].([]any)[0].(map[string]any)["answer"] = previewTextRef(b, []byte{0xff})
				}
			}
			f := newPreviewFixture(t, change)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := error(ErrStore)
			switch kind {
			case "later-open-failure":
				f.materials.failAt = 3
			case "closed-snapshot":
				f.materials.closeRead = true
			case "missing-object", "corrupt-object":
				answer := f.bundle.Document["test_cases"].([]any)[0].(map[string]any)["answer"].(map[string]any)
				path := filepath.Join(f.root, answer["sha256"].(string))
				if kind == "missing-object" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(path, []byte("4\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "logout":
				want = redisstore.ErrStale
				f.materials.onOpen = func() { f.sessions.err = redisstore.ErrStale }
			case "actor-change":
				want = identity.ErrConflict
				f.materials.onOpen = func() { f.sessions.view.AuthGeneration++ }
			case "role-revoked":
				want = identity.ErrForbidden
				f.repository.final = identity.ErrForbidden
			case "metadata-change":
				f.repository.changeFinalDraft = true
			case "source-change":
				f.repository.changeSourceID = true
			case "cancel":
				want = context.Canceled
				f.materials.onOpen = cancel
			}
			got, err := f.importer.Preview(ctx, f.sessions.credential, previewID)
			if !errors.Is(err, want) || !reflect.DeepEqual(got, Preview{}) {
				t.Fatalf("failure %s returned partial result or wrong error: %v", kind, err)
			}
			if err := f.local.Close(); err != nil {
				t.Fatal("failed preview retained a snapshot slot")
			}
		})
	}
}

func TestPreviewAndImportShareMaterialCapacity(t *testing.T) {
	for _, active := range []string{"preview", "import"} {
		t.Run(active, func(t *testing.T) {
			f := newPreviewFixture(t, nil)
			entered, release := make(chan struct{}), make(chan struct{})
			var once, signal sync.Once
			finish := func() { once.Do(func() { close(release) }) }
			defer finish()
			block := func() { signal.Do(func() { close(entered) }); <-release }
			importDraft := func() error {
				_, err := f.importer.Import(t.Context(), f.sessions.credential, "shared-material-request-key", "library-checker:aplusb")
				return err
			}
			previewDraft := func() error {
				_, err := f.importer.Preview(t.Context(), f.sessions.credential, previewID)
				return err
			}
			first, second := previewDraft, importDraft
			if active == "preview" {
				f.materials.onOpen = block
			} else {
				f.materials.onPut = block
				first, second = importDraft, previewDraft
			}
			done := make(chan error, 1)
			go func() { done <- first() }()
			<-entered
			if err := second(); !errors.Is(err, ErrBusy) {
				t.Fatal("competing import/preview was queued or admitted")
			}
			finish()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if err := second(); err != nil {
				t.Fatal("completed material work retained capacity")
			}
		})
	}
}
