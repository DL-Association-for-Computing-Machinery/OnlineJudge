package problem

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

type observedConvolutionObjects struct {
	*materialstore.Local
	opens []materialstore.Object
}

func (s *observedConvolutionObjects) Open(ctx context.Context, o materialstore.Object) (*materialstore.Snapshot, error) {
	s.opens = append(s.opens, o)
	return s.Local.Open(ctx, o)
}

func TestFreezeConvolutionUsesOnlyBoundPersistentSolverObjects(t *testing.T) {
	const id = "00000000000040008000000000000001"
	const owner = "00000000000040008000000000000002"
	const oracleBytes, candidateBytes = "synthetic oracle program", "synthetic candidate program"
	b := problemfixture.NewConvolution(t)
	b.AddOriginal(t, convolutionDraftPath+"/sol/naive.cpp", []byte(oracleBytes))
	b.AddOriginal(t, convolutionDraftPath+"/sol/correct.cpp", []byte(candidateBytes))
	b.Write(t)
	prepared, err := LoadBundle(b.Directory, "library-checker:convolution_mod", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	content := prepared.Content()
	expected, err := prepared.ImportIdentity()
	if err != nil {
		t.Fatal(err)
	}
	closePrepared(t, prepared)
	draft := Draft{ID: id, Owner: owner, SourceID: content.SourceID, Version: content.Version,
		ImportRevision: content.RequestSHA256, State: "private-draft", ValidationState: "not-run",
		Document: content.Document, References: content.References}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	local, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 64 << 10, MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })
	store := &observedConvolutionObjects{Local: local}
	objects := make([]materialstore.Object, 0, 2)
	for _, source := range []string{oracleBytes, candidateBytes} {
		object := materialstore.Object{SHA256: hash([]byte(source)), Size: int64(len(source))}
		if _, err := local.Put(t.Context(), object, strings.NewReader(source)); err != nil {
			t.Fatal(err)
		}
		objects = append(objects, object)
	}
	// None of the other draft objects are present; the real store has one I/O slot.
	pins := ConvolutionSolverObjects{Oracle: objects[0], Candidate: objects[1]}
	snapshot, err := draft.FreezeConvolutionSources(t.Context(), id, owner, expected, pins, store)
	if err != nil || snapshot.Oracle != oracleBytes || snapshot.Candidate != candidateBytes || !slices.Equal(store.opens, objects) {
		t.Fatalf("freeze did not close and open only the two expected roles: %v, %v", store.opens, err)
	}
	if raw, err := json.Marshal(snapshot); err != nil || strings.Contains(string(raw), oracleBytes) || strings.Contains(string(raw), candidateBytes) {
		t.Fatal("private compilation bytes reached JSON")
	}
	store.opens = nil
	changed := draft
	changed.References = slices.Clone(draft.References)
	changed.References[0].Object = objects[1]
	if s, err := changed.FreezeConvolutionSources(t.Context(), id, owner, expected, pins, store); s != (FrozenConvolutionSources{}) ||
		!errors.Is(err, ErrInvalid) || len(store.opens) != 0 {
		t.Fatal("changed complete revision reached object I/O", err)
	}
	swapped := ConvolutionSolverObjects{Oracle: pins.Candidate, Candidate: pins.Oracle}
	if s, err := draft.FreezeConvolutionSources(t.Context(), id, owner, expected, swapped, store); s != (FrozenConvolutionSources{}) ||
		!errors.Is(err, ErrInvalid) || len(store.opens) != 0 {
		t.Fatal("unreviewed solver roles reached object I/O", err)
	}
	// A same-length corruption must not yield even a partial pair.
	if err := os.WriteFile(filepath.Join(root, objects[1].SHA256), []byte(strings.Repeat("x", len(candidateBytes))), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := draft.FreezeConvolutionSources(t.Context(), id, owner, expected, pins, store); s != (FrozenConvolutionSources{}) ||
		!errors.Is(err, materialstore.ErrIntegrity) || !slices.Equal(store.opens, objects) {
		t.Fatal("corrupted candidate escaped the paired snapshot", err)
	}
}
