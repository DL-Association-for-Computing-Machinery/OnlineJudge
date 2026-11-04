package problemvalidation_test

import (
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestStaticRangeSumSourceBindingRejectsBorrowedMaterial(t *testing.T) {
	profile, tier, err := problemvalidation.ResolveSource("library-checker:static_range_sum")
	if err != nil || tier != experiment.ReferenceStaticRangeSum || profile.Runtime.OutputBytes != 8<<20 || profile.Text.Bytes != 8<<20 || profile.Text.TokenBytes != 4096 || profile.Diagnostics != 4096 || profile.Files.PayloadBytes != 512<<20 || profile.Materials.InputBytes != 16<<20 || profile.Materials.AnswerBytes != 8<<20 || profile.Materials.TotalBytes != 256<<20 {
		t.Fatal("inconsistent fixed source admission", err)
	}
	_, old := unionRunnerFixture(t, []byte("5\n"))
	s := &sandbox{}
	if _, err := problemvalidation.New(s).ValidateSource(context.Background(), old, "library-checker:static_range_sum"); err == nil || len(s.calls) != 0 {
		t.Fatal("old source borrowed larger output/material limits")
	}
	_, current := referenceRunnerFixture(t, problemfixture.NewStaticRangeSum(t), []byte("5\n"))
	for _, source := range []string{"library-checker:unionfind", "library-checker:zalgorithm", "library-checker:suffixarray", "library-checker:primality_test", "library-checker:staticrmq", "library-checker:point_add_range_sum", "library-checker:associative_array", "library-checker:convolution_mod"} {
		if _, err := problemvalidation.New(s).ValidateSource(context.Background(), current, source); err == nil || len(s.calls) != 0 {
			t.Fatal("new source borrowed a different admission", source)
		}
	}
	for _, source := range []string{"library-checker:unknown"} {
		if _, _, err := problemvalidation.ResolveSource(source); err == nil {
			t.Fatal("unadmitted source accepted by new tier", source)
		}
	}
}
