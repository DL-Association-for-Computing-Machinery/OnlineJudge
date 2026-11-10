package problemreference

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func freezeFixture(t *testing.T, b *problemfixture.Bundle, source []byte) (*problem.Prepared, Request, problem.Draft, map[string][]byte) {
	t.Helper()
	provenance := b.Document["provenance"].(map[string]any)
	if source != nil {
		b.AddOriginal(t, provenance["source_path"].(string)+"/sol/correct.cpp", source)
	}
	b.BindReferenceCases(t)
	p := preparedCatalogFixture(t, b)
	content := p.Content()
	catalog, err := NewCatalog(map[string]*problem.Prepared{content.SourceID: p})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := catalog.Select(draftSelection(t, p))
	if err != nil {
		t.Fatal(err)
	}
	d := problem.Draft{ID: strings.Repeat("1", 32), Owner: strings.Repeat("2", 32), SourceID: content.SourceID,
		Version: content.Version, ImportRevision: content.RequestSHA256, Document: content.Document, References: content.References,
		State: "private-draft", ValidationState: "not-run"}
	objects := make(map[string][]byte)
	for _, raw := range b.Files {
		objects[problemfixture.Hash(raw)] = raw
	}
	return p, Request{Owner: d.Owner, DraftID: d.ID, State: Queued, Specification: spec}, d, objects
}

func freezeStore(t *testing.T, d problem.Draft, objects map[string][]byte) (*materialstore.Local, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "objects")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, ref := range d.References {
		raw, ok := objects[ref.Object.SHA256]
		if !ok {
			t.Fatal("fixture object missing")
		}
		if _, err := s.Put(t.Context(), ref.Object, bytes.NewReader(raw)); err != nil {
			t.Fatal(err)
		}
	}
	return s, root
}

func TestFreezeDraftMatchesAllMaintenancePlans(t *testing.T) {
	for _, tc := range []struct {
		name string
		new  func(*testing.T) *problemfixture.Bundle
	}{
		{"aplusb", problemfixture.New}, {"unionfind", problemfixture.NewUnionFind},
		{"zalgorithm", problemfixture.NewZAlgorithm}, {"suffixarray", problemfixture.NewSuffixArray},
		{"primality_test", problemfixture.NewPrimality}, {"static_range_sum", problemfixture.NewStaticRangeSum},
		{"associative_array", problemfixture.NewAssociativeArray}, {"point_add_range_sum", problemfixture.NewPointAddRangeSum},
		{"staticrmq", problemfixture.NewStaticRMQ}, {"convolution_mod", problemfixture.NewConvolution},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			b := tc.new(t)
			source := []byte("synthetic source bytes\n")
			if tc.name == "primality_test" {
				source = nil // retain the fixture's fixed dependency closure
			}
			p, req, draft, objects := freezeFixture(t, b, source)
			store, _ := freezeStore(t, draft, objects)
			var want problem.ReferencePlan
			if tc.name == "aplusb" {
				var err error
				want, err = p.PrepareReference(ctx, problem.SmallReferenceLimits())
				if err != nil {
					t.Fatal(err)
				}
				want.SourceData = nil
				for i := range want.Cases {
					want.Cases[i].InputData, want.Cases[i].AnswerData = nil, nil
				}
			} else {
				legacy, err := p.PrepareDiskReference(ctx)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = legacy.Close() })
				want = legacy.Plan()
				if s, err := req.FreezeDraft(ctx, draft, store); s != nil || !errors.Is(err, problem.ErrBusy) {
					t.Fatal("persisted preparation bypassed the existing snapshot slot", err)
				}
				if err := legacy.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if err := p.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(b.Directory); err != nil {
				t.Fatal(err)
			}
			frozen, err := req.FreezeDraft(ctx, draft, store)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = frozen.Close() })
			if got := frozen.Plan(); !reflect.DeepEqual(got, want) || got.Digest() != want.Digest() {
				t.Fatal("persisted material plan differs from bundle plan")
			}
			if err := store.Close(); err != nil {
				t.Fatal("freeze retained an object-store operation", err)
			}
			raw, err := frozen.ReadSource(ctx)
			if err != nil || !bytes.Equal(raw, objects[want.Source.SHA256]) {
				t.Fatal("frozen source changed", err)
			}
			deps, err := frozen.ReadDependencies(ctx)
			if err != nil || len(deps) != len(want.Dependencies) {
				t.Fatal("wrong frozen dependency closure", err)
			}
			for i, dep := range want.Dependencies {
				if deps[i].Path != dep.Path || !bytes.Equal(deps[i].Data, objects[dep.Object.SHA256]) {
					t.Fatal("frozen dependency changed")
				}
			}
			for i, point := range want.Cases {
				in, out, err := frozen.ReadCase(ctx, i)
				if err != nil || !bytes.Equal(in, objects[point.Input.SHA256]) || !bytes.Equal(out, objects[point.Answer.SHA256]) {
					t.Fatal("frozen point bytes changed", err)
				}
			}
		})
	}
}

type freezeOpenFunc func(context.Context, materialstore.Object) (*materialstore.Snapshot, error)

func (f freezeOpenFunc) Open(ctx context.Context, o materialstore.Object) (*materialstore.Snapshot, error) {
	return f(ctx, o)
}

func TestFreezeDraftRejectsBeforeMaterialIO(t *testing.T) {
	_, req, draft, _ := freezeFixture(t, problemfixture.New(t), []byte("source\n"))
	noIO := freezeOpenFunc(func(context.Context, materialstore.Object) (*materialstore.Snapshot, error) {
		t.Fatal("invalid request reached object store")
		return nil, nil
	})
	changed := req
	changed.Specification.ProfileJSON = []byte(`{"id":"retired-profile"}`)
	changed.Specification.ProfileSHA256 = profileDigest(changed.Specification.ProfileJSON)
	if s, err := changed.FreezeDraft(t.Context(), draft, noIO); s != nil || !errors.Is(err, ErrSourceChanged) {
		t.Fatal("retired profile was accepted", err)
	}
	changed = req
	changed.State = Cancelled
	if s, err := changed.FreezeDraft(t.Context(), draft, noIO); s != nil || !errors.Is(err, ErrInvalid) {
		t.Fatal("cancelled receipt was accepted", err)
	}
	draft.References = draft.References[1:]
	if s, err := req.FreezeDraft(t.Context(), draft, noIO); s != nil || !errors.Is(err, problem.ErrInvalid) {
		t.Fatal("incomplete metadata was accepted", err)
	}
	_, req, draft, _ = freezeFixture(t, problemfixture.New(t), bytes.Repeat([]byte{'s'}, 65<<10))
	if s, err := req.FreezeDraft(t.Context(), draft, noIO); s != nil || !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("oversize source was accepted", err)
	}
}

func TestFreezeDraftLateFailureReleasesBothSlots(t *testing.T) {
	_, req, draft, objects := freezeFixture(t, problemfixture.NewUnionFind(t), []byte("source\n"))
	store, root := freezeStore(t, draft, objects)
	answer := []byte("3\n")
	answerHash := problemfixture.Hash(answer)
	path := filepath.Join(root, answerHash)
	if err := os.WriteFile(path, []byte("9\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := req.FreezeDraft(t.Context(), draft, store); s != nil || !errors.Is(err, materialstore.ErrIntegrity) {
		t.Fatal("same-size late corruption returned a snapshot", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if s, err := req.FreezeDraft(t.Context(), draft, store); s != nil || !errors.Is(err, materialstore.ErrNotFound) || strings.Contains(err.Error(), root) {
		t.Fatal("missing object was accepted or leaked storage details", err)
	}
	if _, err := store.Put(t.Context(), materialstore.Object{SHA256: answerHash, Size: int64(len(answer))}, bytes.NewReader(answer)); err != nil {
		t.Fatal("object slot leaked after failure", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cancelOnAnswer := freezeOpenFunc(func(ctx context.Context, object materialstore.Object) (*materialstore.Snapshot, error) {
		s, err := store.Open(ctx, object)
		if err == nil && object.SHA256 == answerHash {
			cancel()
		}
		return s, err
	})
	if s, err := req.FreezeDraft(ctx, draft, cancelOnAnswer); s != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("late cancellation returned a snapshot", err)
	}
	s, err := req.FreezeDraft(t.Context(), draft, store)
	if err != nil {
		t.Fatal("snapshot or object slot leaked after failure", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
