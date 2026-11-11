package problemvalidation

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

// CheckerTrial is a single use, fully preflighted A+B trial. It owns the
// same bounded memory snapshot for preflight and execution.
type CheckerTrial struct {
	used         atomic.Bool
	profile      Profile
	plan         problem.ReferencePlan
	candidateSHA string
}

// CheckerTrialReport is private maintenance evidence, separate from accepted
// reference reports and production judge results. It contains no source or I/O.
type CheckerTrialReport struct {
	Schema          string               `json:"schema"`
	CandidateSHA256 string               `json:"candidate_sha256"`
	FormalPlanSHA   string               `json:"formal_plan_sha256"`
	RuntimeProfile  Profile              `json:"runtime_profile"`
	RuntimeSHA256   string               `json:"runtime_profile_sha256"`
	Verdict         string               `json:"verdict"`
	Points          []Point              `json:"points"`
	CompileUsage    *execution.Usage     `json:"compile_usage,omitempty"`
	Diagnostic      *ExecutionDiagnostic `json:"execution_diagnostic,omitempty"`
	StopDiagnostic  *StopDiagnostic      `json:"stop_diagnostic,omitempty"`
	Slot            string               `json:"slot"`
	StopConfirmed   bool                 `json:"stop_confirmed"`
	Complete        bool                 `json:"complete"`
}

// PrepareCheckerTrial rederives the candidate from trusted bundle material and
// independently pinned digests. A trial cannot be opened with unequal caps.
func PrepareCheckerTrial(ctx context.Context, prepared *problem.Prepared, expectedPlan, expectedCandidate string, limits judging.TextLimits) (*CheckerTrial, error) {
	profile := PinnedProfile()
	profile.ID = "p4-checker-candidate-aplusb-reference-v1"
	profile.Runtime = experiment.PinnedReferenceEnvironment()
	profile.Diagnostics = experiment.ReferenceDiagnosticBytes
	profile.Text = limits
	if ctx == nil || !limits.Valid() || limits.Bytes != profile.Runtime.OutputBytes ||
		!problem.ValidImportRevision(expectedPlan) || !problem.ValidImportRevision(expectedCandidate) {
		return nil, ErrCheckerCandidateBinding
	}
	plan, err := prepared.PrepareReference(ctx, profile.Materials)
	if err != nil {
		return nil, err
	}
	c := checkerCandidateFor(plan, expectedPlan, judging.LibraryCheckerTextVersion, limits)
	if c.Digest() != expectedCandidate {
		return nil, ErrCheckerCandidateBinding
	}
	if err := checkCandidateAnswers(ctx, plan, memoryReference{plan}, expectedPlan, limits); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &CheckerTrial{plan: plan, profile: profile, candidateSHA: expectedCandidate}, nil
}

// Run uses only the verified reference cache and returns provisional evidence.
// The caller owns the slot and must confirm Stop independently before Finalize.
func (t *CheckerTrial) Run(ctx context.Context, g *execution.CachedGoJudge) (CheckerTrialReport, error) {
	if t == nil || ctx == nil || g == nil || t.candidateSHA == "" || !t.used.CompareAndSwap(false, true) {
		return CheckerTrialReport{}, errors.New("invalid or used checker trial")
	}
	ctx, cancel := context.WithTimeout(ctx, t.profile.Wall)
	defer cancel()
	r := Report{Verdict: "SE", Points: make([]Point, 0, len(t.plan.Cases))}
	for _, c := range t.plan.Cases {
		r.Points = append(r.Points, Point{ID: c.ID, Verdict: "NOT_RUN"})
	}
	r, err := New(g).runPrepared(ctx, r, t.profile, t.plan, memoryReference{t.plan}, nil, g)
	if err = errors.Join(err, ctx.Err()); err != nil {
		r.Verdict = "SE"
	}
	return CheckerTrialReport{
		Schema: "oj.checker-trial.v1", CandidateSHA256: t.candidateSHA, FormalPlanSHA: t.plan.Digest(),
		RuntimeProfile: t.profile, RuntimeSHA256: t.profile.Digest(), Verdict: r.Verdict, Points: r.Points,
		CompileUsage: r.CompileUsage, Diagnostic: r.ExecutionDiagnostic,
	}, err
}

// Finalize makes completion contingent on execution, Stop and lock cleanup.
func (r *CheckerTrialReport) Finalize(runErr, stopErr, cleanErr error) error {
	r.StopConfirmed = stopErr == nil
	err := errors.Join(runErr, stopErr, cleanErr)
	r.Complete = err == nil && r.Schema == "oj.checker-trial.v1" && r.Verdict != "SE"
	if !r.Complete {
		r.Verdict = "SE"
		if err == nil {
			err = errors.New("checker trial has no conclusive evidence")
		}
	}
	return err
}
