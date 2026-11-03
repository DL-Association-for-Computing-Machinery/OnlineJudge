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

func repeatedAnswerFixture(t *testing.T, b *problemfixture.Bundle, count int) *problem.Prepared {
	t.Helper()
	prov := b.Document["provenance"].(map[string]any)
	b.AddOriginal(t, prov["source_path"].(string)+"/sol/correct.cpp", []byte("int main(){}"))
	answer := bytes.Repeat([]byte("1\n"), 2<<20)
	sha := problemfixture.Hash(answer)
	b.Files["objects/sha256/"+sha] = answer
	first := b.Document["test_cases"].([]any)[0].(map[string]any)
	first["answer"] = map[string]any{"path": "objects/sha256/" + sha, "size": len(answer), "sha256": sha}
	for i := 1; i < count; i++ {
		id := fmt.Sprintf("case-%04d", i+1)
		b.Document["test_cases"] = append(b.Document["test_cases"].([]any), map[string]any{"id": id, "input": first["input"], "answer": first["answer"], "sample": false})
		prov["source_case_mapping"] = append(prov["source_case_mapping"].([]any), map[string]any{"case_id": id, "upstream_case": fmt.Sprintf("random_%02d", i-1)})
	}
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, prov["source_id"].(string), b.Lock)
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

func TestSuffixArrayLargerSnapshotDoesNotWidenOtherSources(t *testing.T) {
	ctx := context.Background()
	old := repeatedAnswerFixture(t, problemfixture.NewZAlgorithm(t), 17)
	if _, err := old.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("old 64MiB cap widened", err)
	}
	p := repeatedAnswerFixture(t, problemfixture.NewSuffixArray(t), 17)
	s, err := p.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal("greater-than-64MiB snapshot refused", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	plan := s.Plan()
	var total int64
	for _, c := range plan.Cases {
		total += c.Input.Size + c.Answer.Size
		if c.InputData != nil || c.AnswerData != nil {
			t.Fatal("private bytes retained in full plan")
		}
	}
	if total <= 64<<20 || total >= 128<<20 || len(plan.Cases) != 17 {
		t.Fatal("test did not cross old complete-set cap")
	}
	answer, err := s.ReadAnswer(ctx, 16)
	if err != nil || len(answer) != 4<<20 || answer[0] != '1' || answer[len(answer)-1] != '\n' {
		t.Fatal("large snapshot final point unreadable", err)
	}
	if _, err := old.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrBusy) {
		t.Fatal("new source bypassed shared quota", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	over := repeatedAnswerFixture(t, problemfixture.NewSuffixArray(t), 32)
	if _, err := over.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("128MiB complete-set cap bypassed", err)
	}
}
