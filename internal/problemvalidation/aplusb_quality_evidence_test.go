package problemvalidation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func TestApPlusBQualityMatchesFrozenPlanAndReviewedReport(t *testing.T) {
	bundle, path := os.Getenv("OJ_APLUSB_QUALITY_BUNDLE"), os.Getenv("OJ_APLUSB_QUALITY_REPORT")
	if bundle == "" && path == "" {
		t.Skip("requires the reviewed A+B bundle and report")
	}
	read := func(path string) []byte {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	lock := read("../../config/problem-sources/library-checker.json")
	raw := read(path)
	var report struct {
		SourceID    string `json:"source_id"`
		Version     string `json:"content_version"`
		Revision    string `json:"import_revision"`
		Lock        string `json:"source_lock_sha256"`
		Index       string `json:"bundle_index_sha256"`
		Plan        string `json:"formal_plan_sha256"`
		Profile     string `json:"profile_sha256"`
		Reference   string `json:"reference_sha256"`
		Validator   string `json:"validator_sha256"`
		OracleSuite string `json:"independent_suite"`
		Inputs      []struct {
			Case  string `json:"case"`
			Input string `json:"input_sha256"`
			Exit  *int   `json:"validator_exit"`
		} `json:"inputs"`
		Controls []struct {
			ID    string `json:"id"`
			Input string `json:"input_sha256"`
			Exit  *int   `json:"exit_code"`
		} `json:"controls"`
		Probes []struct {
			ID     string `json:"id"`
			Input  string `json:"input_sha256"`
			Answer string `json:"answer_sha256"`
			Actual string `json:"actual_sha256"`
		} `json:"probes"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	p, err := problem.LoadBundle(bundle, "library-checker:aplusb", lock)
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
	plan, err := p.PrepareReference(t.Context(), problem.SmallReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	profile, _, err := ResolveSource(id.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	digest := func(raw []byte) string { return fmt.Sprintf("%x", sha256.Sum256(raw)) }
	var lockFiles struct {
		Files map[string]struct {
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(lock, &lockFiles); err != nil {
		t.Fatal(err)
	}
	if report.SourceID != id.SourceID || report.Version != id.Version || report.Revision != id.ImportRevision ||
		report.Lock != id.SourceLockSHA256 || report.Index != digest(read(filepath.Join(bundle, "index.json"))) ||
		report.Plan != plan.Digest() || report.Profile != profile.Digest() || report.Reference != plan.Source.SHA256 ||
		report.Validator != lockFiles.Files["sample/aplusb/verifier.cpp"].SHA256 {
		t.Fatal("quality report does not match the fresh plan, profile and pinned programs")
	}
	input, small, ok := AdmittedApPlusBQuality(id, plan.Digest(), profile.Digest())
	if !ok || input.ReportSHA256 != digest(raw) || small.ReportSHA256 != input.ReportSHA256 ||
		input.Inputs != len(plan.Cases) || len(report.Inputs) != input.Inputs || len(report.Controls) != input.Controls ||
		len(report.Probes) != small.Cases || report.OracleSuite != small.Suite {
		t.Fatal("admitted summaries differ from the verified report")
	}
	for i, point := range report.Inputs {
		if point.Case != plan.Cases[i].ID || point.Input != plan.Cases[i].Input.SHA256 || point.Exit == nil || *point.Exit != 0 {
			t.Fatalf("validator did not accept frozen input %d", i+1)
		}
	}
	for i, control := range []struct {
		id, bytes string
		exit      int
	}{
		{"valid-boundary", "0 1000000000\n", 0},
		{"outside-domain", "0 1000000001\n", 3},
		{"truncated", "0 1000000000", 3},
		{"trailing-data", "0 1000000000\n0\n", 3},
	} {
		got := report.Controls[i]
		if got.ID != control.id || got.Input != digest([]byte(control.bytes)) || got.Exit == nil || *got.Exit != control.exit {
			t.Fatalf("wrong validator control: %s", control.id)
		}
	}
	values := [...]int{0, 1, 1 << 29, 999999999, 1000000000}
	for i, probe := range report.Probes {
		a, b := values[i/len(values)], values[i%len(values)]
		if probe.ID != fmt.Sprintf("small-%02d", i+1) || probe.Input != digest(fmt.Appendf(nil, "%d %d\n", a, b)) ||
			probe.Answer != digest(fmt.Appendf(nil, "%d\n", a+b)) || probe.Actual != probe.Answer {
			t.Fatalf("incorrect independent probe %d", i+1)
		}
	}
}
