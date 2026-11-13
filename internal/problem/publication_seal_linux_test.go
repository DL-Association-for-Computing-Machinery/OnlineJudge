//go:build linux

package problem

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestSealPublicationMaterialsKeepsPrivateBytesOutAndPinsCompleteRevision(t *testing.T) {
	f, d, bound, plan := publicationFixture(t, false)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	p, err := d.PlanPublicationMaterials(publicationFixtureID, d.ID, d.Owner, bound, plan, plan.Digest())
	if err != nil {
		t.Fatal(err)
	}
	expected := plan.Digest()
	// Inputs remain caller-owned; later changes cannot alter the prepared plan.
	plan.Cases[1].Answer = plan.Cases[0].Answer
	sealed, err := p.Seal(ctx, f.local)
	if err != nil || sealed.FormalPlan != expected || sealed.Import != bound || !validHash(sealed.Version) {
		t.Fatalf("could not seal exact publication materials: %v", err)
	}
	repeated, err := p.Seal(ctx, f.local)
	if err != nil || repeated != sealed {
		t.Fatalf("repeat produced another version: %v", err)
	}
	raw, err := readPreviewBytes(ctx, f.local, sealed.Statement)
	if err != nil {
		t.Fatal(err)
	}
	var display PublicationStatement
	if err := json.Unmarshal(raw, &display); err != nil {
		t.Fatal(err)
	}
	if display.ProblemID != publicationFixtureID || display.Version != sealed.Version ||
		len(display.Examples) != 1 || display.Examples[0].Input != "1 2\n" || display.Examples[0].Output != "3\n" ||
		len(display.Images) != 1 || display.Images[0].Source != "images/diagram.png" || display.Images[0].MediaType != "image/png" ||
		bytes.Contains(display.Images[0].Data, []byte("private image trailer")) {
		t.Fatal("display artifact changed the admitted statement, sample or image")
	}
	for _, excluded := range []string{d.ID, d.Owner, bound.ImportRevision, bound.SourceLockSHA256, expected, "synthetic hidden", "private reference", "private-unused"} {
		if bytes.Contains(raw, []byte(excluded)) {
			t.Fatalf("display artifact leaks %q", excluded)
		}
	}
	// Change only a private inventory row that is not used in display or in
	// the formal test ordering; the complete immutable import still changes.
	d.References = append(d.References, Reference{Name: "extra-private", Object: d.References[0].Object})
	slices.SortFunc(d.References, func(a, b Reference) int { return strings.Compare(a.Name, b.Name) })
	newRevision, err := importRevision(d.Document, d.References)
	if err != nil {
		t.Fatal(err)
	}
	d.ImportRevision, bound.ImportRevision = newRevision, newRevision
	nextPlan := p.formal
	nextPlan.ImportRevision = newRevision
	next, err := d.PlanPublicationMaterials(publicationFixtureID, d.ID, d.Owner, bound, nextPlan, nextPlan.Digest())
	if err != nil {
		t.Fatal(err)
	}
	second, err := next.Seal(ctx, f.local)
	if err != nil || second.Version == sealed.Version || second.Statement == sealed.Statement {
		t.Fatalf("complete import changes did not produce a new version: %v", err)
	}
}

func TestSealPublicationMaterialsFailsBeforeCreatingDisplayArtifact(t *testing.T) {
	for _, kind := range []string{"missing formal answer", "damaged formal answer", "unsupported display image"} {
		t.Run(kind, func(t *testing.T) {
			f, d, bound, plan := publicationFixture(t, kind == "unsupported display image")
			switch kind {
			case "missing formal answer":
				if err := os.Remove(filepath.Join(f.root, plan.Cases[1].Answer.SHA256)); err != nil {
					t.Fatal(err)
				}
			case "damaged formal answer":
				path := filepath.Join(f.root, plan.Cases[1].Answer.SHA256)
				bad := bytes.Repeat([]byte("!"), int(plan.Cases[1].Answer.Size))
				if err := os.WriteFile(path, bad, 0600); err != nil {
					t.Fatal(err)
				}
			}
			p, err := d.PlanPublicationMaterials(publicationFixtureID, d.ID, d.Owner, bound, plan, plan.Digest())
			if err != nil {
				t.Fatal(err)
			}
			puts := 0
			f.materials.onPut = func() { puts++ }
			if got, err := p.Seal(t.Context(), f.materials); err == nil || got != (PublicationMaterials{}) || puts != 0 {
				t.Fatalf("partial display artifact escaped, puts=%d, err=%v", puts, err)
			}
		})
	}
}

type publicationPutFailure struct {
	materialstore.Store
	put func(context.Context, materialstore.Object, io.Reader) (materialstore.Object, error)
}

func (m publicationPutFailure) Put(ctx context.Context, o materialstore.Object, r io.Reader) (materialstore.Object, error) {
	return m.put(ctx, o, r)
}

func TestSealPublicationMaterialsDoesNotReturnUnconfirmedReference(t *testing.T) {
	f, d, bound, plan := publicationFixture(t, false)
	p, err := d.PlanPublicationMaterials(publicationFixtureID, d.ID, d.Owner, bound, plan, plan.Digest())
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"write outcome unknown", "wrong object", "cancellation after write"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var called bool
			store := publicationPutFailure{Store: f.local, put: func(ctx context.Context, o materialstore.Object, body io.Reader) (materialstore.Object, error) {
				called = true
				switch kind {
				case "write outcome unknown":
					return materialstore.Object{}, materialstore.ErrCommitUnknown
				case "wrong object":
					return materialstore.Object{SHA256: o.SHA256, Size: o.Size + 1}, nil
				default:
					obj, err := f.local.Put(ctx, o, body)
					if err != nil {
						t.Fatal(err)
					}
					cancel()
					return obj, nil
				}
			}}
			got, err := p.Seal(ctx, store)
			if !called || got != (PublicationMaterials{}) || err == nil ||
				(kind == "write outcome unknown" && !errors.Is(err, materialstore.ErrCommitUnknown)) ||
				(kind == "cancellation after write" && !errors.Is(err, context.Canceled)) {
				t.Fatalf("unconfirmed seal returned a reference: %v", err)
			}
		})
	}
}

func TestSealPublicationMaterialsVerifiesFormalDependenciesPrivately(t *testing.T) {
	b := problemfixture.NewPrimality(t)
	b.BindReferenceCases(t)
	b.Write(t)
	prepared, err := LoadBundle(b.Directory, "library-checker:primality_test", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePrepared(t, prepared) })
	bound, err := prepared.ImportIdentity()
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := prepared.PrepareDiskReference(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	formal := frozen.Plan()
	if err := frozen.Close(); err != nil || len(formal.Dependencies) != 4 {
		t.Fatalf("fixture did not bind four formal headers: %v", err)
	}
	c := prepared.Content()
	d := Draft{ID: previewID, Owner: "11111111111111111111111111111111", SourceID: c.SourceID,
		Version: c.Version, ImportRevision: c.RequestSHA256, State: "private-draft", ValidationState: "not-run",
		Document: c.Document, References: c.References}
	p, err := d.PlanPublicationMaterials(publicationFixtureID, d.ID, d.Owner, bound, formal, formal.Digest())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "objects")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	local, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := prepared.persist(t.Context(), local); err != nil {
		t.Fatal(err)
	}
	sealed, err := p.Seal(t.Context(), local)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := readPreviewBytes(t.Context(), local, sealed.Statement)
	if err != nil {
		t.Fatal(err)
	}
	for _, dependency := range formal.Dependencies {
		if bytes.Contains(raw, []byte(dependency.Path)) || bytes.Contains(raw, []byte(dependency.Object.SHA256)) {
			t.Fatal("sealed display included the private dependency closure")
		}
	}
	dependency := formal.Dependencies[0].Object
	if err := os.Remove(filepath.Join(root, dependency.SHA256)); err != nil {
		t.Fatal(err)
	}
	if got, err := p.Seal(t.Context(), local); err == nil || got != (PublicationMaterials{}) {
		t.Fatal("repeat seal recovered an old result without verifying its formal dependencies")
	}
}
