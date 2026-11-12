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

func TestZAlgorithmCoverageMatchesFrozenEvidence(t *testing.T) {
	read := func(name string, into any) []byte {
		t.Helper()
		raw, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	type binding struct {
		Source   string `json:"source_id"`
		Lock     string `json:"source_lock_sha256"`
		Version  string `json:"content_version"`
		Revision string `json:"import_revision"`
		Plan     string `json:"formal_plan_sha256"`
		Profile  string `json:"profile_sha256"`
		Index    string `json:"bundle_index_sha256"`
	}
	type point struct {
		Sample bool   `json:"sample"`
		Input  string `json:"input_sha256"`
		Answer string `json:"answer_sha256"`
	}
	var quality struct {
		binding
		Rows []struct {
			point
			ID string `json:"id"`
		} `json:"formal_inputs"`
		Independent []json.RawMessage `json:"independent_cases"`
	}
	var report struct {
		binding
		Rows []struct {
			point
			ID       string   `json:"case"`
			Criteria []string `json:"criteria"`
		} `json:"formal_points"`
		Witnesses  map[string]string `json:"non_sample_witnesses"`
		Rejections map[string][]int  `json:"checker_rejections"`
		CPU        struct {
			point
			ID           string `json:"case"`
			Candidate    string `json:"candidate"`
			Controls     int    `json:"small_controls"`
			Limit        int    `json:"cpu_soft_limit_ms"`
			CandidateCPU int    `json:"candidate_cpu_ms"`
			Signal       string `json:"candidate_signal"`
			ReferenceCPU int    `json:"reference_cpu_ms"`
			CheckerExit  *int   `json:"reference_checker_exit"`
		} `json:"cpu_probe"`
	}
	read("zalgorithm-quality-20261112.json", &quality)
	raw := read("zalgorithm-coverage-20261112.json", &report)
	if report.binding != quality.binding || len(report.Rows) != len(quality.Rows) {
		t.Fatal("coverage is not bound to the reviewed quality report")
	}
	id := problem.ImportIdentity{SourceID: report.Source, SourceLockSHA256: report.Lock, Version: report.Version, ImportRevision: report.Revision}
	got, ok := AdmittedTestCoverage(id, report.Plan, report.Profile)
	digest := sha256.Sum256(raw)
	if !ok || got.ReportSHA256 != hex.EncodeToString(digest[:]) || got.Points != len(report.Rows) ||
		got.NonSampleCriteria != len(report.Witnesses) || got.SampleOnlyCriteria != 0 {
		t.Fatalf("coverage summary does not match the reviewed report: %+v", got)
	}
	samples, witnesses := 0, make(map[string]bool)
	cpuMatches := false
	for i, row := range report.Rows {
		if prior := quality.Rows[i]; prior.ID != row.ID || prior.point != row.point {
			t.Fatalf("coverage row %d differs from the frozen point", i+1)
		}
		if row.Sample {
			samples++
			continue
		}
		for rule, point := range report.Witnesses {
			if row.ID == point && slices.Contains(row.Criteria, rule) {
				witnesses[rule] = true
			}
		}
		if row.ID == report.CPU.ID {
			cpuMatches = row.Input == report.CPU.Input && row.Answer == report.CPU.Answer
		}
	}
	if samples != got.SamplePoints || len(witnesses) != len(report.Witnesses) || len(report.Rejections) != len(got.WrongSolutions) {
		t.Fatal("unexpected point inventory or missing non-sample witnesses")
	}
	for _, candidate := range got.WrongSolutions {
		positions := report.Rejections[candidate.Candidate]
		if len(positions) == 0 || len(positions) != candidate.RejectedPoints {
			t.Fatalf("unexpected rejection count for %s", candidate.Candidate)
		}
		last := 0
		for _, position := range positions {
			if position <= last || position > len(report.Rows) || report.Rows[position-1].Sample {
				t.Fatalf("invalid non-sample rejection position %d", position)
			}
			last = position
		}
	}
	cpu := got.LocalCPUProbe
	if !cpuMatches || cpu == nil || cpu.Candidate != report.CPU.Candidate || cpu.SmallControlsAccepted != report.CPU.Controls ||
		cpu.SmallControlsAccepted != len(quality.Independent) || cpu.NonSamplePoints != 1 || cpu.CPUSoftLimitMS != report.CPU.Limit ||
		cpu.CandidateOutcome != "cpu_limit_exceeded" || cpu.ReferenceOutcome != "accepted" ||
		report.CPU.Signal != "SIGXCPU" || report.CPU.CandidateCPU < report.CPU.Limit-100 ||
		report.CPU.CandidateCPU > report.CPU.Limit+1000 || report.CPU.ReferenceCPU < 0 || report.CPU.ReferenceCPU >= report.CPU.Limit ||
		report.CPU.CheckerExit == nil || *report.CPU.CheckerExit != 0 {
		t.Fatalf("unexpected bounded local CPU observation: %+v", cpu)
	}
}
