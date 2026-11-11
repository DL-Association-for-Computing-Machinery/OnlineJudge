//go:build linux

package problem_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func TestReviewMaterialsReadsOnlyPinnedPrimalitySourceClosure(t *testing.T) {
	b, p := dependencySnapshotFixture(t, nil)
	ctx := t.Context()
	frozen, err := p.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plan := frozen.Plan()
	if err := frozen.Close(); err != nil {
		t.Fatal(err)
	}
	bound, err := p.ImportIdentity()
	if err != nil {
		t.Fatal(err)
	}
	c := p.Content()
	id, owner := strings.Repeat("1", 32), strings.Repeat("2", 32)
	d := problem.Draft{ID: id, Owner: owner, State: "private-draft", ValidationState: "not-run",
		SourceID: c.SourceID, Version: c.Version, ImportRevision: c.RequestSHA256, Document: c.Document, References: c.References}
	manifest, err := d.PrepareReviewMaterials(id, owner, bound, plan)
	if err != nil {
		t.Fatal(err)
	}
	altered := plan
	altered.Dependencies = slices.Clone(plan.Dependencies)
	altered.Dependencies[0].Object = plan.Source
	if _, err := d.PrepareReviewMaterials(id, owner, bound, altered); err == nil {
		t.Fatal("another object took the place of an admitted header")
	}
	root := filepath.Join(t.TempDir(), "objects")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	local, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 1 << 20, MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Error(err)
		}
	})
	needed := map[materialstore.Object]bool{plan.Source: true}
	for _, dep := range plan.Dependencies {
		needed[dep.Object] = true
	}
	for _, ref := range c.References {
		if ref.Name == "statement" || strings.HasPrefix(ref.Name, "tests/case-0001/") {
			needed[ref.Object] = true
		}
	}
	// Omit every other object (including the formal catalog and case hashes).
	// Success proves this read never needs the rest of the Worker snapshot.
	for _, raw := range b.Files {
		sum := sha256.Sum256(raw)
		o := materialstore.Object{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(raw))}
		if needed[o] {
			if _, err := local.Put(ctx, o, bytes.NewReader(raw)); err != nil {
				t.Fatal(err)
			}
			delete(needed, o)
		}
	}
	preview, source, err := manifest.Read(ctx, local)
	if err != nil || len(needed) != 0 || preview.DraftID != id || len(preview.Examples) != 1 ||
		!strings.Contains(source.Source, "../lib/prime.hpp") || len(source.Dependencies) != 4 {
		t.Fatalf("could not read the admitted statement and source closure: %v", err)
	}
	for i, dep := range source.Dependencies {
		sum := sha256.Sum256([]byte(dep.Source))
		if dep.Name != plan.Dependencies[i].Path || hex.EncodeToString(sum[:]) != plan.Dependencies[i].Object.SHA256 {
			t.Fatal("review header drifted from the frozen plan")
		}
	}
	last := plan.Dependencies[len(plan.Dependencies)-1].Object
	path := filepath.Join(root, last.SHA256)
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		t.Fatalf("could not read synthetic last header: %v", err)
	}
	raw[0] ^= 1
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	preview, source, err = manifest.Read(ctx, local)
	if err == nil || preview.DraftID != "" || source.Source != "" || len(source.Dependencies) != 0 {
		t.Fatal("last-header corruption returned partial material")
	}
	// The failed read must not leave the only object slot occupied.
	next, err := local.Open(ctx, plan.Source)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
}
