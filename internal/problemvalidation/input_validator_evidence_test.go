package problemvalidation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func TestAdmittedInputValidatorRequiresExactBinding(t *testing.T) {
	const source = "library-checker:aplusb"
	const plan = "4632b8970118f8a75ac39857d31d2afb8244683892b9edc49357f9f5aefeba2a"
	want := problem.ImportIdentity{SourceID: source, SourceLockSHA256: SourceLockSHA256,
		Version:        "87ebb9680aa3abe190b61d4a5ccac1dc83e48b29656364e445e36fabe7467cc9",
		ImportRevision: "d19c436bc1631cb2e42b38f0036de45c100ea2f76c953df502b960287c248dd5"}
	ev, ok := AdmittedInputValidator(want, plan)
	if !ok || ev.Inputs != 12 || ev.Controls != 4 || !problem.ValidImportRevision(ev.ReportSHA256) {
		t.Fatalf("known complete v6 binding was not admitted: %+v", ev)
	}
	independent, ok := AdmittedIndependentCorrectness(want, plan)
	if !ok || independent.Cases != 25 || independent.Suite != "lc-small-oracles.v1" || independent.ReportSHA256 != ev.ReportSHA256 {
		t.Fatalf("known complete v6 binding was not admitted for independent correctness: %+v", independent)
	}
	wrong, ok := AdmittedWrongSolution(want, plan)
	if !ok || wrong.Origin != "upstream" || wrong.Candidate != "wa.cpp" ||
		wrong.RejectedCasePosition != 4 || wrong.AcceptedCasePositions == nil ||
		len(wrong.AcceptedCasePositions) != 0 || wrong.ReportSHA256 != ev.ReportSHA256 {
		t.Fatalf("known complete v6 binding did not expose its wrong solution: %+v", wrong)
	}
	other := strings.Repeat("f", 64)
	rendering, ok := AdmittedStatementRendering(want, plan)
	if !ok || rendering.Examples != 2 || rendering.Formulas != 6 || rendering.FormatBlocks != 2 ||
		!problem.ValidImportRevision(rendering.ReportSHA256) || rendering.ReportSHA256 == ev.ReportSHA256 {
		t.Fatalf("known complete v6 binding did not expose its browser evidence: %+v", rendering)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*problem.ImportIdentity, *string)
	}{
		{"source", func(i *problem.ImportIdentity, _ *string) { i.SourceID = "library-checker:unionfind" }},
		{"source lock", func(i *problem.ImportIdentity, _ *string) { i.SourceLockSHA256 = other }},
		{"content version", func(i *problem.ImportIdentity, _ *string) { i.Version = other }},
		{"same content with a different complete revision", func(i *problem.ImportIdentity, _ *string) { i.ImportRevision = other }},
		{"formal plan", func(_ *problem.ImportIdentity, p *string) { *p = other }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, hash := want, plan
			tc.mutate(&got, &hash)
			if ev, ok := AdmittedInputValidator(got, hash); ok || ev != (InputValidatorEvidence{}) {
				t.Fatalf("different binding inherited trusted validator evidence: %+v", ev)
			}
			if ev, ok := AdmittedIndependentCorrectness(got, hash); ok || ev != (IndependentCorrectnessEvidence{}) {
				t.Fatalf("different binding inherited independent correctness evidence: %+v", ev)
			}
			if ev, ok := AdmittedWrongSolution(got, hash); ok || ev.ReportSHA256 != "" {
				t.Fatalf("different binding inherited wrong solution evidence: %+v", ev)
			}
			if ev, ok := AdmittedStatementRendering(got, hash); ok || ev != (StatementRenderingEvidence{}) {
				t.Fatalf("different binding inherited statement rendering evidence: %+v", ev)
			}
		})
	}
}

// This re-derives all ten bindings from the original immutable bundle, without
// compiling or running any imported program. Ordinary CI does not have the bundle.
func TestReviewEvidenceBindingsMatchV6Bundle(t *testing.T) {
	dir := os.Getenv("OJ_V6_INPUT_EVIDENCE_BUNDLE")
	if dir == "" {
		t.Skip("requires the verified Library Checker v6 private bundle")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "provenance", "source-validation.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != admittedInputValidatorReport {
		t.Fatal("bundle report is not the reviewed v6 artifact")
	}
	var report struct {
		Schema   string `json:"schema"`
		Problems []struct {
			SourceID  string `json:"source_id"`
			Cases     int    `json:"cases"`
			Validator string `json:"input_validator"`
			Controls  []struct {
				ID   string `json:"id"`
				Exit *int   `json:"exit_code"`
			} `json:"input_controls"`
			Independent struct {
				Suite     string `json:"suite"`
				Reference string `json:"reference_sha256"`
				Cases     []struct {
					ValidatorExit *int `json:"validator_exit"`
					ReferenceExit *int `json:"reference_exit"`
					CheckerExit   *int `json:"checker_exit"`
				} `json:"cases"`
			} `json:"independent"`
			UpstreamWrong *wrongSolutionProbe `json:"wrong_solution"`
			LocalWrong    *wrongSolutionProbe `json:"local_wrong_solution"`
		} `json:"problems"`
	}
	if err := json.Unmarshal(raw, &report); err != nil || report.Schema != "oj.source-validation.v6" ||
		len(report.Problems) != 10 || len(reviewedV6Bindings) != 10 || len(reviewedV6WrongSolutions) != 10 {
		t.Fatal("unexpected source validation inventory", err)
	}
	lock, err := os.ReadFile("../../config/problem-sources/library-checker.json")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	total, independentTotal := 0, 0
	seen := map[string]bool{}
	for _, row := range report.Problems {
		if seen[row.SourceID] {
			t.Fatal("duplicate validator report source", row.SourceID)
		}
		seen[row.SourceID] = true
		total += row.Cases
		independentTotal += len(row.Independent.Cases)
		t.Run(row.SourceID, func(t *testing.T) {
			if row.Validator != "passed-all" || len(row.Controls) != 4 {
				t.Fatal("missing validator verdict or controls")
			}
			for i, name := range []string{"valid-boundary", "outside-domain", "truncated", "trailing-data"} {
				wantExit := 3
				if i == 0 {
					wantExit = 0
				}
				if row.Controls[i].ID != name || row.Controls[i].Exit == nil || *row.Controls[i].Exit != wantExit {
					t.Fatal("validator controls do not match the recorded outcomes")
				}
			}
			id, plan := readInputValidatorPlan(t, ctx, dir, row.SourceID, lock)
			checkWrongSolutionReport(t, id, plan, row.UpstreamWrong, row.LocalWrong)
			ev, ok := AdmittedInputValidator(id, plan.Digest())
			if !ok || ev.Inputs != len(plan.Cases) || ev.Inputs != row.Cases || ev.Controls != len(row.Controls) {
				t.Fatalf("review binding does not match verified source material: %+v", ev)
			}
			if row.Independent.Suite != "lc-small-oracles.v1" || row.Independent.Reference != plan.Source.SHA256 {
				t.Fatal("independent oracle report does not bind the frozen reference solution")
			}
			for _, c := range row.Independent.Cases {
				for _, status := range []*int{c.ValidatorExit, c.ReferenceExit, c.CheckerExit} {
					if status == nil || *status != 0 {
						t.Fatal("missing or failed recorded oracle outcome")
					}
				}
			}
			independent, ok := AdmittedIndependentCorrectness(id, plan.Digest())
			if row.SourceID == ConvolutionSourceID {
				if ok || independent != (IndependentCorrectnessEvidence{}) || len(row.Independent.Cases) != 6 {
					t.Fatal("convolution source preparation replaced its station differential requirement")
				}
			} else if !ok || independent.Suite != row.Independent.Suite ||
				independent.ReportSHA256 != ev.ReportSHA256 || independent.Cases != len(row.Independent.Cases) {
				t.Fatalf("reviewed oracle evidence does not match the frozen report: %+v", independent)
			}
		})
	}
	if total != 254 {
		t.Fatalf("validator report contains %d inputs, want 254", total)
	}
	if independentTotal != 198 {
		t.Fatalf("report contains %d oracle cases, want 198", independentTotal)
	}
}

type wrongSolutionPoint struct {
	Position      int    `json:"case_position"`
	Input         string `json:"input_sha256"`
	Answer        string `json:"answer_sha256"`
	CandidateExit *int   `json:"candidate_exit"`
	CheckerExit   *int   `json:"checker_exit"`
}

type wrongSolutionProbe struct {
	wrongSolutionPoint
	Solution  string               `json:"solution"`
	Candidate string               `json:"candidate"`
	Suite     string               `json:"suite"`
	Accepted  []wrongSolutionPoint `json:"preceding_ac"`
}

func checkWrongSolutionReport(t *testing.T, id problem.ImportIdentity, plan problem.ReferencePlan, upstream, local *wrongSolutionProbe) {
	t.Helper()
	var probe *wrongSolutionProbe
	var origin, candidate string
	switch {
	case upstream != nil && local == nil:
		probe, origin, candidate = upstream, "upstream", upstream.Solution
	case upstream == nil && local != nil && local.Suite == "lc-formal-wrong.v1":
		probe, origin, candidate = local, "local", local.Candidate
	default:
		t.Fatal("expected exactly one recorded wrong solution")
	}
	verifyPoint := func(point wrongSolutionPoint, checkerExit int) {
		t.Helper()
		if point.Position < 1 || point.Position > len(plan.Cases) {
			t.Fatal("wrong solution position outside the frozen plan")
		}
		c := plan.Cases[point.Position-1]
		if c.Input.SHA256 != point.Input || c.Answer.SHA256 != point.Answer ||
			point.CandidateExit == nil || *point.CandidateExit != 0 || point.CheckerExit == nil || *point.CheckerExit != checkerExit {
			t.Fatal("wrong solution result does not bind the point and observed exits")
		}
	}
	verifyPoint(probe.wrongSolutionPoint, 1)
	var accepted []int
	for _, point := range probe.Accepted {
		verifyPoint(point, 0)
		accepted = append(accepted, point.Position)
	}
	hash := plan.Digest()
	got, ok := AdmittedWrongSolution(id, hash)
	if !ok || got.ReportSHA256 != admittedInputValidatorReport || got.Origin != origin ||
		got.Candidate != candidate || got.RejectedCasePosition != probe.Position || !slices.Equal(got.AcceptedCasePositions, accepted) {
		t.Fatalf("wrong solution projection does not match reviewed report: %+v", got)
	}
	if len(accepted) > 0 {
		got.AcceptedCasePositions[0] = -1
		again, ok := AdmittedWrongSolution(id, hash)
		if !ok || !slices.Equal(again.AcceptedCasePositions, accepted) {
			t.Fatal("caller mutated the reviewed positions")
		}
	}
}

func readInputValidatorPlan(t *testing.T, ctx context.Context, dir, source string, lock []byte) (problem.ImportIdentity, problem.ReferencePlan) {
	t.Helper()
	p, err := problem.LoadBundle(dir, source, lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	id, err := p.ImportIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if source == "library-checker:aplusb" {
		plan, err := p.PrepareReference(ctx, problem.SmallReferenceLimits())
		if err != nil {
			t.Fatal(err)
		}
		return id, plan
	}
	snapshot, err := p.PrepareDiskReference(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := snapshot.Close(); err != nil {
			t.Error(err)
		}
	})
	return id, snapshot.Plan()
}
