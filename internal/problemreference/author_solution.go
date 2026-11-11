package problemreference

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// SolutionBinding identifies the exact import and formal solution closure.
type SolutionBinding struct {
	ImportRevision   string `json:"import_revision"`
	FormalPlanSHA256 string `json:"formal_plan_sha256"`
}

// Valid checks the format of both expected digests.
func (b SolutionBinding) Valid() bool {
	return problem.ValidImportRevision(b.ImportRevision) && problem.ValidImportRevision(b.FormalPlanSHA256)
}

// SolutionConfirmation is the owner's receipt for an explicit confirmation.
type SolutionConfirmation struct {
	Owner     string `json:"-"`
	IntakeKey string `json:"request_key"`
	SolutionBinding
	ConfirmedAt time.Time `json:"confirmed_at"`
}

// Valid checks the receipt's public and authorization binding fields.
func (r SolutionConfirmation) Valid() bool {
	return identity.ValidID(r.Owner) && problem.ValidKey(r.IntakeKey) && r.SolutionBinding.Valid() && !r.ConfirmedAt.IsZero()
}

// AuthorSolutionSnapshot contains only the authorized selection and its binding.
type AuthorSolutionSnapshot struct {
	Owner, DraftID, ReviewID string
	AuthorRevision           int64
	Binding                  SolutionBinding
	Manifest                 problem.ReviewMaterialManifest
}

// AuthorSolution is the exact source the owner can inspect before confirming.
type AuthorSolution struct {
	SolutionBinding
	StandardSolution problem.ReviewSolution `json:"standard_solution"`
}

// MaxAuthorSolutionResponse bounds JSON expansion of the selected source closure.
const MaxAuthorSolutionResponse = 512 << 10

// AuthorSolutionRepository rechecks author authority and fixed review evidence
// inside each snapshot, receipt, or confirmation transaction.
type AuthorSolutionRepository interface {
	AuthorSolutionSnapshot(context.Context, identity.ProblemActor, string) (AuthorSolutionSnapshot, error)
	ConfirmAuthorSolution(context.Context, identity.ProblemActor, string, AuthorSolutionSnapshot) (SolutionConfirmation, error)
	AuthorSolutionConfirmation(context.Context, identity.ProblemActor, string) (SolutionConfirmation, error)
}

// AuthorSolutions reads and records only the current author's explicit intent.
type AuthorSolutions struct {
	sessions SessionReader
	repo     AuthorSolutionRepository
	objects  problem.ReferenceObjectReader
	acquire  func() (func(), error)
}

// NewAuthorSolutions shares the runtime's private material read budget.
func NewAuthorSolutions(sessions SessionReader, repo AuthorSolutionRepository, objects problem.ReferenceObjectReader, acquire func() (func(), error)) (*AuthorSolutions, error) {
	if sessions == nil || repo == nil || objects == nil || acquire == nil {
		return nil, ErrInvalid
	}
	return &AuthorSolutions{sessions: sessions, repo: repo, objects: objects, acquire: acquire}, nil
}

// SolutionForSubject reads the exact, authorized formal source and dependencies.
func (s *AuthorSolutions) SolutionForSubject(ctx context.Context, c redisstore.Credential, expected, key string) (AuthorSolution, error) {
	return s.read(ctx, c, expected, key, nil, func(a identity.ProblemActor, snapshot AuthorSolutionSnapshot) error {
		after, err := s.repo.AuthorSolutionSnapshot(ctx, a, key)
		if err != nil {
			return err
		}
		if !snapshot.SameSelection(after) {
			return ErrReviewChanged
		}
		return ctx.Err()
	})
}

// ConfirmSolutionForSubject explicitly confirms the expected frozen solution.
func (s *AuthorSolutions) ConfirmSolutionForSubject(ctx context.Context, c redisstore.Credential, expected, key string, binding SolutionBinding) (SolutionConfirmation, error) {
	if !binding.Valid() {
		return SolutionConfirmation{}, ErrInvalid
	}
	var receipt SolutionConfirmation
	_, err := s.read(ctx, c, expected, key, &binding, func(a identity.ProblemActor, snapshot AuthorSolutionSnapshot) error {
		var err error
		receipt, err = s.repo.ConfirmAuthorSolution(ctx, a, key, snapshot)
		return err
	})
	if err != nil {
		return SolutionConfirmation{}, err
	}
	if err := ctx.Err(); err != nil {
		return SolutionConfirmation{}, errors.Join(ErrCommitUnknown, err)
	}
	if !receipt.Valid() || receipt.Owner != expected || receipt.IntakeKey != key || receipt.SolutionBinding != binding {
		return SolutionConfirmation{}, ErrStore
	}
	return receipt, nil
}

// ConfirmationForSubject recovers a receipt using the original intake key.
func (s *AuthorSolutions) ConfirmationForSubject(ctx context.Context, c redisstore.Credential, expected, key string) (SolutionConfirmation, error) {
	a, err := s.actor(ctx, c, expected, key)
	if err != nil {
		return SolutionConfirmation{}, err
	}
	r, err := s.repo.AuthorSolutionConfirmation(ctx, a, key)
	if err != nil {
		return SolutionConfirmation{}, err
	}
	if err := ctx.Err(); err != nil {
		return SolutionConfirmation{}, err
	}
	if !r.Valid() || r.Owner != expected || r.IntakeKey != key {
		return SolutionConfirmation{}, ErrStore
	}
	return r, nil
}

// SameSelection also compares current role revision and frozen references.
func (s AuthorSolutionSnapshot) SameSelection(other AuthorSolutionSnapshot) bool {
	return s.Owner == other.Owner && s.DraftID == other.DraftID && s.ReviewID == other.ReviewID &&
		s.AuthorRevision == other.AuthorRevision && s.Binding == other.Binding && s.Manifest.Equal(other.Manifest)
}

func (s *AuthorSolutions) read(ctx context.Context, c redisstore.Credential, expected, key string, binding *SolutionBinding,
	recheck func(identity.ProblemActor, AuthorSolutionSnapshot) error,
) (AuthorSolution, error) {
	var zero AuthorSolution
	before, err := s.actor(ctx, c, expected, key)
	if err != nil {
		return zero, err
	}
	frozen, err := s.repo.AuthorSolutionSnapshot(ctx, before, key)
	if err != nil {
		return zero, err
	}
	if frozen.Owner != expected || frozen.AuthorRevision < 1 || !identity.ValidID(frozen.DraftID) ||
		!identity.ValidID(frozen.ReviewID) || !frozen.Binding.Valid() {
		return zero, ErrStore
	}
	if binding != nil && frozen.Binding != *binding {
		return zero, ErrReviewChanged
	}
	release, err := s.acquire()
	if err != nil {
		return zero, err
	}
	if release == nil {
		return zero, ErrStore
	}
	defer release()
	view, err := frozen.Manifest.ReadSolution(ctx, s.objects)
	if err != nil {
		return zero, err
	}
	after, err := s.actor(ctx, c, expected, key)
	if err != nil {
		return zero, err
	}
	if after != before {
		return zero, identity.ErrConflict
	}
	if err := recheck(after, frozen); err != nil {
		return zero, err
	}
	return AuthorSolution{SolutionBinding: frozen.Binding, StandardSolution: view}, nil
}

func (s *AuthorSolutions) actor(ctx context.Context, c redisstore.Credential, expected, key string) (identity.ProblemActor, error) {
	if s == nil || ctx == nil || s.sessions == nil || s.repo == nil || s.objects == nil || s.acquire == nil ||
		!identity.ValidID(expected) || !problem.ValidKey(key) {
		return identity.ProblemActor{}, ErrInvalid
	}
	a, err := observeActor(ctx, s.sessions, c)
	if err != nil {
		return identity.ProblemActor{}, err
	}
	if a.SubjectID != expected {
		return identity.ProblemActor{}, ErrSubjectChanged
	}
	return a, nil
}
