package problemvalidation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

const aplusbJudgeReportSchema = "oj.aplusb-isolated-judge-report.v1"
const localTextWrongReportSchema = "oj.local-text-wrong-solution-report.v1"

// CheckerTrial is a single use trial. It holds the same verified snapshot for
// preflight and execution. Call Close even when no run was started.
type CheckerTrial struct {
	used             atomic.Bool
	profile          Profile
	tier             experiment.ReferenceTier
	plan             problem.ReferencePlan
	materials        referenceReader
	candidateSHA     string
	manifestSHA      string
	submissionSHA    string
	submissionSource []byte
}

// CheckerTrialReport is private maintenance evidence, separate from accepted
// reference reports and production judge results. It contains no source or I/O.
type CheckerTrialReport struct {
	Schema              string               `json:"schema"`
	CandidateSHA256     string               `json:"candidate_sha256"`
	FormalPlanSHA       string               `json:"formal_plan_sha256"`
	RuntimeProfile      Profile              `json:"runtime_profile"`
	RuntimeSHA256       string               `json:"runtime_profile_sha256"`
	Verdict             string               `json:"verdict"`
	Points              []Point              `json:"points"`
	CompileUsage        *execution.Usage     `json:"compile_usage,omitempty"`
	Diagnostic          *ExecutionDiagnostic `json:"execution_diagnostic,omitempty"`
	StopDiagnostic      *StopDiagnostic      `json:"stop_diagnostic,omitempty"`
	Slot                string               `json:"slot"`
	StopConfirmed       bool                 `json:"stop_confirmed"`
	Complete            bool                 `json:"complete"`
	JudgeManifestSHA256 string               `json:"judge_manifest_sha256,omitempty"`
	SubmissionSHA256    string               `json:"submission_sha256,omitempty"`
}

// PrepareCheckerTrial rederives the candidate from trusted bundle material and
// independently pinned digests. A trial cannot be opened with unequal caps.
func PrepareCheckerTrial(ctx context.Context, prepared *problem.Prepared, expectedPlan, expectedCandidate string, limits judging.TextLimits) (*CheckerTrial, error) {
	return PrepareCheckerTrialWithLifetime(ctx, ctx, prepared, expectedPlan, expectedCandidate, limits)
}

// PrepareCheckerTrialWithLifetime bounds the initial freeze independently of
// later execution against it. The lifetime must remain live until Close.
func PrepareCheckerTrialWithLifetime(ctx, lifetime context.Context, prepared *problem.Prepared, expectedPlan, expectedCandidate string, limits judging.TextLimits) (_ *CheckerTrial, err error) {
	if ctx == nil || lifetime == nil || ctx.Err() != nil || lifetime.Err() != nil {
		return nil, ErrCheckerCandidateBinding
	}
	id, err := prepared.ImportIdentity()
	if err != nil {
		return nil, err
	}
	profile, tier, err := checkerTrialProfile(id.SourceID)
	if err != nil {
		return nil, err
	}
	profile.Text = limits
	if !limits.Valid() || limits.Bytes != profile.Runtime.OutputBytes ||
		!problem.ValidImportRevision(expectedPlan) || !problem.ValidImportRevision(expectedCandidate) {
		return nil, ErrCheckerCandidateBinding
	}
	var plan problem.ReferencePlan
	var reader referenceReader
	if id.SourceID != "library-checker:aplusb" {
		snapshot, err := prepared.PrepareDiskReferenceWithLifetime(ctx, lifetime)
		if err != nil {
			return nil, err
		}
		plan, reader = snapshot.Plan(), snapshot
	} else {
		plan, reader, err = freezeReferences(ctx, prepared, profile.Materials, false)
		if err != nil {
			return nil, err
		}
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, reader.Close())
		}
	}()
	c := checkerCandidateFor(plan, expectedPlan, judging.LibraryCheckerTextVersion, limits)
	if c.Digest() != expectedCandidate {
		return nil, ErrCheckerCandidateBinding
	}
	if err := checkCandidateAnswers(ctx, plan, reader, expectedPlan, limits); err != nil {
		return nil, err
	}
	if err := errors.Join(ctx.Err(), lifetime.Err()); err != nil {
		return nil, err
	}
	return &CheckerTrial{plan: plan, materials: reader, profile: profile, tier: tier, candidateSHA: expectedCandidate}, nil
}

func checkerTrialProfile(sourceID string) (Profile, experiment.ReferenceTier, error) {
	switch sourceID {
	case "library-checker:aplusb":
		p := PinnedProfile()
		p.ID = "p4-checker-candidate-aplusb-reference-v1"
		p.Runtime = experiment.PinnedReferenceEnvironment()
		p.Diagnostics = experiment.ReferenceDiagnosticBytes
		return p, experiment.ReferenceSmall, nil
	case "library-checker:unionfind":
		p := UnionFindProfile()
		p.ID = "p4-checker-candidate-unionfind-reference-v1"
		return p, experiment.ReferenceSmall, nil
	case "library-checker:zalgorithm":
		p, tier, err := ResolveSource(sourceID)
		p.ID = "p4-checker-candidate-zalgorithm-reference-v1"
		return p, tier, err
	default:
		return Profile{}, 0, ErrCheckerCandidateBinding
	}
}

// Tier binds the trial to its reviewed slot and cached runner configuration.
func (t *CheckerTrial) Tier() experiment.ReferenceTier { return t.tier }

// Close releases the snapshot. The owner must not call it during Run.
func (t *CheckerTrial) Close() error {
	if t == nil || t.materials == nil {
		return nil
	}
	return t.materials.Close()
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
	var err error
	schema := "oj.checker-trial.v1"
	runner := New(g)
	if t.submissionSHA == "" {
		r, err = runner.runPrepared(ctx, r, t.profile, t.plan, t.materials, nil, g)
	} else {
		schema = aplusbJudgeReportSchema
		if t.manifestSHA == "" {
			schema = localTextWrongReportSchema
		}
		r, err = runner.runPreparedSource(ctx, r, t.profile, t.plan, t.materials, nil, g, t.submissionSource)
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		r.Verdict = "SE"
	}
	return CheckerTrialReport{
		Schema: schema, CandidateSHA256: t.candidateSHA, FormalPlanSHA: t.plan.Digest(),
		RuntimeProfile: t.profile, RuntimeSHA256: t.profile.Digest(), Verdict: r.Verdict, Points: r.Points,
		CompileUsage: r.CompileUsage, Diagnostic: r.ExecutionDiagnostic, JudgeManifestSHA256: t.manifestSHA, SubmissionSHA256: t.submissionSHA,
	}, err
}

// Finalize makes completion contingent on execution, Stop and lock cleanup.
func (r *CheckerTrialReport) Finalize(runErr, stopErr, cleanErr error) error {
	r.StopConfirmed = stopErr == nil
	err := errors.Join(runErr, stopErr, cleanErr)
	validSchema := r.Schema == "oj.checker-trial.v1" && r.JudgeManifestSHA256 == "" && r.SubmissionSHA256 == "" ||
		r.Schema == aplusbJudgeReportSchema && problem.ValidImportRevision(r.JudgeManifestSHA256) && problem.ValidImportRevision(r.SubmissionSHA256) ||
		r.Schema == localTextWrongReportSchema && r.JudgeManifestSHA256 == "" && problem.ValidImportRevision(r.SubmissionSHA256)
	r.Complete = err == nil && validSchema && r.Verdict != "SE"
	if !r.Complete {
		r.Verdict = "SE"
		if err == nil {
			err = errors.New("checker trial has no conclusive evidence")
		}
	}
	return err
}

// PrepareAPlusBJudge checks the same fixed materials before the host can start.
// The compiled bytes come from the pinned submission rather than the reference.
func PrepareAPlusBJudge(ctx context.Context, prepared *problem.Prepared, plan, candidate, manifest, sourceSHA, source string, limits judging.TextLimits) (*CheckerTrial, error) {
	return PrepareAPlusBJudgeForTier(ctx, prepared, plan, candidate, manifest, sourceSHA, source, limits, experiment.ReferenceSmall)
}

// PrepareAPlusBJudgeForTier requires a manifest matching the explicitly selected standalone runtime.
func PrepareAPlusBJudgeForTier(ctx context.Context, prepared *problem.Prepared, plan, candidate, manifest, sourceSHA, source string, limits judging.TextLimits, tier experiment.ReferenceTier) (*CheckerTrial, error) {
	if (tier != experiment.ReferenceSmall && tier != experiment.ReferenceAPlusBUbuntu) ||
		!problem.ValidImportRevision(manifest) || !problem.ValidImportRevision(sourceSHA) ||
		len(source) == 0 || len(source) > submission.MaxSourceBytes || !utf8.ValidString(source) || strings.ContainsRune(source, 0) {
		return nil, ErrCheckerCandidateBinding
	}
	h := sha256.Sum256([]byte(source))
	if hex.EncodeToString(h[:]) != sourceSHA {
		return nil, ErrCheckerCandidateBinding
	}
	trial, err := prepareAPlusBJudgeTrial(ctx, prepared, plan, candidate, limits, tier)
	if err != nil {
		return nil, err
	}
	if trial.aplusbManifest().Digest() != manifest || ctx.Err() != nil {
		return nil, errors.Join(ErrCheckerCandidateBinding, trial.Close(), ctx.Err())
	}
	trial.manifestSHA, trial.submissionSHA, trial.submissionSource = manifest, sourceSHA, []byte(source)
	return trial, nil
}
