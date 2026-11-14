package problemreference

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ReviewApprovalRepository rechecks current reviewer and assignment authority under lock.
type ReviewApprovalRepository interface {
	ApproveReview(context.Context, identity.ProblemActor, string, ApprovalIntent) (ReviewApprovalReceipt, error)
	ReviewApprovalReceipt(context.Context, identity.ProblemActor, string, string) (ReviewApprovalReceipt, error)
}

// ReviewerApprovals resolves an ordinary session before each decision or readback.
type ReviewerApprovals struct {
	sessions SessionReader
	repo     ReviewApprovalRepository
}

// NewReviewerApprovals constructs the optional approval capability.
func NewReviewerApprovals(sessions SessionReader, repo ReviewApprovalRepository) (*ReviewerApprovals, error) {
	if sessions == nil || repo == nil {
		return nil, ErrInvalid
	}
	return &ReviewerApprovals{sessions: sessions, repo: repo}, nil
}

// ApprovalForSubject records a decision under a fresh session observation.
func (s *ReviewerApprovals) ApprovalForSubject(ctx context.Context, c redisstore.Credential, expected, id string, i ApprovalIntent) (ReviewApprovalReceipt, error) {
	if !i.Valid() {
		return ReviewApprovalReceipt{}, ErrInvalid
	}
	a, err := s.actor(ctx, c, expected, id)
	if err != nil {
		return ReviewApprovalReceipt{}, err
	}
	return s.repo.ApproveReview(ctx, a, id, i)
}

// ApprovalReceiptForSubject reads the original request key under current authority.
func (s *ReviewerApprovals) ApprovalReceiptForSubject(ctx context.Context, c redisstore.Credential, expected, id, key string) (ReviewApprovalReceipt, error) {
	if !problem.ValidKey(key) {
		return ReviewApprovalReceipt{}, ErrInvalid
	}
	a, err := s.actor(ctx, c, expected, id)
	if err != nil {
		return ReviewApprovalReceipt{}, err
	}
	return s.repo.ReviewApprovalReceipt(ctx, a, id, key)
}

func (s *ReviewerApprovals) actor(ctx context.Context, c redisstore.Credential, expected, id string) (identity.ProblemActor, error) {
	var zero identity.ProblemActor
	if s == nil || s.sessions == nil || s.repo == nil || ctx == nil || !identity.ValidID(expected) || !identity.ValidID(id) {
		return zero, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	a, err := observeActor(ctx, s.sessions, c)
	if err != nil {
		return zero, err
	}
	if a.SubjectID != expected {
		return zero, ErrSubjectChanged
	}
	return a, nil
}
