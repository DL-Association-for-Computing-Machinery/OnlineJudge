package problem

import (
	"context"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestSnapshotCloseFailureIsStableAndReleasesOwnership(t *testing.T) {
	b := problemfixture.NewUnionFind(t)
	b.AddOriginal(t, "data_structure/unionfind/sol/correct.cpp", []byte("int main(){}"))
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := LoadBundle(b.Directory, "library-checker:unionfind", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	s, err := p.PrepareUnionFindReference(context.Background(), UnionFindReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.file.Close(); err != nil {
		t.Fatal(err)
	}
	err = s.Close()
	if err == nil || s.Close() != err || strings.Contains(err.Error(), "snapshot-") {
		t.Fatal("close evidence not stable/redacted", err)
	}
	if err := p.Close(); err != nil {
		t.Fatal("failed close retained quota", err)
	}
}
