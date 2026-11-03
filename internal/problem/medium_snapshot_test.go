package problem_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func mediumSnapshotFixture(t *testing.T, input, answer []byte) *problem.Prepared {
	t.Helper()
	b := problemfixture.NewZAlgorithm(t)
	b.AddOriginal(t, "string/zalgorithm/sol/correct.cpp", []byte("int main(){}"))
	c := b.Document["test_cases"].([]any)[0].(map[string]any)
	for role, raw := range map[string][]byte{"input": input, "answer": answer} {
		sha := problemfixture.Hash(raw)
		c[role] = map[string]any{"path": "objects/sha256/" + sha, "sha256": sha, "size": len(raw)}
		b.Files["objects/sha256/"+sha] = raw
	}
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:zalgorithm", b.Lock)
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

func TestMediumDiskAdmissionExactBoundsAndSharedQuota(t *testing.T) {
	ctx := context.Background()
	input := bytes.Repeat([]byte{255}, 1<<20)
	answer := bytes.Repeat([]byte("1\n"), 2<<20)
	p := mediumSnapshotFixture(t, input, answer)
	if _, err := p.PrepareUnionFindReference(ctx, problem.UnionFindReferenceLimits()); !errors.Is(err, problem.ErrReferenceUnsupported) {
		t.Fatal("legacy source entry widened", err)
	}
	s, err := p.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	in, out, err := s.ReadCase(ctx, 0)
	if err != nil || !bytes.Equal(in, input) || !bytes.Equal(out, answer) {
		t.Fatal("medium exact boundaries changed", err)
	}
	_, other, _ := unionSnapshotFixture(t)
	if _, err := other.PrepareDiskReference(ctx); !errors.Is(err, problem.ErrBusy) {
		t.Fatal("source bypassed global snapshot quota", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	old, err := other.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal("quota retained", err)
	}
	_ = old.Close()
	for _, source := range []string{"library-checker:aplusb", "library-checker:other", ""} {
		if _, err := problem.DiskReferenceLimits(source); err == nil {
			t.Fatal("unknown disk source accepted")
		}
	}
}

func TestMediumDiskInputAnswerCeilings(t *testing.T) {
	for _, role := range []string{"input", "answer"} {
		t.Run(role, func(t *testing.T) {
			input, answer := []byte("input"), []byte("1\n")
			if role == "input" {
				input = bytes.Repeat([]byte{'i'}, (1<<20)+1)
			} else {
				answer = bytes.Repeat([]byte{'a'}, (4<<20)+1)
			}
			p := mediumSnapshotFixture(t, input, answer)
			if _, err := p.PrepareDiskReference(context.Background()); !errors.Is(err, problem.ErrReferenceUnsupported) {
				t.Fatal("medium material ceiling bypassed", err)
			}
		})
	}
}
