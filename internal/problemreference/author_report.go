package problemreference

import (
	"context"
	"encoding/json"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// MaxAuthorReportBytes bounds the public projection, including the small
// generated counterexample. No stored Worker evidence is returned verbatim.
const MaxAuthorReportBytes = 2048

// AuthorReport is the result visible to the current owner of the private draft.
type AuthorReport struct {
	Owner      string                  `json:"-"`
	RequestID  string                  `json:"request_id"`
	RequestKey string                  `json:"request_key"`
	DraftID    string                  `json:"draft_id"`
	Version    string                  `json:"content_version"`
	Revision   string                  `json:"import_revision"`
	Formal     AuthorFormalResult      `json:"formal"`
	Comparison *AuthorComparisonResult `json:"comparison"`
}

// AuthorFormalResult gives only the aggregate outcome of the official tests.
type AuthorFormalResult struct {
	Verdict string `json:"verdict"`
	Passed  int    `json:"passed"`
	Total   int    `json:"total"`
}

// AuthorComparisonResult contains only the outcome and, for a mismatch, the
// generated small counterexample. There are no official inputs or answers.
type AuthorComparisonResult struct {
	Status         string                `json:"status"`
	Matched        int                   `json:"matched"`
	Total          int                   `json:"total"`
	Counterexample *AuthorCounterexample `json:"counterexample"`
}

// AuthorCounterexample is the bounded generated input and two output vectors.
type AuthorCounterexample struct {
	Case     int      `json:"case"`
	Input    string   `json:"input"`
	Expected []uint32 `json:"expected"`
	Actual   []uint32 `json:"actual"`
}

// ProjectAuthorReport checks the historical binding and builds the explicit
// author view. Current Worker eligibility is intentionally irrelevant here.
func ProjectAuthorReport(r Request, expected ExpectedPlan, formalSHA, evidenceSHA string, raw []byte) (AuthorReport, error) {
	var zero AuthorReport
	if r.State != Reported || !r.Specification.Valid() || !expected.ValidForRequest(r) ||
		formalSHA != expected.SHA256 || len(raw) > MaxWorkerEvidenceBytes || profileDigest(raw) != evidenceSHA {
		return zero, ErrInvalid
	}
	e, err := decodeWorkerEvidence(raw)
	if err != nil || e.ProfileSHA256 != r.Specification.ProfileSHA256 {
		return zero, ErrInvalid
	}
	var formal problem.ReferencePlan
	var profile problemvalidation.Profile
	if json.Unmarshal(expected.Encoding, &formal) != nil || json.Unmarshal(r.Specification.ProfileJSON, &profile) != nil ||
		!e.validFormalResults(formal, profile) {
		return zero, ErrInvalid
	}
	view := AuthorReport{Owner: r.Owner, RequestID: r.ID, RequestKey: r.RequestKey, DraftID: r.DraftID, Version: formal.Version,
		Revision: r.Specification.Identity.ImportRevision,
		Formal:   AuthorFormalResult{Verdict: e.Verdict, Total: len(formal.Cases)}}
	for _, p := range e.Points {
		if p.Verdict == "AC" {
			view.Formal.Passed++
		}
	}
	// An independent plan could already exist before comparison evidence
	// became mandatory at first report acceptance.
	if c := e.Comparison; c != nil {
		var plan problemvalidation.ConvolutionPlan
		if expected.Differential == nil || c.PlanSHA256 != expected.Differential.SHA256 ||
			json.Unmarshal(expected.Differential.Encoding, &plan) != nil || !plan.ValidHistoricalOutcome(c.Status, c.Matched, c.Counterexample) {
			return zero, ErrInvalid
		}
		view.Comparison = &AuthorComparisonResult{Status: c.Status, Matched: c.Matched, Total: plan.Cases}
		if diff := c.Counterexample; diff != nil {
			view.Comparison.Counterexample = &AuthorCounterexample{Case: diff.Case, Input: diff.Input, Expected: diff.Expected, Actual: diff.Actual}
		}
	}
	return view, nil
}

// ReportForSubject reads the terminal result using the same ordinary-session
// subject guard as the other author operations.
func (s *Service) ReportForSubject(ctx context.Context, credential redisstore.Credential, expected, key string) (AuthorReport, error) {
	var zero AuthorReport
	if s == nil || ctx == nil || s.sessions == nil || s.repository == nil || !identity.ValidID(expected) || !problem.ValidKey(key) {
		return zero, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	actor, err := observeActor(ctx, s.sessions, credential)
	if err != nil {
		return zero, err
	}
	if actor.SubjectID != expected {
		return zero, ErrSubjectChanged
	}
	view, err := s.repository.AuthorReport(ctx, actor, key)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if view.Owner != expected || view.RequestKey != key {
		return zero, ErrStore
	}
	return view, nil
}
