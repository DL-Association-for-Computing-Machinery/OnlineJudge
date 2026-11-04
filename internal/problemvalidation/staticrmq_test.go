package problemvalidation_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestStaticRMQProfilePreservesOldProfilesAndSourceBinding(t *testing.T) {
	p, tier, err := problemvalidation.ResolveSource("library-checker:staticrmq")
	if err != nil || tier != experiment.ReferenceStaticRMQ || p.Materials.InputBytes != 16<<20 || p.Materials.TotalBytes != 256<<20 || p.Files.PayloadBytes != 512<<20 || p.Text.Bytes != 4<<20 {
		t.Fatal("source admission", err)
	}
	primality, _, err := problemvalidation.ResolveSource("library-checker:primality_test")
	if err != nil || primality.Digest() != "68f8cab1263e8c12897c6f0fc3a66ce915ba5296ee3304c03b2a6b5bd3419276" {
		t.Fatal("fifth source profile changed", err)
	}
	_, old := unionRunnerFixture(t, []byte("5\n"))
	s := &sandbox{}
	if _, err := problemvalidation.New(s).ValidateSource(context.Background(), old, "library-checker:staticrmq"); err == nil || len(s.calls) != 0 {
		t.Fatal("old source borrowed staticrmq profile")
	}
	_, current := referenceRunnerFixture(t, problemfixture.NewStaticRMQ(t), []byte("5\n"))
	for _, source := range []string{"library-checker:suffixarray", "library-checker:point_add_range_sum"} {
		if _, err := problemvalidation.New(s).ValidateSource(context.Background(), current, source); err == nil || len(s.calls) != 0 {
			t.Fatal("staticrmq borrowed another source", source)
		}
	}
	_, bulk := referenceRunnerFixture(t, problemfixture.NewPointAddRangeSum(t), []byte("5\n"))
	if _, err := problemvalidation.New(s).ValidateSource(context.Background(), bulk, "library-checker:staticrmq"); err == nil || len(s.calls) != 0 {
		t.Fatal("bulk source borrowed staticrmq admission")
	}
}

func TestStaticRMQLastMaterialAndCheckerFailuresPrecedeVerify(t *testing.T) {
	for _, mode := range []string{"bad-input-hash", "long-token"} {
		t.Run(mode, func(t *testing.T) {
			answer := []byte("5\n")
			if mode == "long-token" {
				answer = bytes.Repeat([]byte{'9'}, 4097)
			}
			b, prepared := referenceRunnerFixture(t, problemfixture.NewStaticRMQ(t), answer)
			if mode == "bad-input-hash" {
				last := b.Document["test_cases"].([]any)[1].(map[string]any)["input"].(map[string]any)
				name := last["path"].(string)
				raw := append([]byte(nil), b.Files[name]...)
				raw[0] ^= 255
				if err := os.WriteFile(filepath.Join(b.Directory, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			s := &sandbox{}
			if _, err := problemvalidation.New(s).ValidateSource(context.Background(), prepared, "library-checker:staticrmq"); err == nil || len(s.calls) != 0 {
				t.Fatal("bad final material reached executor")
			}
		})
	}
}
