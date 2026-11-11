package problemreference

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ReviewIntent selects an exact draft revision and one already accepted report.
// The client does not supply the evidence hashes or a verdict.
type ReviewIntent struct {
	RequestKey         string `json:"request_key"`
	DraftID            string `json:"draft_id"`
	ImportRevision     string `json:"import_revision"`
	ReferenceRequestID string `json:"reference_request_id"`
}

// Valid checks the input shape, not current authorization.
func (i ReviewIntent) Valid() bool {
	return problem.ValidKey(i.RequestKey) && identity.ValidID(i.DraftID) &&
		problem.ValidImportRevision(i.ImportRevision) && identity.ValidID(i.ReferenceRequestID)
}

// ReviewReceipt acknowledges intake only. It is not an approval, validation
// state or grant to read the selected report.
type ReviewReceipt struct {
	Owner string `json:"-"`
	ID    string `json:"id"`
	ReviewIntent
	ContentVersion string    `json:"content_version"`
	CreatedAt      time.Time `json:"created_at"`
}

// Valid checks the receipt shape before it crosses the HTTP boundary.
func (r ReviewReceipt) Valid() bool {
	return identity.ValidID(r.Owner) && identity.ValidID(r.ID) && r.ReviewIntent.Valid() &&
		problem.ValidImportRevision(r.ContentVersion) && !r.CreatedAt.IsZero()
}

// ReviewIntakeRepository performs an author check in the same transaction as
// an immutable intake write or owner receipt.
type ReviewIntakeRepository interface {
	SubmitReview(context.Context, identity.ProblemActor, ReviewIntent) (ReviewReceipt, error)
	ReviewReceipt(context.Context, identity.ProblemActor, string) (ReviewReceipt, error)
}

// ReviewIntake is the author-facing intake capability. There is no reviewer
// assignment or approval in this service.
type ReviewIntake struct {
	sessions SessionReader
	repo     ReviewIntakeRepository
}

// NewReviewIntake binds intake to ordinary sessions and current DB authority.
func NewReviewIntake(sessions SessionReader, repo ReviewIntakeRepository) (*ReviewIntake, error) {
	if sessions == nil || repo == nil {
		return nil, ErrInvalid
	}
	return &ReviewIntake{sessions: sessions, repo: repo}, nil
}

// SubmitForSubject records or recovers the exact owner/key intent.
func (s *ReviewIntake) SubmitForSubject(ctx context.Context, credential redisstore.Credential, expected string, input ReviewIntent) (ReviewReceipt, error) {
	if !input.Valid() {
		return ReviewReceipt{}, ErrInvalid
	}
	actor, err := s.actor(ctx, credential, expected)
	if err != nil {
		return ReviewReceipt{}, err
	}
	receipt, err := s.repo.SubmitReview(ctx, actor, input)
	if err != nil {
		return ReviewReceipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return ReviewReceipt{}, errors.Join(ErrCommitUnknown, err)
	}
	if !receipt.Valid() || receipt.Owner != expected || receipt.ReviewIntent != input {
		return ReviewReceipt{}, ErrStore
	}
	return receipt, nil
}

// ReceiptForSubject observes the original key. Absence is not proof that an
// earlier write was rolled back.
func (s *ReviewIntake) ReceiptForSubject(ctx context.Context, credential redisstore.Credential, expected, key string) (ReviewReceipt, error) {
	if !problem.ValidKey(key) {
		return ReviewReceipt{}, ErrInvalid
	}
	actor, err := s.actor(ctx, credential, expected)
	if err != nil {
		return ReviewReceipt{}, err
	}
	receipt, err := s.repo.ReviewReceipt(ctx, actor, key)
	if err != nil {
		return ReviewReceipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return ReviewReceipt{}, err
	}
	if !receipt.Valid() || receipt.Owner != expected || receipt.RequestKey != key {
		return ReviewReceipt{}, ErrStore
	}
	return receipt, nil
}

func (s *ReviewIntake) actor(ctx context.Context, credential redisstore.Credential, expected string) (identity.ProblemActor, error) {
	if s == nil || ctx == nil || s.sessions == nil || s.repo == nil || !identity.ValidID(expected) {
		return identity.ProblemActor{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return identity.ProblemActor{}, err
	}
	a, err := observeActor(ctx, s.sessions, credential)
	if err != nil {
		return identity.ProblemActor{}, err
	}
	if a.SubjectID != expected {
		return identity.ProblemActor{}, ErrSubjectChanged
	}
	return a, nil
}
