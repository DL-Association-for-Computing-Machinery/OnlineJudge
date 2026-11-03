package problemvalidation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
)

func TestSuccessfulReferenceReportJSONCompatibility(t *testing.T) {
	r := Report{Schema: "oj.reference-validation.v1", Status: "reference-validation-passed", Publication: "blocked", Profile: PinnedProfile(), ProfileSHA256: PinnedProfile().Digest(), Verdict: "AC", Points: []Point{{ID: "case-0001", Verdict: "AC", Usage: &execution.Usage{CPU: 1, Memory: 2}}}, CompileUsage: &execution.Usage{CPU: 3, Memory: 4}, StopConfirmed: true, Slot: "synthetic-slot", ContainerID: "synthetic-container", StartedAt: "synthetic-lifetime"}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(raw))
	if got != "8bf9e56626c84d19e31a7050e36330866f798ab8c59b56866032ef7cfea4baa6" {
		t.Fatal("successful reference report JSON changed", got)
	}
}
