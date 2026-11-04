package problem_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestPointAddCompleteSnapshotAdmissionAndSharedQuota(t *testing.T) {
	ctx := context.Background()
	old := repeatedAnswerFixture(t, problemfixture.NewSuffixArray(t), 33)
	if _, err := old.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("old 128MiB cap widened", err)
	}
	p := repeatedAnswerFixture(t, problemfixture.NewPointAddRangeSum(t), 33)
	s, err := p.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal("greater-than-128MiB snapshot refused", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	plan := s.Plan()
	var total int64
	for _, c := range plan.Cases {
		total += c.Input.Size + c.Answer.Size
		if c.InputData != nil || c.AnswerData != nil {
			t.Fatal("full material retained in plan")
		}
	}
	answer, err := s.ReadAnswer(ctx, 32)
	if total <= 128<<20 || total >= 256<<20 || err != nil || len(answer) != 4<<20 || answer[len(answer)-1] != '\n' {
		t.Fatal("larger snapshot not frozen/read", err)
	}
	if _, err := old.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrBusy) {
		t.Fatal("source bypassed shared snapshot quota", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	over := repeatedAnswerFixture(t, problemfixture.NewPointAddRangeSum(t), 64)
	if _, err := over.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("complete set exceeded 256MiB", err)
	}
}

func TestPointAddInputLimit(t *testing.T) {
	for _, size := range []int{16 << 20, (16 << 20) + 1} {
		b := problemfixture.NewPointAddRangeSum(t)
		b.AddOriginal(t, "data_structure/point_add_range_sum/sol/correct.cpp", []byte("int main(){}"))
		input := bytes.Repeat([]byte{'x'}, size)
		sha := problemfixture.Hash(input)
		b.Files["objects/sha256/"+sha] = input
		b.Document["test_cases"].([]any)[0].(map[string]any)["input"] = map[string]any{"path": "objects/sha256/" + sha, "size": size, "sha256": sha}
		b.BindReferenceCases(t)
		b.Write(t)
		p, err := problem.LoadBundle(b.Directory, "library-checker:point_add_range_sum", b.Lock)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = p.Close() })
		s, err := p.PrepareDiskReference(context.Background())
		if size == 16<<20 {
			if err != nil || s.Plan().Cases[0].Input.Size != int64(size) {
				t.Fatal("input cap refused", err)
			}
			actual, _, err := s.ReadCase(context.Background(), 0)
			if err != nil || !bytes.Equal(actual, input) {
				t.Fatal("input cap bytes not frozen", err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(err, problem.ErrReferenceUnsupported) {
			t.Fatal("input cap bypassed", err)
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
