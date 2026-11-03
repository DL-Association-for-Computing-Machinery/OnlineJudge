package problemvalidation_test

import (
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestSourceProfilesPreserveOldDigestsAndRejectBorrowing(t *testing.T) {
	if problemvalidation.PinnedProfile().Digest() != "658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437" || problemvalidation.UnionFindProfile().Digest() != "a3f3b1fcea5d65e79003497ee98d713e4323131fb5446e08ea6482d9486c74c2" {
		t.Fatal("immutable old profile changed")
	}
	p, tier, err := problemvalidation.ResolveSource("library-checker:zalgorithm")
	if err != nil || tier != experiment.ReferenceMedium || p.Text.Bytes != 4<<20 || p.Diagnostics != 4096 {
		t.Fatal("medium source admission", err)
	}
	if p.Digest() != "32aa64eb913108ccc089a0548a98cb777f019f37cd4bc349ab87ff0ddd1a515d" {
		t.Fatal("zalgorithm profile changed")
	}
	suffix, suffixTier, err := problemvalidation.ResolveSource("library-checker:suffixarray")
	if err != nil || suffixTier != tier || suffix.Materials.TotalBytes != 128<<20 || p.Materials.TotalBytes != 64<<20 || suffix.Digest() == p.Digest() {
		t.Fatal("source-specific snapshot admission", err)
	}
	if suffix.Digest() != "0082bb2f961c1f1c082c0ed671881bca49cdaae72d9587bb76d8424ff5e661e7" {
		t.Fatal("suffixarray profile changed")
	}
	point, pointTier, err := problemvalidation.ResolveSource("library-checker:point_add_range_sum")
	if err != nil || pointTier != experiment.ReferenceMediumBulk || point.Materials.TotalBytes != 256<<20 || point.Materials.InputBytes != 16<<20 || point.Files.PayloadBytes != 512<<20 {
		t.Fatal("bulk source admission", err)
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
	_, zalg := referenceRunnerFixture(t, problemfixture.NewZAlgorithm(t), []byte("5\n"))
	if _, err := problemvalidation.New(s).ValidateSource(context.Background(), zalg, "library-checker:suffixarray"); err == nil || len(s.calls) != 0 {
		t.Fatal("zalgorithm borrowed larger snapshot profile")
	}
	_, suffixPrepared := referenceRunnerFixture(t, problemfixture.NewSuffixArray(t), []byte("5\n"))
	if _, err := problemvalidation.New(s).ValidateSource(context.Background(), suffixPrepared, "library-checker:point_add_range_sum"); err == nil || len(s.calls) != 0 {
		t.Fatal("suffixarray borrowed bulk material profile")
	}
}
