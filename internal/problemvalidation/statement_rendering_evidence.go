package problemvalidation

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"

const statementRenderingReport = "8054f52499d7f987eedd78c64a16877b0391555909b74bb735409a51355f42f3"

// StatementRenderingMatch records why an earlier browser observation also
// applies to a different content version with identical rendering inputs.
type StatementRenderingMatch struct {
	ObservedContentVersion string `json:"observed_content_version"`
	ReportSHA256           string `json:"report_sha256"`
}

// StatementRenderingEvidence summarizes the historical desktop and mobile
// browser checks on matching content. It does not attest to statement semantics.
type StatementRenderingEvidence struct {
	ReportSHA256    string                   `json:"report_sha256"`
	Examples        int                      `json:"examples"`
	Formulas        int                      `json:"formulas"`
	FormatBlocks    int                      `json:"format_blocks"`
	HistoricalMatch *StatementRenderingMatch `json:"historical_match,omitempty"`
}

type statementRenderingCounts struct {
	examples, formulas, formatBlocks int
}

// Counts come from the #151 Chromium report. Each source's observed content
// version matches reviewedV6Bindings; the browser did not import the v6 revision.
var reviewedStatementRendering = map[string]statementRenderingCounts{
	"library-checker:aplusb":              {examples: 2, formulas: 6, formatBlocks: 2},
	"library-checker:associative_array":   {examples: 1, formulas: 8, formatBlocks: 1},
	ConvolutionSourceID:                   {examples: 2, formulas: 8, formatBlocks: 2},
	"library-checker:point_add_range_sum": {examples: 1, formulas: 9},
	"library-checker:primality_test":      {examples: 1, formulas: 7, formatBlocks: 1},
	"library-checker:static_range_sum":    {examples: 1, formulas: 11, formatBlocks: 1},
	"library-checker:staticrmq":           {examples: 1, formulas: 7},
	"library-checker:suffixarray":         {examples: 4, formulas: 8, formatBlocks: 2},
	"library-checker:unionfind":           {examples: 1, formulas: 10},
	"library-checker:zalgorithm":          {examples: 4, formulas: 8},
}

// AdmittedStatementRendering projects the historical report for an exact
// reviewed v6 binding. The caller must first verify the stored identity and plan.
func AdmittedStatementRendering(identity problem.ImportIdentity, planSHA256 string) (StatementRenderingEvidence, bool) {
	if _, ok := admittedV6Binding(identity, planSHA256); !ok {
		return StatementRenderingEvidence{}, false
	}
	return historicalStatementRendering(identity.SourceID)
}

// AdmittedRevisedStatementRendering uses reviewed equivalence between
// historical browser inputs and a current preview, not a new browser run.
// The caller must first verify the stored plan and accepted report.
func AdmittedRevisedStatementRendering(id problem.ImportIdentity, planSHA256, profileSHA256 string) (StatementRenderingEvidence, bool) {
	var match StatementRenderingMatch
	switch {
	case matchesReviewedApPlusBPlan(id, planSHA256, profileSHA256):
		match = StatementRenderingMatch{
			ObservedContentVersion: "87ebb9680aa3abe190b61d4a5ccac1dc83e48b29656364e445e36fabe7467cc9",
			ReportSHA256:           "3c8ec439df6cd551a87ea40e405cd06990d0f9476ad2e3413c4ecbc894329644",
		}
	case matchesReviewedUnionFindPlan(id, planSHA256, profileSHA256):
		match = StatementRenderingMatch{
			ObservedContentVersion: "fd871a02594fcd663790cd0530153df2ad18bf95110559ce552a121c34705bed",
			ReportSHA256:           "3dd77d0fa9bc501ae23a3c352e0c83486b71d6b8d619de4451b27534db86d453",
		}
	case matchesReviewedZAlgorithmPlan(id, planSHA256, profileSHA256):
		match = StatementRenderingMatch{
			ObservedContentVersion: "151da5a28d0bd733fe4837547ec909137a36b0e532409c93dfd7bf3c2a7e6b41",
			ReportSHA256:           "b2319f18ba9b65eec5aa720d2867795acdde450a14d0a101d6d4291bda499d3e",
		}
	default:
		return StatementRenderingEvidence{}, false
	}
	ev, ok := historicalStatementRendering(id.SourceID)
	if !ok {
		return StatementRenderingEvidence{}, false
	}
	ev.HistoricalMatch = &match
	return ev, true
}

func historicalStatementRendering(sourceID string) (StatementRenderingEvidence, bool) {
	counts, ok := reviewedStatementRendering[sourceID]
	if !ok {
		return StatementRenderingEvidence{}, false
	}
	return StatementRenderingEvidence{
		ReportSHA256: statementRenderingReport, Examples: counts.examples,
		Formulas: counts.formulas, FormatBlocks: counts.formatBlocks,
	}, true
}
