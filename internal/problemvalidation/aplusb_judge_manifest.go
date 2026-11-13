package problemvalidation

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

const (
	aplusbJudgeSchema = "oj.aplusb-isolated-judge.v1"
	aplusbSourceID    = "library-checker:aplusb"
)

// APlusBJudgeManifest identifies one isolated judge configuration and its
// ordered materials. It contains no reference program or test bytes.
type APlusBJudgeManifest struct {
	Schema          string            `json:"schema"`
	Candidate       CheckerCandidate  `json:"candidate"`
	CandidateSHA256 string            `json:"candidate_sha256"`
	RuntimeSHA256   string            `json:"runtime_profile_sha256"`
	Tests           []APlusBJudgeTest `json:"tests"`
}

// ExportAPlusBMaterials emits only the verified input and answer objects. The
// caller must stage them privately and publish metadata only after success.
func ExportAPlusBMaterials(ctx context.Context, prepared *problem.Prepared, plan, candidate, expected string, limits judging.TextLimits,
	put func(string, []byte) error) (manifest materials.APlusBManifest, err error) {
	trial, err := prepareAPlusBJudgeTrial(ctx, prepared, plan, candidate, limits)
	if err != nil {
		return manifest, err
	}
	defer func() { err = errors.Join(err, trial.Close()) }()
	if trial.aplusbManifest().Digest() != expected || len(trial.plan.Cases) != 17 || put == nil {
		return manifest, ErrCheckerCandidateBinding
	}
	manifest = materials.APlusBManifest{Version: "aplusb-" + expected, Origin: materials.APlusBOrigin,
		Language: "gnu++17", JudgeSHA256: expected, TokenBytes: limits.TokenBytes,
		Checker: judging.LibraryCheckerTextVersion, RuntimeSHA256: experiment.APlusBRuntimeSHA256()}
	seen := make(map[string]bool)
	for _, point := range trial.plan.Cases {
		for _, obj := range []struct {
			hash string
			data []byte
		}{
			{point.Input.SHA256, point.InputData}, {point.Answer.SHA256, point.AnswerData},
		} {
			if err := ctx.Err(); err != nil {
				return materials.APlusBManifest{}, err
			}
			if !seen[obj.hash] {
				if err := put(obj.hash, obj.data); err != nil {
					return materials.APlusBManifest{}, err
				}
				seen[obj.hash] = true
			}
		}
		manifest.Tests = append(manifest.Tests, materials.APlusBPoint{ID: point.ID, Input: point.Input.SHA256, Answer: point.Answer.SHA256})
	}
	return manifest, ctx.Err()
}

// APlusBJudgeTest records the identities and sizes of the point's two objects.
type APlusBJudgeTest struct {
	ID     string               `json:"id"`
	Input  materialstore.Object `json:"input"`
	Answer materialstore.Object `json:"answer"`
}

// Digest binds the compact manifest JSON; it is not an authorization.
func (m APlusBJudgeManifest) Digest() string { return digest(m) }

// PrepareAPlusBJudgeManifest rederives the fixed material manifest from an
// independently pinned formal plan and checker candidate.
func PrepareAPlusBJudgeManifest(ctx context.Context, prepared *problem.Prepared, plan, candidate string, limits judging.TextLimits) (APlusBJudgeManifest, error) {
	var zero APlusBJudgeManifest
	trial, err := prepareAPlusBJudgeTrial(ctx, prepared, plan, candidate, limits)
	if err != nil {
		return zero, err
	}
	manifest := trial.aplusbManifest()
	if err := errors.Join(trial.Close(), ctx.Err()); err != nil {
		return zero, err
	}
	return manifest, nil
}

func prepareAPlusBJudgeTrial(ctx context.Context, prepared *problem.Prepared, plan, candidate string, limits judging.TextLimits) (*CheckerTrial, error) {
	id, err := prepared.ImportIdentity()
	if err != nil {
		return nil, err
	}
	if id.SourceID != aplusbSourceID || id.SourceLockSHA256 != SourceLockSHA256 {
		return nil, ErrCheckerCandidateBinding
	}
	return PrepareCheckerTrial(ctx, prepared, plan, candidate, limits)
}

func (t *CheckerTrial) aplusbManifest() APlusBJudgeManifest {
	points := make([]APlusBJudgeTest, len(t.plan.Cases))
	for i, c := range t.plan.Cases {
		points[i] = APlusBJudgeTest{ID: c.ID, Input: c.Input, Answer: c.Answer}
	}
	return APlusBJudgeManifest{
		Schema: aplusbJudgeSchema, Candidate: checkerCandidateFor(t.plan, t.plan.Digest(), judging.LibraryCheckerTextVersion, t.profile.Text),
		CandidateSHA256: t.candidateSHA, RuntimeSHA256: t.profile.Digest(),
		Tests: points,
	}
}
