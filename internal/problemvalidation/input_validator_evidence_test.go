package problemvalidation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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
	other := strings.Repeat("f", 64)
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
		})
	}
}

// This re-derives all ten bindings from the original immutable bundle, without
// compiling or running any imported program. Ordinary CI does not have the bundle.
func TestInputValidatorBindingsMatchV6Bundle(t *testing.T) {
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
		} `json:"problems"`
	}
	if err := json.Unmarshal(raw, &report); err != nil || report.Schema != "oj.source-validation.v6" ||
		len(report.Problems) != 10 || len(admittedInputValidatorBindings) != 10 {
		t.Fatal("unexpected source validation inventory", err)
	}
	lock, err := os.ReadFile("../../config/problem-sources/library-checker.json")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	total := 0
	seen := map[string]bool{}
	for _, row := range report.Problems {
		if seen[row.SourceID] {
			t.Fatal("duplicate validator report source", row.SourceID)
		}
		seen[row.SourceID] = true
		total += row.Cases
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
			ev, ok := AdmittedInputValidator(id, plan.Digest())
			if !ok || ev.Inputs != len(plan.Cases) || ev.Inputs != row.Cases || ev.Controls != len(row.Controls) {
				t.Fatalf("review binding does not match verified source material: %+v", ev)
			}
		})
	}
	if total != 254 {
		t.Fatalf("validator report contains %d inputs, want 254", total)
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
