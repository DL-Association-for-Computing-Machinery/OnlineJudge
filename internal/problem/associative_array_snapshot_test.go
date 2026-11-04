package problem_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func associativeSnapshotBundle(t *testing.T, input, answer []byte, count int) *problemfixture.Bundle {
	t.Helper()
	b := problemfixture.NewAssociativeArray(t)
	b.AddOriginal(t, "data_structure/associative_array/sol/correct.cpp", []byte("int main(){}"))
	refs := map[string]any{}
	for role, raw := range map[string][]byte{"input": input, "answer": answer} {
		sha := problemfixture.Hash(raw)
		refs[role] = map[string]any{"path": "objects/sha256/" + sha, "size": len(raw), "sha256": sha}
		b.Files["objects/sha256/"+sha] = raw
	}
	cases, mapping := make([]any, 0, count), make([]any, 0, count)
	for i := 0; i < count; i++ {
		id, upstream := fmt.Sprintf("case-%04d", i+1), fmt.Sprintf("random_%02d", i-1)
		if i == 0 {
			upstream = "example_00"
		}
		cases = append(cases, map[string]any{"id": id, "input": refs["input"], "answer": refs["answer"], "sample": i == 0})
		mapping = append(mapping, map[string]any{"case_id": id, "upstream_case": upstream})
	}
	b.Document["test_cases"] = cases
	b.Document["provenance"].(map[string]any)["source_case_mapping"] = mapping
	b.BindReferenceCases(t)
	b.Write(t)
	return b
}

func loadAssociativeSnapshot(t *testing.T, b *problemfixture.Bundle) *problem.Prepared {
	t.Helper()
	p, err := problem.LoadBundle(b.Directory, "library-checker:associative_array", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	return p
}

func associativeSnapshotFixture(t *testing.T, input, answer []byte, count int) *problem.Prepared {
	t.Helper()
	return loadAssociativeSnapshot(t, associativeSnapshotBundle(t, input, answer, count))
}

func TestAssociativeArrayLargeCompleteSnapshotAndSharedQuota(t *testing.T) {
	ctx := context.Background()
	input, answer := bytes.Repeat([]byte{255}, 8<<20), bytes.Repeat([]byte("1\n"), 4<<20)
	p := associativeSnapshotFixture(t, input, answer, 20)
	s, err := p.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	plan := s.Plan()
	var total int64
	for _, point := range plan.Cases {
		total += point.Input.Size + point.Answer.Size
		if point.InputData != nil || point.AnswerData != nil {
			t.Fatal("full plan retained private material")
		}
	}
	if total <= 256<<20 || total >= 512<<20 || len(plan.Cases) != 20 {
		t.Fatal("complete snapshot did not cross prior 256MiB ceiling")
	}
	in, out, err := s.ReadCase(ctx, 19)
	if err != nil || !bytes.Equal(in, input) || !bytes.Equal(out, answer) {
		t.Fatal("last frozen point changed", err)
	}
	old := staticRangeSumSnapshotFixture(t, []byte("input"), []byte("1\n"))
	if _, err := old.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrBusy) {
		t.Fatal("new source bypassed shared snapshot ownership", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	over := associativeSnapshotFixture(t, input, answer, 32)
	if _, err := over.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("512MiB plus source and catalogs accepted", err)
	}
	check, err := old.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal("failed admission retained shared quota", err)
	}
	if err := check.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAssociativeArrayInputAndAnswerCompleteCeilings(t *testing.T) {
	ctx := context.Background()
	input, answer := bytes.Repeat([]byte{255}, 48<<20), bytes.Repeat([]byte("1\n"), 4<<20)
	p := associativeSnapshotFixture(t, input, answer, 1)
	s, err := p.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	in, out, err := s.ReadCase(ctx, 0)
	if err != nil || !bytes.Equal(in, input) || !bytes.Equal(out, answer) {
		t.Fatal("complete exact input/answer ceiling changed", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"input", "answer"} {
		t.Run(role, func(t *testing.T) {
			in, out := []byte("input"), []byte("1\n")
			if role == "input" {
				in = bytes.Repeat([]byte{'i'}, (48<<20)+1)
			} else {
				out = bytes.Repeat([]byte{'a'}, (8<<20)+1)
			}
			p := associativeSnapshotFixture(t, in, out, 1)
			if _, err := p.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrReferenceUnsupported) {
				t.Fatal("source ceiling exceeded", err)
			}
		})
	}
}

func TestAssociativeArrayTotalExactCapIncludesSourceAndCatalogs(t *testing.T) {
	b := associativeSnapshotBundle(t, bytes.Repeat([]byte{'i'}, 8<<20), bytes.Repeat([]byte("1\n"), 4<<20), 32)
	// Remove 64KiB from one input, then spend that exact space on the source
	// and both catalogs. Hash-list size is unchanged by replacing a hash.
	input := bytes.Repeat([]byte{'i'}, (8<<20)-(64<<10))
	sha := problemfixture.Hash(input)
	b.Files["objects/sha256/"+sha] = input
	b.Document["test_cases"].([]any)[0].(map[string]any)["input"] = map[string]any{"path": "objects/sha256/" + sha, "size": len(input), "sha256": sha}
	b.BindReferenceCases(t)
	originals := b.Document["provenance"].(map[string]any)["original_files"].(map[string]any)
	sourceSize := 64 << 10
	for _, name := range []string{"info.toml", "hash.json"} {
		sourceSize -= originals["source/data_structure/associative_array/"+name].(map[string]any)["size"].(int)
	}
	if sourceSize <= 0 {
		t.Fatal("catalog fixture exceeded reserved space")
	}
	b.AddOriginal(t, "data_structure/associative_array/sol/correct.cpp", bytes.Repeat([]byte{'s'}, sourceSize))
	b.Write(t)
	p := loadAssociativeSnapshot(t, b)
	s, err := p.PrepareDiskReference(context.Background())
	if err != nil {
		t.Fatal("exact 512MiB complete snapshot rejected", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	plan := s.Plan()
	total := plan.Source.Size + plan.Definition.Size + plan.Hashes.Size
	for _, point := range plan.Cases {
		total += point.Input.Size + point.Answer.Size
	}
	if total != 512<<20 {
		t.Fatal("test did not reach the exact full-material cap", total)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	b.AddOriginal(t, "data_structure/associative_array/sol/correct.cpp", bytes.Repeat([]byte{'s'}, sourceSize+1))
	b.Write(t)
	over := loadAssociativeSnapshot(t, b)
	if _, err := over.PrepareDiskReference(context.Background()); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("512MiB plus one source byte admitted", err)
	}
}
