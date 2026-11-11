package problemvalidation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

// CheckerCandidateSchema identifies the immutable configuration encoding.
const CheckerCandidateSchema = "oj.checker-candidate.v1"

// ErrCheckerCandidateBinding marks a missing or different trusted plan.
var ErrCheckerCandidateBinding = errors.New("checker candidate does not match the expected plan")

// CheckerCandidate records the exact settings whose reference answers passed
// offline preflight. The digest identifies content; it grants no authority.
type CheckerCandidate struct {
	Schema           string `json:"schema"`
	CheckerVersion   string `json:"checker_version"`
	Bytes            int    `json:"bytes"`
	TokenBytes       int    `json:"token_bytes"`
	SourceID         string `json:"source_id"`
	ContentVersion   string `json:"content_version"`
	ImportRevision   string `json:"import_revision"`
	SourceLockSHA256 string `json:"source_lock_sha256"`
	FormalPlanSHA256 string `json:"formal_plan_sha256"`
	AnswerCount      int    `json:"answer_count"`
}

// Digest is SHA-256 of the compact JSON configuration, without a trailing newline.
func (c CheckerCandidate) Digest() string {
	raw, _ := json.Marshal(c)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// PreflightCheckerCandidate checks every answer of a bounded frozen reference
// plan. The caller supplies the independently trusted expected plan digest
// and loads the bundle using a trusted source lock. Neither answer validity nor
// an exact digest proves contestant execution on a target machine.
func PreflightCheckerCandidate(ctx context.Context, prepared *problem.Prepared, expectedPlan, version string, limits judging.TextLimits) (CheckerCandidate, error) {
	if version != judging.LibraryCheckerTextVersion {
		return CheckerCandidate{}, errors.New("unsupported checker version")
	}
	if !limits.Valid() {
		return CheckerCandidate{}, judging.ErrTextLimits
	}
	if ctx == nil || !problem.ValidImportRevision(expectedPlan) {
		return CheckerCandidate{}, ErrCheckerCandidateBinding
	}
	id, err := prepared.ImportIdentity()
	if err != nil {
		return CheckerCandidate{}, err
	}
	plan, reader, err := freezeReferences(ctx, prepared, problem.SmallReferenceLimits(), id.SourceID != "library-checker:aplusb")
	if err != nil {
		return CheckerCandidate{}, err
	}
	err = checkCandidateAnswers(ctx, plan, reader, expectedPlan, limits)
	if err := errors.Join(err, reader.Close(), ctx.Err()); err != nil {
		return CheckerCandidate{}, err
	}
	return CheckerCandidate{
		Schema: CheckerCandidateSchema, CheckerVersion: version, Bytes: limits.Bytes, TokenBytes: limits.TokenBytes,
		SourceID: plan.SourceID, ContentVersion: plan.Version, ImportRevision: plan.ImportRevision,
		SourceLockSHA256: plan.SourceLockSHA256, FormalPlanSHA256: expectedPlan, AnswerCount: len(plan.Cases),
	}, nil
}

func checkCandidateAnswers(ctx context.Context, plan problem.ReferencePlan, reader referenceReader, expected string, limits judging.TextLimits) error {
	if plan.Digest() != expected {
		return ErrCheckerCandidateBinding
	}
	for i, c := range plan.Cases {
		r, size, err := reader.AnswerAt(ctx, i)
		if err != nil {
			return fmt.Errorf("%s: %w", c.ID, err)
		}
		if size != c.Answer.Size {
			return fmt.Errorf("%s: %w", c.ID, judging.ErrTextRead)
		}
		if err := judging.ValidateLibraryCheckerReferenceAt(ctx, judging.FrozenText{Reader: r, Size: size}, limits); err != nil {
			return fmt.Errorf("%s: %w", c.ID, err)
		}
	}
	return nil
}
