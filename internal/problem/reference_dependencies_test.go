package problem_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func dependencySnapshotFixture(t *testing.T, mutate func(*problemfixture.Bundle)) (*problemfixture.Bundle, *problem.Prepared) {
	t.Helper()
	b := problemfixture.NewPrimality(t)
	if mutate != nil {
		mutate(b)
	}
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:primality_test", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return b, p
}

func TestDependencySnapshotBindsLayoutAndUsesFrozenBytes(t *testing.T) {
	b, p := dependencySnapshotFixture(t, nil)
	s, err := p.PrepareDiskReference(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	plan := s.Plan()
	want := []string{"lib/prime.hpp", "lib/montgomery.hpp", "lib/util.hpp", "common/random.h"}
	var paths []string
	for _, dependency := range plan.Dependencies {
		paths = append(paths, dependency.Path)
	}
	if plan.SourcePath != "sol/correct.cpp" || !reflect.DeepEqual(paths, want) {
		t.Fatal("compilation layout not frozen")
	}
	originalDigest := plan.Digest()
	plan.Dependencies[0].Path = "changed/path.h"
	if plan.Digest() == originalDigest || s.Plan().Digest() != originalDigest {
		t.Fatal("layout identity or returned slice is not detached")
	}
	plan = s.Plan()
	plan.SourcePath = "changed.cpp"
	if plan.Digest() == originalDigest {
		t.Fatal("entry path omitted from identity")
	}
	deps, err := s.ReadDependencies(context.Background())
	if err != nil || len(deps) != 4 {
		t.Fatal(err)
	}
	raw := b.Document["provenance"].(map[string]any)["original_files"].(map[string]any)["source/common/random.h"].(map[string]any)
	path := filepath.Join(b.Directory, raw["path"].(string))
	if err := os.WriteFile(path, []byte("replaced after freezing"), 0600); err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), deps[3].Data...)
	deps[3].Data[0] ^= 255
	again, err := s.ReadDependencies(context.Background())
	if err != nil || !bytes.Equal(again[3].Data, before) {
		t.Fatal("dependency re-read live material or shared slices")
	}
	in, answer, err := s.ReadCase(context.Background(), 0)
	if err != nil || !bytes.Equal(in, []byte("1 2\n")) || !bytes.Equal(answer, []byte("3\n")) {
		t.Fatal("case spans shifted into dependency bytes")
	}
	encoded, err := json.Marshal(s.Plan())
	if err != nil || bytes.Contains(encoded, before) || bytes.Contains(encoded, []byte("inline int")) {
		t.Fatal("plan serialized dependency contents")
	}
}

func TestDependencySnapshotRejectsLastCorruptionAndAggregateLimit(t *testing.T) {
	b, p := dependencySnapshotFixture(t, nil)
	ref := b.Document["provenance"].(map[string]any)["original_files"].(map[string]any)["source/common/random.h"].(map[string]any)
	path := filepath.Join(b.Directory, ref["path"].(string))
	old := b.Files[ref["path"].(string)]
	changed := append([]byte(nil), old...)
	changed[0] ^= 255
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.PrepareDiskReference(context.Background()); !errors.Is(err, materialstore.ErrIntegrity) {
		t.Fatal("last dependency corruption accepted", err)
	}
	for _, over := range []bool{false, true} {
		_, prepared := dependencySnapshotFixture(t, func(b *problemfixture.Bundle) {
			orig := b.Document["provenance"].(map[string]any)["original_files"].(map[string]any)
			total := 0
			for _, name := range []string{"number_theory/primality_test/sol/correct.cpp", "number_theory/primality_test/lib/prime.hpp", "number_theory/primality_test/lib/montgomery.hpp", "number_theory/primality_test/lib/util.hpp"} {
				r := orig["source/"+name].(map[string]any)
				total += len(b.Files[r["path"].(string)])
			}
			size := (64 << 10) - total
			if over {
				size++
			}
			b.AddOriginal(t, "common/random.h", bytes.Repeat([]byte{'x'}, size))
		})
		s, err := prepared.PrepareDiskReference(context.Background())
		if over {
			if !errors.Is(err, problem.ErrReferenceUnsupported) {
				t.Fatal("aggregate source cap bypassed", err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
		}
	}
}

func TestDependencyReadCloseAndCancellationNeverReturnPartial(t *testing.T) {
	_, p := dependencySnapshotFixture(t, nil)
	s, err := p.PrepareDiskReference(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if files, err := s.ReadDependencies(ctx); !errors.Is(err, context.Canceled) || files != nil {
		t.Fatal("cancellation returned dependency bytes")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			files, err := s.ReadDependencies(context.Background())
			if err == nil && len(files) != 4 {
				t.Error("partial success")
			}
			if err != nil && files != nil {
				t.Error("partial bytes on failure")
			}
		})
	}
	wg.Go(func() { _ = s.Close() })
	wg.Wait()
	if files, err := s.ReadDependencies(context.Background()); err == nil || files != nil {
		t.Fatal("closed snapshot usable")
	}
}
