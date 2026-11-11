package problem_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func unionSnapshotFixture(t *testing.T) (*problemfixture.Bundle, *problem.Prepared, []byte) {
	t.Helper()
	b := problemfixture.NewUnionFind(t)
	b.AddOriginal(t, "data_structure/unionfind/sol/correct.cpp", []byte("int main(){}"))
	input := bytes.Repeat([]byte{0, 255, 2, '\n'}, 3<<18)
	sha := problemfixture.Hash(input)
	b.Files["objects/sha256/"+sha] = input
	b.Document["test_cases"].([]any)[0].(map[string]any)["input"] = map[string]any{"path": "objects/sha256/" + sha, "size": len(input), "sha256": sha}
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:unionfind", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	return b, p, input
}

func TestDiskReferenceFrozenBytesOwnershipAndClose(t *testing.T) {
	b, p, input := unionSnapshotFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := p.PrepareUnionFindReference(ctx, problem.UnionFindReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if !errors.Is(p.Close(), problem.ErrBusy) {
		t.Fatal("prepared lost snapshot ownership")
	}
	_, other, _ := unionSnapshotFixture(t)
	if _, err := other.PrepareUnionFindReference(ctx, problem.UnionFindReferenceLimits()); !errors.Is(err, problem.ErrBusy) {
		t.Fatal("aggregate snapshot quota bypassed", err)
	}
	plan := s.Plan()
	digest := plan.Digest()
	if plan.SourceData != nil || plan.Cases[0].InputData != nil || plan.Cases[0].AnswerData != nil {
		t.Fatal("plan retained private bytes")
	}
	path := filepath.Join(b.Directory, "objects/sha256/"+plan.Cases[0].Input.SHA256)
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, len(input)), 0600); err != nil {
		t.Fatal(err)
	}
	plan.Cases[0].Input.Size = 1
	plan.Cases[0].ID = "untrusted"
	in, answer, err := s.ReadCase(ctx, 0)
	if err != nil || !bytes.Equal(in, input) || string(answer) != "3\n" || s.Plan().Digest() != digest {
		t.Fatal("frozen object or offsets changed", err)
	}
	in[0] = 9
	again, _, err := s.ReadCase(ctx, 0)
	if err != nil || again[0] != 0 {
		t.Fatal("read slices aliased", err)
	}
	if _, err := s.ReadAnswer(ctx, -1); !errors.Is(err, problem.ErrInvalid) {
		t.Fatal("bad index", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, e := s.ReadSource(ctx)
			if e != nil || string(raw) != "int main(){}" {
				t.Error("concurrent read", e)
			}
		}()
	}
	wg.Wait()
	cancel()
	if _, err := s.ReadAnswer(context.Background(), 0); !errors.Is(err, context.Canceled) {
		t.Fatal("snapshot context lost", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal("repeated close", err)
	}
	if _, err := s.ReadSource(context.Background()); !errors.Is(err, problem.ErrInvalid) {
		t.Fatal("closed snapshot read", err)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := other.PrepareUnionFindReference(context.Background(), problem.UnionFindReferenceLimits())
	if err != nil {
		t.Fatal("quota not released", err)
	}
	if err := s2.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBorrowedAnswerBoundsLifetimeAndConcurrentClose(t *testing.T) {
	b, p, _ := unionSnapshotFixture(t)
	life, stop := context.WithCancel(t.Context())
	defer stop()
	s, err := p.PrepareUnionFindReference(life, problem.UnionFindReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	call, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader, size, err := s.AnswerAt(call, 0)
	if err != nil || size != 2 {
		t.Fatal(size, err)
	}
	plan := s.Plan()
	path := filepath.Join(b.Directory, "objects/sha256/"+plan.Cases[0].Answer.SHA256)
	if err := os.WriteFile(path, []byte("9\n"), 0600); err != nil {
		t.Fatal(err)
	}
	buf := []byte("xxxx")
	n, err := reader.ReadAt(buf, 0)
	if n != 2 || err != io.EOF || string(buf) != "3\nxx" {
		t.Fatalf("span leaked adjacent bytes or reflected original: n=%d err=%v bytes=%q", n, err, buf)
	}
	if n, err := reader.ReadAt(buf, size); n != 0 || err != io.EOF {
		t.Fatalf("span end was readable: %d %v", n, err)
	}
	if _, _, err := s.AnswerAt(t.Context(), -1); !errors.Is(err, problem.ErrInvalid) {
		t.Fatal(err)
	}
	cancel()
	if n, err := reader.ReadAt(buf, 0); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("call cancellation lost: %d %v", n, err)
	}
	reader, _, err = s.AnswerAt(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 10 {
		group.Go(func() {
			var dest [2]byte
			n, err := reader.ReadAt(dest[:], 0)
			readComplete := n == 2 && err == nil && string(dest[:]) == "3\n"
			alreadyClosed := n == 0 && errors.Is(err, problem.ErrInvalid)
			if readComplete || alreadyClosed {
				return
			}
			t.Errorf("read/close returned %d, %v", n, err)
		})
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	group.Wait()
	if n, err := reader.ReadAt(buf, 0); n != 0 || !errors.Is(err, problem.ErrInvalid) {
		t.Fatalf("borrowed reader survived snapshot close: %d %v", n, err)
	}
	_, second, _ := unionSnapshotFixture(t)
	s2, err := second.PrepareUnionFindReference(life, problem.UnionFindReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s2.Close(); err != nil {
			t.Error(err)
		}
	})
	r2, _, err := s2.AnswerAt(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if n, err := r2.ReadAt(buf, 0); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("snapshot cancellation lost: %d %v", n, err)
	}
}

func TestDiskReferenceRejectsCorruptionLimitsAndOldAdmission(t *testing.T) {
	b, p, _ := unionSnapshotFixture(t)
	limits := problem.UnionFindReferenceLimits()
	if _, err := p.PrepareReference(context.Background(), limits); !errors.Is(err, problem.ErrInvalid) {
		t.Fatal("old limits widened", err)
	}
	limits.TotalBytes++
	if _, err := p.PrepareUnionFindReference(context.Background(), limits); !errors.Is(err, problem.ErrInvalid) {
		t.Fatal("arbitrary disk budget", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.PrepareUnionFindReference(ctx, problem.UnionFindReferenceLimits()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel", err)
	}
	ref := b.Document["test_cases"].([]any)[0].(map[string]any)["answer"].(map[string]any)
	if err := os.WriteFile(filepath.Join(b.Directory, ref["path"].(string)), []byte("4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.PrepareUnionFindReference(context.Background(), problem.UnionFindReferenceLimits()); err == nil {
		t.Fatal("corrupt answer admitted")
	}
	_, other, _ := unionSnapshotFixture(t)
	s, err := other.PrepareUnionFindReference(context.Background(), problem.UnionFindReferenceLimits())
	if err != nil {
		t.Fatal("failed preparation leaked ownership", err)
	}
	_ = s.Close()
}

func TestDiskReferenceConcurrentReadAndClose(t *testing.T) {
	_, p, _ := unionSnapshotFixture(t)
	s, err := p.PrepareUnionFindReference(context.Background(), problem.UnionFindReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			in, answer, e := s.ReadCase(context.Background(), 0)
			if e == nil {
				if len(in) != 3<<20 || string(answer) != "3\n" {
					t.Error("partial concurrent read")
				}
			} else if !errors.Is(e, problem.ErrInvalid) {
				t.Error(e)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if e := s.Close(); e != nil {
			t.Error(e)
		}
	}()
	close(start)
	wg.Wait()
	if err := p.Close(); err != nil {
		t.Fatal("close did not release prepared", err)
	}
}

func TestDiskReferenceRejectsWholeSetBudgets(t *testing.T) {
	for _, role := range []string{"input", "answer", "total", "cases"} {
		t.Run(role, func(t *testing.T) {
			b := problemfixture.NewUnionFind(t)
			b.AddOriginal(t, "data_structure/unionfind/sol/correct.cpp", []byte("int main(){}"))
			first := b.Document["test_cases"].([]any)[0].(map[string]any)
			size, count, target := 4<<20+1, 1, "input"
			switch role {
			case "answer":
				size, target = 256<<10+1, "answer"
			case "total":
				size, count = 3<<20, 22
			case "cases":
				size, count = 1, 65
			}
			raw := bytes.Repeat([]byte{'x'}, size)
			sha := problemfixture.Hash(raw)
			first[target] = map[string]any{"path": "objects/sha256/" + sha, "size": size, "sha256": sha}
			b.Files["objects/sha256/"+sha] = raw
			provenance := b.Document["provenance"].(map[string]any)
			for i := 1; i < count; i++ {
				id := fmt.Sprintf("case-%04d", i+1)
				b.Document["test_cases"] = append(b.Document["test_cases"].([]any), map[string]any{"id": id, "input": first["input"], "answer": first["answer"], "sample": false})
				provenance["source_case_mapping"] = append(provenance["source_case_mapping"].([]any), map[string]any{"case_id": id, "upstream_case": fmt.Sprintf("random_%02d", i-1)})
			}
			b.BindReferenceCases(t)
			b.Write(t)
			p, err := problem.LoadBundle(b.Directory, "library-checker:unionfind", b.Lock)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = p.Close() }()
			if _, err := p.PrepareUnionFindReference(context.Background(), problem.UnionFindReferenceLimits()); !errors.Is(err, problem.ErrReferenceUnsupported) {
				t.Fatal("whole-set budget admitted", err)
			}
		})
	}
}
