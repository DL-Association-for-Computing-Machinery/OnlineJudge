package problemvalidation_test

import (
	"context"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestAssociativeArraySourceAdmissionCannotBeBorrowed(t *testing.T) {
	p, tier, err := problemvalidation.ResolveSource("library-checker:associative_array")
	if err != nil || tier != experiment.ReferenceAssociativeArray || p.Materials.InputBytes != 48<<20 || p.Materials.AnswerBytes != 8<<20 || p.Materials.TotalBytes != 512<<20 || p.Text.Bytes != 8<<20 || p.Text.TokenBytes != 4096 || p.Diagnostics != 4096 || p.Files.PayloadBytes != 1<<30 || p.Wall != 3*time.Minute {
		t.Fatal("inconsistent fixed source budgets", err)
	}
	_, current := referenceRunnerFixture(t, problemfixture.NewAssociativeArray(t), []byte("5\n"))
	for _, source := range []string{"library-checker:unionfind", "library-checker:zalgorithm", "library-checker:suffixarray", "library-checker:primality_test", "library-checker:static_range_sum", "library-checker:staticrmq", "library-checker:point_add_range_sum", "library-checker:convolution_mod"} {
		s := &sandbox{}
		if _, err := problemvalidation.New(s).ValidateSource(context.Background(), current, source); err == nil || len(s.calls) != 0 {
			t.Fatal("source borrowed another admission", source)
		}
	}
	_, old := referenceRunnerFixture(t, problemfixture.NewStaticRangeSum(t), []byte("5\n"))
	s := &sandbox{}
	if _, err := problemvalidation.New(s).ValidateSource(context.Background(), old, "library-checker:associative_array"); err == nil || len(s.calls) != 0 {
		t.Fatal("old source borrowed larger input or longer run budget")
	}
}
