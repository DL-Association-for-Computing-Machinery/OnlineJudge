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

func TestStatementRenderingMatchesHistoricalBrowserReport(t *testing.T) {
	path := os.Getenv("OJ_V6_STATEMENT_BROWSER_REPORT")
	if path == "" {
		t.Skip("requires the verified #151 browser report")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != statementRenderingReport {
		t.Fatal("not the reviewed historical browser report")
	}
	var report struct {
		Success   bool     `json:"success"`
		Viewports []string `json:"viewports"`
		Problems  []struct {
			SourceID     string `json:"source_id"`
			Version      string `json:"version"`
			Examples     int    `json:"examples"`
			Formulas     int    `json:"formulas"`
			FormatBlocks int    `json:"format_blocks"`
		} `json:"problems"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if !report.Success || !slices.Equal(report.Viewports, []string{"1280x960", "390x844"}) ||
		len(report.Problems) != len(reviewedV6Bindings) || len(report.Problems) != len(reviewedStatementRendering) {
		t.Fatal("incomplete browser evidence")
	}
	seen := map[string]bool{}
	var examples, formulas, formatBlocks int
	for _, row := range report.Problems {
		if seen[row.SourceID] {
			t.Fatal("duplicate report entry", row.SourceID)
		}
		seen[row.SourceID] = true
		binding := reviewedV6Bindings[row.SourceID]
		identity := problem.ImportIdentity{SourceID: row.SourceID, Version: row.Version,
			ImportRevision: binding.revision, SourceLockSHA256: SourceLockSHA256}
		got, ok := AdmittedStatementRendering(identity, binding.plan)
		if !ok || got.Examples != row.Examples || got.Formulas != row.Formulas || got.FormatBlocks != row.FormatBlocks || got.HistoricalMatch != nil {
			t.Fatalf("%s: stored evidence does not match the historical observation: %+v", row.SourceID, got)
		}
		examples += row.Examples
		formulas += row.Formulas
		formatBlocks += row.FormatBlocks
	}
	if examples != 18 || formulas != 82 || formatBlocks != 9 {
		t.Fatalf("unexpected browser totals: examples=%d formulas=%d format blocks=%d", examples, formulas, formatBlocks)
	}
}

func TestRevisedRenderingEvidenceMatchesReviewedEquivalence(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		examples, formulas, formatBlocks int
	}{
		{"aplusb", 2, 6, 2},
		{"unionfind", 1, 10, 0},
		{"zalgorithm", 4, 8, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile("testdata/" + tc.name + "-render-equivalence-20261112.json")
			if err != nil {
				t.Fatal(err)
			}
			var report struct {
				Browser        string `json:"browser_report_sha256"`
				Observed       string `json:"historical_content_version"`
				Local          string `json:"local_content_version"`
				ImportRevision string `json:"local_import_revision"`
				Plan           string `json:"local_formal_plan_sha256"`
				Profile        string `json:"profile_sha256"`
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			id := problem.ImportIdentity{SourceID: "library-checker:" + tc.name, Version: report.Local,
				ImportRevision: report.ImportRevision, SourceLockSHA256: SourceLockSHA256}
			e, ok := AdmittedRevisedStatementRendering(id, report.Plan, report.Profile)
			digest := sha256.Sum256(raw)
			if !ok || e.ReportSHA256 != report.Browser || e.HistoricalMatch == nil ||
				e.HistoricalMatch.ReportSHA256 != hex.EncodeToString(digest[:]) ||
				e.HistoricalMatch.ObservedContentVersion != report.Observed || report.Observed == report.Local ||
				e.Examples != tc.examples || e.Formulas != tc.formulas || e.FormatBlocks != tc.formatBlocks {
				t.Fatal("historical rendering does not match the reviewed equivalence for the local revision")
			}
		})
	}
}
