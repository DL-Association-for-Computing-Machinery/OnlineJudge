package problemvalidation

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func TestSPJReportCannotBeReusedAcrossPlanOrRuntime(t *testing.T) {
	hash := handoffDigest([]byte("fixture"))
	material := materialstore.Object{SHA256: hash, Size: 7}
	plan := problem.ReferencePlan{Version: "old", Cases: []problem.ReferenceCase{{ID: "case-0001", Input: material, Answer: material}}}
	report := spjReport{Schema: materials.LocalSPJChecker, Source: hash, Artifact: hash,
		Image: experiment.PythonImageID, Header: experiment.SPJHeaderSHA256,
		Plan: plan.Digest(), Runtime: experiment.LocalSPJRuntimeSHA256(),
		Points: []spjPoint{{Input: hash, Answer: hash, Canonical: 50, Alternate: 50, Invalid: 51}}}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifySPJReport(raw, plan, hash); err != nil {
		t.Fatal(err)
	}
	changed := plan
	changed.Version = "new"
	if _, err := verifySPJReport(raw, changed, hash); !errors.Is(err, ErrCheckerTrialEvidence) {
		t.Fatalf("old report authorized new plan: %v", err)
	}
	report.Runtime = hash
	raw, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifySPJReport(raw, plan, hash); !errors.Is(err, ErrCheckerTrialEvidence) {
		t.Fatalf("old runtime report accepted: %v", err)
	}
}
