package problemvalidation_test

import (
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestSourceProfilesPreserveOldDigestsAndRejectBorrowing(t *testing.T) {
	if problemvalidation.PinnedProfile().Digest() != "658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437" || problemvalidation.UnionFindProfile().Digest() != "a3f3b1fcea5d65e79003497ee98d713e4323131fb5446e08ea6482d9486c74c2" {
		t.Fatal("immutable old profile changed")
	}
	p, tier, err := problemvalidation.ResolveSource("library-checker:zalgorithm")
	if err != nil || tier != experiment.ReferenceMedium || p.Text.Bytes != 4<<20 || p.Diagnostics != 4096 {
		t.Fatal("medium source admission", err)
	}
	if _, _, err := problemvalidation.ResolveSource("library-checker:unknown"); err == nil {
		t.Fatal("unknown source accepted")
	}
	_, prepared := unionRunnerFixture(t, []byte("5\n"))
	s := &sandbox{}
	if _, err := problemvalidation.New(s).ValidateSource(context.Background(), prepared, "library-checker:zalgorithm"); err == nil || len(s.calls) != 0 {
		t.Fatal("source borrowed medium runtime")
	}
	if _, err := problemvalidation.New(s).ValidateUnionFind(context.Background(), prepared, p); err == nil || len(s.calls) != 0 {
		t.Fatal("old entry widened")
	}
}
