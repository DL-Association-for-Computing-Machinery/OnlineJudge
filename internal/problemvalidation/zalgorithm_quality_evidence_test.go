package problemvalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func TestZAlgorithmQualityMatchesReviewedReport(t *testing.T) {
	raw, err := os.ReadFile("testdata/zalgorithm-quality-20261112.json")
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Source   string            `json:"source_id"`
		Version  string            `json:"content_version"`
		Revision string            `json:"import_revision"`
		Lock     string            `json:"source_lock_sha256"`
		Plan     string            `json:"formal_plan_sha256"`
		Profile  string            `json:"profile_sha256"`
		Suite    string            `json:"independent_suite"`
		Inputs   []json.RawMessage `json:"formal_inputs"`
		Controls []json.RawMessage `json:"controls"`
		Cases    []json.RawMessage `json:"independent_cases"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	id := problem.ImportIdentity{SourceID: report.Source, Version: report.Version, ImportRevision: report.Revision, SourceLockSHA256: report.Lock}
	profile, _, err := ResolveSource(id.SourceID)
	if err != nil || profile.Digest() != report.Profile || id.SourceLockSHA256 != SourceLockSHA256 {
		t.Fatalf("reviewed report does not match the current lock and profile: %v", err)
	}
	input, independent, ok := AdmittedZAlgorithmQuality(id, report.Plan, report.Profile)
	sum := sha256.Sum256(raw)
	if !ok || input.ReportSHA256 != hex.EncodeToString(sum[:]) || independent.ReportSHA256 != input.ReportSHA256 ||
		input.Inputs != len(report.Inputs) || input.Controls != len(report.Controls) ||
		independent.Cases != len(report.Cases) || independent.Suite != report.Suite {
		t.Fatalf("admitted summaries do not match the reviewed report: %+v, %+v", input, independent)
	}
}
