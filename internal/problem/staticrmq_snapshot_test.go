package problem_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func staticRMQSnapshotFixture(t *testing.T, input, answer []byte) *problem.Prepared {
	t.Helper()
	b := problemfixture.NewStaticRMQ(t)
	b.AddOriginal(t, "data_structure/staticrmq/sol/correct.cpp", []byte("int main(){}"))
	c := b.Document["test_cases"].([]any)[0].(map[string]any)
	for role, raw := range map[string][]byte{"input": input, "answer": answer} {
		sha := problemfixture.Hash(raw)
		c[role] = map[string]any{"path": "objects/sha256/" + sha, "size": len(raw), "sha256": sha}
		b.Files["objects/sha256/"+sha] = raw
	}
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:staticrmq", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestStaticRMQSnapshotCrossesOldCapAndKeepsSharedQuota(t *testing.T) {
	ctx := context.Background()
	old := repeatedAnswerFixture(t, problemfixture.NewSuffixArray(t), 33)
	if _, err := old.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("suffixarray material cap widened", err)
	}
	p := repeatedAnswerFixture(t, problemfixture.NewStaticRMQ(t), 33)
	s, err := p.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var total int64
	for _, point := range s.Plan().Cases {
		total += point.Input.Size + point.Answer.Size
		if point.InputData != nil || point.AnswerData != nil {
			t.Fatal("full private content retained in metadata")
		}
	}
	if total <= 128<<20 || total >= 256<<20 {
		t.Fatal("test did not cross the previous source cap")
	}
	answer, err := s.ReadAnswer(ctx, 32)
	if err != nil || len(answer) != 4<<20 || answer[0] != '1' || answer[len(answer)-1] != '\n' {
		t.Fatal("last large frozen point", err)
	}
	if _, err := old.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrBusy) {
		t.Fatal("staticrmq bypassed the shared snapshot quota", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	over := repeatedAnswerFixture(t, problemfixture.NewStaticRMQ(t), 64)
	if _, err := over.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("256MiB plus catalog admitted", err)
	}
	// Rejection must release the same quota, not just fail before returning data.
	valid := staticRMQSnapshotFixture(t, []byte("input"), []byte("1\n"))
	check, err := valid.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal("rejected set retained quota", err)
	}
	if err := check.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStaticRMQInputAndAnswerExactCeilings(t *testing.T) {
	ctx := context.Background()
	input := bytes.Repeat([]byte{255}, 16<<20)
	answer := bytes.Repeat([]byte("1\n"), 2<<20)
	p := staticRMQSnapshotFixture(t, input, answer)
	s, err := p.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	in, out, err := s.ReadCase(ctx, 0)
	if err != nil || !bytes.Equal(in, input) || !bytes.Equal(out, answer) {
		t.Fatal("exact source ceilings changed frozen bytes", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"input", "answer"} {
		t.Run(role, func(t *testing.T) {
			in, out := []byte("input"), []byte("1\n")
			if role == "input" {
				in = bytes.Repeat([]byte{'i'}, (16<<20)+1)
			} else {
				out = bytes.Repeat([]byte{'a'}, (4<<20)+1)
			}
			p := staticRMQSnapshotFixture(t, in, out)
			if _, err := p.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrReferenceUnsupported) {
				t.Fatal("source ceiling bypassed", err)
			}
		})
	}
}
