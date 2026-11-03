package problemvalidation_test

import (
	"context"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestMediumSourceRunnerAllAnswersPrecedeExecution(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		answer := []byte("5\n")
		if invalid {
			answer = []byte{255, '\n'}
		}
		_, p := referenceRunnerFixture(t, problemfixture.NewZAlgorithm(t), answer)
		profile, _, err := problemvalidation.ResolveSource("library-checker:zalgorithm")
		if err != nil {
			t.Fatal(err)
		}
		s := &sandbox{runtime: profile.Runtime, compile: accepted(""), run: []execution.Observation{accepted("3"), accepted("5")}}
		s.compile.Artifact = "compiled"
		r, err := problemvalidation.New(s).ValidateSource(context.Background(), p, "library-checker:zalgorithm")
		if invalid {
			if err == nil || len(s.calls) != 0 {
				t.Fatal("bad later answer reached medium executor")
			}
			continue
		}
		if err != nil || r.Verdict != "AC" || len(r.Points) != 2 || len(s.calls) != 5 || r.ProfileSHA256 != profile.Digest() {
			t.Fatal("complete medium source failed", err)
		}
		if err := r.Finalize(nil, nil, nil); err != nil || r.Status != "reference-validation-passed" || r.Publication != "blocked" {
			t.Fatal("medium publication/stop gate")
		}
	}
}
