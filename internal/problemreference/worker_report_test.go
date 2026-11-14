package problemreference

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestLocalPackageReportRequiresItsPlanProfileAndSlot(t *testing.T) {
	hash := strings.Repeat("a", 64)
	obj := materialstore.Object{SHA256: hash, Size: 1}
	plan := problem.ReferencePlan{SourceID: "library-checker:new_package", Version: hash, ImportRevision: hash, SourceLockSHA256: hash,
		Source: obj, Definition: obj, Hashes: obj, LocalText: true, Cases: []problem.ReferenceCase{{ID: "case-0001", Input: obj, Answer: obj}}}
	local := problemvalidation.LocalTextProfile()
	legacy := problemvalidation.PinnedProfile()
	binding := problem.ImportIdentity{SourceID: plan.SourceID, Version: hash, ImportRevision: hash, SourceLockSHA256: hash}
	slot, err := workerhost.ReferenceSlotForTier(experiment.ReferenceAPlusBUbuntu)
	if err != nil {
		t.Fatal(err)
	}
	e := WorkerEvidence{Schema: workerEvidenceSchema, Verdict: "AC", CompileUsage: &execution.Usage{},
		Points:        []problemvalidation.Point{{ID: "case-0001", Verdict: "AC", Usage: &execution.Usage{}}},
		StopConfirmed: true, Slot: slot.ContainerName(), ContainerID: hash, StartedAt: "2026-11-14T00:00:00Z"}
	for _, tc := range []struct {
		name    string
		profile problemvalidation.Profile
		marked  bool
		slot    string
		want    bool
	}{
		{"local", local, true, slot.ContainerName(), true},
		{"ordinary profile", legacy, true, slot.ContainerName(), false},
		{"unmarked plan", local, false, slot.ContainerName(), false},
		{"wrong slot", local, true, (workerhost.M0Slot{}).ContainerName(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := plan
			p.LocalText = tc.marked
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			profile, err := json.Marshal(tc.profile)
			if err != nil {
				t.Fatal(err)
			}
			spec := Specification{Identity: binding, Protocol: IdentityProtocol, ProfileJSON: profile, ProfileSHA256: tc.profile.Digest()}
			ev := e
			ev.ProfileSHA256, ev.Slot = spec.ProfileSHA256, tc.slot
			if ev.ValidFor(spec, ExpectedPlan{Encoding: raw, SHA256: p.Digest()}) != tc.want {
				t.Fatal("incorrect local package evidence admission")
			}
		})
	}
}

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
