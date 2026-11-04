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

func convolutionSnapshotBundle(t *testing.T, input, answer []byte, count int) *problemfixture.Bundle {
	t.Helper()
	b := problemfixture.NewConvolution(t)
	b.AddOriginal(t, "convolution/convolution_mod/sol/correct.cpp", []byte("int main(){}"))
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

func loadConvolutionSnapshot(t *testing.T, b *problemfixture.Bundle) *problem.Prepared {
	t.Helper()
	p, err := problem.LoadBundle(b.Directory, "library-checker:convolution_mod", b.Lock)
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

func TestConvolutionLargeSnapshotAndQuotaRelease(t *testing.T) {
	ctx := context.Background()
	input, answer := bytes.Repeat([]byte{255}, 4<<20), bytes.Repeat([]byte("1\n"), 2<<20)
	p := loadConvolutionSnapshot(t, convolutionSnapshotBundle(t, input, answer, 53))
	s, err := p.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	plan := s.Plan()
	var total int64
	for _, c := range plan.Cases {
		total += c.Input.Size + c.Answer.Size
		if c.InputData != nil || c.AnswerData != nil {
			t.Fatal("plan retained private bytes")
		}
	}
	if total <= 256<<20 || total >= 512<<20 || len(plan.Cases) != 53 {
		t.Fatal("complete snapshot did not cross old total cap")
	}
	in, out, err := s.ReadCase(ctx, 52)
	if err != nil || !bytes.Equal(in, input) || !bytes.Equal(out, answer) {
		t.Fatal("last frozen point changed", err)
	}
	old := associativeSnapshotFixture(t, []byte("input"), []byte("1\n"), 1)
	if _, err := old.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrBusy) {
		t.Fatal("snapshot ownership not shared", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	over := loadConvolutionSnapshot(t, convolutionSnapshotBundle(t, bytes.Repeat([]byte{'i'}, 16<<20), bytes.Repeat([]byte("1\n"), 8<<20), 16))
	if _, err := over.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("512MiB plus catalogs admitted", err)
	}
	check, err := old.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal("admission failure retained shared quota", err)
	}
	if err := check.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConvolutionInputAndAnswerExactCeilings(t *testing.T) {
	for _, role := range []string{"input", "answer"} {
		for _, extra := range []int{0, 1} {
			in, out := []byte("input"), []byte("1\n")
			raw := bytes.Repeat([]byte{'x'}, (16<<20)+extra)
			if role == "input" {
				in = raw
			} else {
				out = raw
			}
			p := loadConvolutionSnapshot(t, convolutionSnapshotBundle(t, in, out, 1))
			s, err := p.PrepareDiskReference(context.Background())
			if extra == 1 {
				if !errors.Is(err, problem.ErrReferenceUnsupported) {
					t.Fatal("per-file cap bypassed", role, err)
				}
				continue
			}
			if err != nil {
				t.Fatal("exact cap refused", role, err)
			}
			actualIn, actualOut, err := s.ReadCase(context.Background(), 0)
			if err != nil || !bytes.Equal(actualIn, in) || !bytes.Equal(actualOut, out) {
				t.Fatal("exact bytes changed", role, err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestConvolutionTotalExactCapIncludesSourceAndCatalogs(t *testing.T) {
	b := convolutionSnapshotBundle(t, bytes.Repeat([]byte{'i'}, 16<<20), bytes.Repeat([]byte("1\n"), 8<<20), 16)
	// Spend precisely the space removed from one input on source and catalogs.
	input := bytes.Repeat([]byte{'i'}, (16<<20)-(64<<10))
	sha := problemfixture.Hash(input)
	b.Files["objects/sha256/"+sha] = input
	b.Document["test_cases"].([]any)[0].(map[string]any)["input"] = map[string]any{"path": "objects/sha256/" + sha, "size": len(input), "sha256": sha}
	b.BindReferenceCases(t)
	originals := b.Document["provenance"].(map[string]any)["original_files"].(map[string]any)
	sourceSize := 64 << 10
	for _, name := range []string{"info.toml", "hash.json"} {
		sourceSize -= originals["source/convolution/convolution_mod/"+name].(map[string]any)["size"].(int)
	}
	if sourceSize <= 0 {
		t.Fatal("catalogs exceed reserved space")
	}
	b.AddOriginal(t, "convolution/convolution_mod/sol/correct.cpp", bytes.Repeat([]byte{'s'}, sourceSize))
	b.Write(t)
	p := loadConvolutionSnapshot(t, b)
	s, err := p.PrepareDiskReference(context.Background())
	if err != nil {
		t.Fatal("exact 512MiB snapshot rejected", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	plan := s.Plan()
	total := plan.Source.Size + plan.Definition.Size + plan.Hashes.Size
	for _, c := range plan.Cases {
		total += c.Input.Size + c.Answer.Size
	}
	if total != 512<<20 {
		t.Fatal("full-material cap not reached", total)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	b.AddOriginal(t, "convolution/convolution_mod/sol/correct.cpp", bytes.Repeat([]byte{'s'}, sourceSize+1))
	b.Write(t)
	if _, err := loadConvolutionSnapshot(t, b).PrepareDiskReference(context.Background()); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("exact cap plus one admitted", err)
	}
}
