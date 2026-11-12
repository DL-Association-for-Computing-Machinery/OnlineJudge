package problemvalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func TestUnionFindCoverageMatchesReviewedReport(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	digest := func(raw []byte) string {
		sum := sha256.Sum256(raw)
		return hex.EncodeToString(sum[:])
	}
	raw := read("unionfind-coverage-20261112.json")
	var report struct {
		Source        string           `json:"source_id"`
		Version       string           `json:"content_version"`
		Revision      string           `json:"import_revision"`
		Lock          string           `json:"source_lock_sha256"`
		Plan          string           `json:"formal_plan_sha256"`
		Profile       string           `json:"profile_sha256"`
		Points        int              `json:"points"`
		SamplePoints  int              `json:"sample_points"`
		Rules         map[string][]int `json:"rules"`
		Wrong         map[string]int   `json:"wrong_solutions"`
		NonSampleRows []struct {
			Position int    `json:"position"`
			Input    string `json:"input_sha256"`
			Answer   string `json:"answer_sha256"`
			Wrong    map[string]struct {
				Exit *int   `json:"checker_exit"`
				Hash string `json:"output_sha256"`
			} `json:"wrong"`
		} `json:"non_sample_results"`
		Slow struct {
			Candidate     string `json:"candidate"`
			Hash          string `json:"candidate_sha256"`
			Input         string `json:"input_sha256"`
			Answer        string `json:"answer_sha256"`
			Position      int    `json:"position"`
			SoftLimit     int    `json:"cpu_soft_seconds"`
			HardLimit     int    `json:"cpu_hard_seconds"`
			WallLimit     int    `json:"wall_limit_seconds"`
			ControlPoints []int  `json:"small_control_positions"`
			CandidateRun  struct {
				Exit *int `json:"exit_code"`
				CPU  int  `json:"cpu_ms"`
			} `json:"candidate_run"`
			Reference struct {
				Exit *int `json:"exit_code"`
				CPU  int  `json:"cpu_ms"`
			} `json:"reference"`
		} `json:"slow"`
	}
	var quality struct {
		Plan   string `json:"formal_plan_sha256"`
		Inputs []struct {
			Sample bool   `json:"sample"`
			Input  string `json:"input_sha256"`
			Answer string `json:"answer_sha256"`
		} `json:"formal_inputs"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(read("unionfind-quality-20261112.json"), &quality); err != nil {
		t.Fatal(err)
	}
	id := problem.ImportIdentity{SourceID: report.Source, Version: report.Version, ImportRevision: report.Revision, SourceLockSHA256: report.Lock}
	got, ok := AdmittedTestCoverage(id, report.Plan, report.Profile)
	if !ok || got.ReportSHA256 != digest(raw) || report.Plan != quality.Plan ||
		got.Points != report.Points || report.Points != len(quality.Inputs) ||
		got.SamplePoints != report.SamplePoints || len(report.NonSampleRows) != report.Points-report.SamplePoints {
		t.Fatalf("coverage does not describe the reviewed plan: %+v", got)
	}
	rows, counts := make(map[int]bool), make(map[string]int)
	for _, row := range report.NonSampleRows {
		if row.Position < 1 || row.Position > len(quality.Inputs) || rows[row.Position] {
			t.Fatalf("invalid or repeated non-sample point %d", row.Position)
		}
		point := quality.Inputs[row.Position-1]
		if point.Sample || point.Input != row.Input || point.Answer != row.Answer || len(row.Wrong) != len(got.WrongSolutions) {
			t.Fatalf("coverage row does not match frozen point %d", row.Position)
		}
		rows[row.Position] = true
		for _, candidate := range got.WrongSolutions {
			result := row.Wrong[candidate.Candidate]
			if result.Exit == nil || (*result.Exit != 0 && *result.Exit != 1) ||
				(point.Answer == digest(nil) && (result.Hash != point.Answer || *result.Exit != 0)) {
				t.Fatalf("invalid wrong-candidate result for point %d", row.Position)
			}
			counts[candidate.Candidate] += *result.Exit
		}
	}
	for rule, positions := range report.Rules {
		found := false
		for _, position := range positions {
			found = found || rows[position]
		}
		if !found {
			t.Fatalf("rule %s has no non-sample witness", rule)
		}
	}
	if got.NonSampleCriteria != len(report.Rules) || got.SampleOnlyCriteria != 0 || len(report.Wrong) != len(counts) {
		t.Fatal("admitted coverage counts differ from the witnesses")
	}
	for _, candidate := range got.WrongSolutions {
		if count := counts[candidate.Candidate]; count == 0 || count != candidate.RejectedPoints || count != report.Wrong[candidate.Candidate] {
			t.Fatalf("wrong-solution counts differ for %s", candidate.Candidate)
		}
	}
	cpu, probe := report.Slow, got.LocalCPUProbe
	if probe == nil || probe.Candidate != cpu.Candidate || cpu.Hash != digest(read("unionfind-relabel.cpp")) ||
		probe.SmallControlsAccepted != len(cpu.ControlPoints) || probe.NonSamplePoints != 1 || !rows[cpu.Position] ||
		cpu.Input != quality.Inputs[cpu.Position-1].Input || cpu.Answer != quality.Inputs[cpu.Position-1].Answer ||
		cpu.SoftLimit != 5 || cpu.HardLimit != 6 || cpu.WallLimit != 15 || probe.CPUSoftLimitMS != 1000*cpu.SoftLimit ||
		cpu.CandidateRun.Exit == nil || *cpu.CandidateRun.Exit != -24 || cpu.CandidateRun.CPU < 4900 || cpu.CandidateRun.CPU > 6000 ||
		cpu.Reference.Exit == nil || *cpu.Reference.Exit != 0 || cpu.Reference.CPU >= probe.CPUSoftLimitMS ||
		probe.CandidateOutcome != "cpu_limit_exceeded" || probe.ReferenceOutcome != "accepted" {
		t.Fatalf("local CPU summary differs from the reviewed SIGXCPU probe: %+v", probe)
	}
}
