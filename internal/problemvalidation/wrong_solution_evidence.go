package problemvalidation

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"

// WrongSolutionEvidence summarizes one reviewed counterexample on the frozen
// formal test set. It does not establish complete test coverage.
type WrongSolutionEvidence struct {
	ReportSHA256          string `json:"report_sha256"`
	Origin                string `json:"origin"`
	Candidate             string `json:"candidate"`
	RejectedCasePosition  int    `json:"rejected_case_position"`
	AcceptedCasePositions []int  `json:"accepted_case_positions"`
}

type reviewedWrongSolution struct {
	origin, candidate string
	rejected          int
	accepted          []int
}

// Positions are one-based in the formal plan. Accepted positions retain
// probe order, which need not be the order of the plan.
var reviewedV6WrongSolutions = map[string]reviewedWrongSolution{
	"library-checker:aplusb":              {origin: "upstream", candidate: "wa.cpp", rejected: 4},
	"library-checker:associative_array":   {origin: "local", candidate: "first-write-wins", rejected: 17},
	ConvolutionSourceID:                   {origin: "local", candidate: "omit-last-coefficient", rejected: 8},
	"library-checker:point_add_range_sum": {origin: "local", candidate: "ignore-updates", rejected: 4},
	"library-checker:primality_test":      {origin: "upstream", candidate: "insufficient_random_bases.cpp", rejected: 13},
	"library-checker:static_range_sum":    {origin: "upstream", candidate: "wa.cpp", rejected: 2},
	"library-checker:staticrmq":           {origin: "local", candidate: "right-endpoint-only", rejected: 4, accepted: []int{8, 9}},
	"library-checker:suffixarray":         {origin: "local", candidate: "leading-character-only", rejected: 32},
	"library-checker:unionfind":           {origin: "local", candidate: "direct-edges-only", rejected: 10, accepted: []int{9}},
	"library-checker:zalgorithm":          {origin: "local", candidate: "zero-at-origin", rejected: 5},
}

// AdmittedWrongSolution returns the reviewed summary for an exact v6 binding.
// Callers must verify the identity and plan in trusted stored evidence first.
func AdmittedWrongSolution(identity problem.ImportIdentity, planSHA256 string) (WrongSolutionEvidence, bool) {
	if _, ok := admittedV6Binding(identity, planSHA256); !ok {
		return WrongSolutionEvidence{}, false
	}
	row, ok := reviewedV6WrongSolutions[identity.SourceID]
	if !ok {
		return WrongSolutionEvidence{}, false
	}
	return WrongSolutionEvidence{
		ReportSHA256: admittedInputValidatorReport, Origin: row.origin, Candidate: row.candidate,
		RejectedCasePosition: row.rejected, AcceptedCasePositions: append([]int{}, row.accepted...),
	}, true
}
