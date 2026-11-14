//go:build linux

package problem

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

func TestReadSealedPublicationStatementUsesExactCanonicalBinding(t *testing.T) {
	f, d, binding, plan := publicationFixture(t, false)
	prepared, err := d.PlanPublicationMaterials(publicationFixtureID, d.ID, d.Owner, binding, plan, plan.Digest())
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := prepared.Seal(t.Context(), f.local)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadSealedPublicationStatement(t.Context(), f.local, sealed)
	if err != nil || got.ProblemID != sealed.ProblemID || got.Version != sealed.Version ||
		len(got.Examples) != 1 || got.Examples[0].Input != "1 2\n" || got.Examples[0].Output != "3\n" || len(got.Images) != 1 {
		t.Fatalf("valid sealed statement did not read back: %v", err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	storeVariant := func(raw []byte) PublicationMaterials {
		t.Helper()
		copy := sealed
		ref := materialstore.Object{SHA256: hash(raw), Size: int64(len(raw))}
		obj, err := f.local.Put(t.Context(), ref, bytes.NewReader(raw))
		if err != nil || obj != ref {
			t.Fatalf("could not store fixture variant: %v", err)
		}
		copy.Statement = ref
		return copy
	}
	badProjection := got
	badProjection.Images[0].MediaType = "image/svg+xml"
	badProjection.Version, err = publicationVersion(badProjection, sealed.FormalPlan)
	if err != nil {
		t.Fatal(err)
	}
	invalidProjection, err := json.Marshal(badProjection)
	if err != nil {
		t.Fatal(err)
	}
	invalidExpected := storeVariant(invalidProjection)
	invalidExpected.Version = badProjection.Version
	for _, changed := range []PublicationMaterials{
		invalidExpected,
		storeVariant(bytes.Replace(raw, []byte("{"), []byte("{\"owner\":\"private\","), 1)),
		storeVariant(bytes.Replace(raw, []byte("{"), []byte("{\"title\":\"duplicate\","), 1)),
		storeVariant(append(bytes.Clone(raw), '\n')),
		storeVariant(bytes.Replace(raw, []byte("1 2"), []byte("1 3"), 1)),
		{ProblemID: d.ID, Version: sealed.Version, Import: sealed.Import, FormalPlan: sealed.FormalPlan, Statement: sealed.Statement},
		{ProblemID: sealed.ProblemID, Version: sealed.Version, Import: sealed.Import, FormalPlan: sealed.Import.ImportRevision, Statement: sealed.Statement},
	} {
		result, err := ReadSealedPublicationStatement(t.Context(), f.local, changed)
		if err == nil || result.ProblemID != "" || result.Markdown != "" || result.Examples != nil {
			t.Fatal("mismatched display produced a usable view")
		}
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ReadSealedPublicationStatement(cancelled, f.local, sealed); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled display read: %v", err)
	}
	path := filepath.Join(f.root, sealed.Statement.SHA256)
	if err := os.WriteFile(path, bytes.Repeat([]byte("!"), len(raw)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSealedPublicationStatement(t.Context(), f.local, sealed); !errors.Is(err, ErrStore) {
		t.Fatalf("damaged display was not rejected: %v", err)
	}
}

func TestVerifySealedPublicationMaterialsRequiresBothObjectSets(t *testing.T) {
	for _, kind := range []string{"matching references", "different full import", "changed private plan", "missing display", "missing formal answer"} {
		t.Run(kind, func(t *testing.T) {
			f, d, binding, plan := publicationFixture(t, false)
			prepared, err := d.PlanPublicationMaterials(publicationFixtureID, d.ID, d.Owner, binding, plan, plan.Digest())
			if err != nil {
				t.Fatal(err)
			}
			sealed, err := prepared.Seal(t.Context(), f.local)
			if err != nil {
				t.Fatal(err)
			}
			wantErr := true
			switch kind {
			case "matching references":
				wantErr = false
			case "different full import":
				sealed.Import.ImportRevision = sealed.Import.Version
			case "changed private plan":
				plan.Cases[1].Answer = plan.Cases[0].Answer
			case "missing display":
				if err := os.Remove(filepath.Join(f.root, sealed.Statement.SHA256)); err != nil {
					t.Fatal(err)
				}
			case "missing formal answer":
				if err := os.Remove(filepath.Join(f.root, plan.Cases[1].Answer.SHA256)); err != nil {
					t.Fatal(err)
				}
			}
			if err := VerifySealedPublicationMaterials(t.Context(), f.local, sealed, plan); (err != nil) != wantErr {
				t.Fatalf("unexpected verification result: %v", err)
			}
		})
	}
}
