package problemreference

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestWorkerEvidenceValidatesDeterminateOrderedResults(t *testing.T) {
	digest := strings.Repeat("a", 64)
	obj := materialstore.Object{SHA256: digest, Size: 1}
	plan := problem.ReferencePlan{SourceID: "library-checker:aplusb", Version: digest, ImportRevision: digest, SourceLockSHA256: digest,
		Source: obj, Definition: obj, Hashes: obj, Cases: []problem.ReferenceCase{{ID: "case-0001", Input: obj, Answer: obj}, {ID: "case-0002", Input: obj, Answer: obj}}}
	encoding, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	profile := problemvalidation.PinnedProfile()
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	spec := Specification{Identity: problem.ImportIdentity{SourceID: plan.SourceID, Version: digest, ImportRevision: digest, SourceLockSHA256: digest}, Protocol: IdentityProtocol, ProfileJSON: profileJSON, ProfileSHA256: profile.Digest()}
	expected := ExpectedPlan{Encoding: encoding, SHA256: plan.Digest()}
	for _, tc := range []struct {
		kind string
		want bool
	}{{"AC", true}, {"WA", true}, {"CE", true}, {"SE", false}, {"missing stop", false}, {"wrong slot", false}, {"wrong order", false}, {"missing point", false}, {"missing usage", false}, {"negative usage", false}, {"over compile limit", false}, {"over run limit", false}, {"continued after WA", false}, {"omitted WA", false}} {
		t.Run(tc.kind, func(t *testing.T) {
			e := WorkerEvidence{Schema: workerEvidenceSchema, ProfileSHA256: spec.ProfileSHA256, Verdict: "AC", CompileUsage: &execution.Usage{CPU: 1, Memory: 2},
				Points:        []problemvalidation.Point{{ID: "case-0001", Verdict: "AC", Usage: &execution.Usage{CPU: 1}}, {ID: "case-0002", Verdict: "AC", Usage: &execution.Usage{CPU: 2}}},
				StopConfirmed: true, Slot: (workerhost.M0Slot{}).ContainerName(), ContainerID: digest, StartedAt: "2026-11-11T00:00:00Z"}
			switch tc.kind {
			case "WA":
				e.Verdict = "WA"
				e.Points[0].Verdict = "WA"
				e.Points[1].Verdict, e.Points[1].Usage = "NOT_RUN", nil
			case "CE":
				e.Verdict = "CE"
				for i := range e.Points {
					e.Points[i].Verdict, e.Points[i].Usage = "NOT_RUN", nil
				}
			case "SE":
				e.Verdict = "SE"
			case "missing stop":
				e.StopConfirmed = false
			case "wrong slot":
				e.Slot = (workerhost.ReferenceSlot{}).ContainerName()
			case "wrong order":
				e.Points[0], e.Points[1] = e.Points[1], e.Points[0]
			case "missing point":
				e.Points = e.Points[:1]
			case "missing usage":
				e.Points[0].Usage = nil
			case "negative usage":
				e.Points[0].Usage.CPU = -1
			case "over compile limit":
				e.CompileUsage.CPU = profile.Runtime.Compile.CPU + 1
			case "over run limit":
				e.Points[0].Usage.Memory = profile.Runtime.Run.Memory + 1
			case "continued after WA":
				e.Verdict = "WA"
				e.Points[0].Verdict = "WA"
			case "omitted WA":
				e.Verdict = "WA"
			}
			if e.ValidFor(spec, expected) != tc.want {
				t.Fatal("incorrect evidence admission", tc.kind)
			}
		})
	}
}
