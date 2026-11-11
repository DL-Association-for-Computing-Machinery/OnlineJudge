package problemvalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func TestApPlusBCoverageMatchesReviewedReport(t *testing.T) {
	path := os.Getenv("OJ_APLUSB_COVERAGE_REVIEW_REPORT")
	if path == "" {
		t.Skip("requires the reviewed A+B coverage report")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		SourceID       string `json:"source_id"`
		Version        string `json:"content_version"`
		ImportRevision string `json:"import_revision"`
		Lock           string `json:"source_lock_sha256"`
		Plan           string `json:"formal_plan_sha256"`
		Profile        string `json:"profile_sha256"`
		Positions      map[string]struct {
			Sample    []int `json:"sample"`
			NonSample []int `json:"non_sample"`
		} `json:"point_positions"`
		Rejections map[string]int `json:"wrong_solution_rejections"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	id := problem.ImportIdentity{SourceID: report.SourceID, Version: report.Version, ImportRevision: report.ImportRevision, SourceLockSHA256: report.Lock}
	ev, ok := AdmittedTestCoverage(id, report.Plan, report.Profile)
	sum := sha256.Sum256(raw)
	if !ok || ev.ReportSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("reviewed report digest or frozen binding differs from admission")
	}
	nonSample, sampleOnly := 0, 0
	for name, p := range report.Positions {
		switch {
		case len(p.NonSample) > 0:
			nonSample++
		case len(p.Sample) > 0:
			sampleOnly++
		default:
			t.Fatalf("coverage criterion has no witness: %s", name)
		}
		for _, pos := range p.Sample {
			if pos < 1 || pos > ev.SamplePoints {
				t.Fatal("sample witness outside the verified sample prefix")
			}
		}
		for _, pos := range p.NonSample {
			if pos <= ev.SamplePoints || pos > ev.Points {
				t.Fatal("non-sample witness outside the verified plan")
			}
		}
	}
	maximum := report.Positions["sum=2000000000"]
	if nonSample != ev.NonSampleCriteria || sampleOnly != ev.SampleOnlyCriteria ||
		!slices.Equal(maximum.Sample, []int{ev.MaximumSumSamplePosition}) || len(maximum.NonSample) != 0 {
		t.Fatal("criteria or maximum-sum witness differs from the reviewed report")
	}
	if len(report.Rejections) != len(ev.WrongSolutions) {
		t.Fatal("wrong solution set differs from the reviewed report")
	}
	for _, candidate := range ev.WrongSolutions {
		if candidate.RejectedPoints != report.Rejections[candidate.Candidate] ||
			candidate.RejectedPoints < 1 || candidate.RejectedPoints > ev.Points-ev.SamplePoints {
			t.Fatalf("invalid rejection summary for %s", candidate.Candidate)
		}
	}
}
