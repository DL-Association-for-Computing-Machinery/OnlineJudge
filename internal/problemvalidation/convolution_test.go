package problemvalidation_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestConvolutionProfileAndOldNineGoldenDigests(t *testing.T) {
	goldens := map[string]string{
		"aplusb":              "658fef827daa872a726e6a728ad05c84a08103bd9857c71606fe76cff92c1437",
		"unionfind":           "a3f3b1fcea5d65e79003497ee98d713e4323131fb5446e08ea6482d9486c74c2",
		"zalgorithm":          "32aa64eb913108ccc089a0548a98cb777f019f37cd4bc349ab87ff0ddd1a515d",
		"suffixarray":         "0082bb2f961c1f1c082c0ed671881bca49cdaae72d9587bb76d8424ff5e661e7",
		"primality_test":      "68f8cab1263e8c12897c6f0fc3a66ce915ba5296ee3304c03b2a6b5bd3419276",
		"point_add_range_sum": "5ab143ba3de72e53f361724db7c65ceca51d770dd81dd94be7495e2c286264c1",
		"staticrmq":           "f6e3875942ca1b4b57a17124ef758cb9ed922e715a624336b6d19191e8e6d946",
		"static_range_sum":    "501331a58bd9dfe76059e79701c80b7e7ba4d8096f1c93252f38c20d199cb028",
		"associative_array":   "abbc0c1081a07c239da97277a7ff8265a64205f847ba26ff606109d0750da519",
	}
	for source, want := range goldens {
		p, _, err := problemvalidation.ResolveSource("library-checker:" + source)
		if err != nil || p.Digest() != want {
			t.Fatal("old source profile changed", source, err)
		}
	}
	p, tier, err := problemvalidation.ResolveSource("library-checker:convolution_mod")
	if err != nil || tier != experiment.ReferenceConvolution || p.ID != "p4-lc-convolution-reference-desktop-arm64-v1" || p.Materials.SourceBytes != 64<<10 || p.Materials.InputBytes != 16<<20 || p.Materials.AnswerBytes != 16<<20 || p.Materials.TotalBytes != 512<<20 || p.Materials.Cases != 64 || p.Text.Bytes != 16<<20 || p.Text.TokenBytes != 4096 || p.Diagnostics != 4096 || p.Files.PayloadBytes != 2<<30 || p.Wall != 3*time.Minute {
		t.Fatal("convolution profile bounds", err)
	}
	digest := p.Digest()
	if digest != "7311e207d680b3be9276a7a57734785551be167aafa8a39b59148c61f7c9544d" {
		t.Fatal("convolution profile digest changed")
	}
	for _, old := range goldens {
		if digest == old {
			t.Fatal("convolution borrowed old profile identity")
		}
	}
	p.Text.Bytes--
	if p.Digest() == digest {
		t.Fatal("profile digest omitted checker budget")
	}
	p.Runtime.SandboxCmd[0] = "changed"
	again, _, err := problemvalidation.ResolveSource("library-checker:convolution_mod")
	if err != nil || again.Digest() != digest {
		t.Fatal("profile shares mutable state", err)
	}
}

func TestConvolutionSourceBindingAndCompleteRunner(t *testing.T) {
	constructors := map[string]func(*testing.T) *problemfixture.Bundle{
		"aplusb": problemfixture.New, "unionfind": problemfixture.NewUnionFind,
		"zalgorithm": problemfixture.NewZAlgorithm, "suffixarray": problemfixture.NewSuffixArray,
		"primality_test": problemfixture.NewPrimality, "point_add_range_sum": problemfixture.NewPointAddRangeSum,
		"staticrmq": problemfixture.NewStaticRMQ, "static_range_sum": problemfixture.NewStaticRangeSum,
		"associative_array": problemfixture.NewAssociativeArray,
	}
	_, current := referenceRunnerFixture(t, problemfixture.NewConvolution(t), []byte("5\n"))
	for source, constructor := range constructors {
		t.Run(source, func(t *testing.T) {
			_, old := referenceRunnerFixture(t, constructor(t), []byte("5\n"))
			s := &sandbox{}
			if _, err := problemvalidation.New(s).ValidateSource(context.Background(), old, "library-checker:convolution_mod"); err == nil || len(s.calls) != 0 {
				t.Fatal("old source borrowed convolution limits", err)
			}
			if _, err := problemvalidation.New(s).ValidateSource(context.Background(), current, "library-checker:"+source); err == nil || len(s.calls) != 0 {
				t.Fatal("convolution borrowed old source", err)
			}
		})
	}
	p, _, err := problemvalidation.ResolveSource("library-checker:convolution_mod")
	if err != nil {
		t.Fatal(err)
	}
	s := &sandbox{runtime: p.Runtime, compile: accepted(""), run: []execution.Observation{accepted("3"), accepted("5")}}
	s.compile.Artifact = "compiled"
	r, err := problemvalidation.New(s).ValidateSource(context.Background(), current, "library-checker:convolution_mod")
	if err != nil || r.Verdict != "AC" || len(r.Points) != 2 || r.ProfileSHA256 != p.Digest() || !reflect.DeepEqual(s.calls, []string{"verify", "compile", "run:1 2\n", "run:2 3\n", "remove"}) {
		t.Fatal("complete source runner or ordered execution changed", err, s.calls)
	}
	if err := r.Finalize(nil, nil, nil); err != nil || r.Status != "reference-validation-passed" || r.Publication != "blocked" {
		t.Fatal("convolution publication/stop gate", err)
	}
}

func TestConvolutionBadLastPointPrecedesVerify(t *testing.T) {
	for _, mode := range []string{"input-hash", "answer-utf8", "answer-token"} {
		t.Run(mode, func(t *testing.T) {
			answer := []byte("5\n")
			if mode == "answer-utf8" {
				answer = []byte{255, '\n'}
			}
			if mode == "answer-token" {
				answer = bytes.Repeat([]byte{'9'}, 4097)
			}
			b, p := referenceRunnerFixture(t, problemfixture.NewConvolution(t), answer)
			if mode == "input-hash" {
				ref := b.Document["test_cases"].([]any)[1].(map[string]any)["input"].(map[string]any)
				raw := append([]byte(nil), b.Files[ref["path"].(string)]...)
				raw[0] ^= 255
				if err := os.WriteFile(filepath.Join(b.Directory, ref["path"].(string)), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			s := &sandbox{}
			if _, err := problemvalidation.New(s).ValidateSource(context.Background(), p, "library-checker:convolution_mod"); err == nil || len(s.calls) != 0 {
				t.Fatal("invalid last material reached Verify", err)
			}
		})
	}
}
