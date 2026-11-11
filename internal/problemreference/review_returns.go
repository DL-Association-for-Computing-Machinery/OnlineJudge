package problemreference

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ErrReviewReturned means these unchanged materials have already been returned.
var ErrReviewReturned = errors.New("unchanged review materials already returned")

// ErrReviewChanged means a reviewer's original view no longer authorizes a write.
var ErrReviewChanged = errors.New("review evidence or assignment changed")

// ReturnIntent binds the reviewer's decision to the observed assignment and
// evidence. Neither the request key nor the reason is a material identity.
type ReturnIntent struct {
	RequestKey         string `json:"request_key"`
	AssignmentRevision int64  `json:"assignment_revision"`
	ImportRevision     string `json:"import_revision"`
	EvidenceSHA256     string `json:"evidence_sha256"`
	Reason             string `json:"reason"`
}

// Valid checks the explicit bounded return intent.
func (i ReturnIntent) Valid() bool {
	return problem.ValidKey(i.RequestKey) && i.AssignmentRevision > 0 &&
		problem.ValidImportRevision(i.ImportRevision) && problem.ValidImportRevision(i.EvidenceSHA256) && validReturnReason(i.Reason)
}

func validReturnReason(reason string) bool {
	return len(reason) > 0 && len(reason) <= 512 && utf8.ValidString(reason) && strings.TrimSpace(reason) == reason &&
		strings.IndexFunc(reason, unicode.IsControl) < 0
}

// ReviewReturnStatus is the author-visible decision for the selected intake.
// It does not disclose reviewer identity or the reviewer's request key.
type ReviewReturnStatus struct {
	Owner          string    `json:"-"`
	IntakeKey      string    `json:"-"`
	ReviewID       string    `json:"review_id"`
	ImportRevision string    `json:"import_revision"`
	EvidenceSHA256 string    `json:"evidence_sha256"`
	Reason         string    `json:"reason"`
	ReturnedAt     time.Time `json:"returned_at"`
}

// Valid checks the fixed identity and decision reason.
func (s ReviewReturnStatus) Valid() bool {
	return identity.ValidID(s.Owner) && problem.ValidKey(s.IntakeKey) &&
		identity.ValidID(s.ReviewID) && problem.ValidImportRevision(s.ImportRevision) &&
		problem.ValidImportRevision(s.EvidenceSHA256) && validReturnReason(s.Reason) && !s.ReturnedAt.IsZero()
}

// ReviewReturnReceipt is the reviewer-scoped recovery view of a committed return.
type ReviewReturnReceipt struct {
	Reviewer string `json:"-"`
	ReviewID string `json:"review_id"`
	ReturnIntent
	ReturnedAt time.Time `json:"returned_at"`
}

// Valid checks the complete committed reviewer receipt.
func (r ReviewReturnReceipt) Valid() bool {
	return identity.ValidID(r.Reviewer) && identity.ValidID(r.ReviewID) && r.ReturnIntent.Valid() && !r.ReturnedAt.IsZero()
}

// ReviewReturnRepository authorizes and records a return under live locks.
type ReviewReturnRepository interface {
	ReturnReview(context.Context, identity.ProblemActor, string, ReturnIntent) (ReviewReturnReceipt, error)
	ReviewReturnReceipt(context.Context, identity.ProblemActor, string, string) (ReviewReturnReceipt, error)
}

// ReviewerReturns binds return decisions and recovery to ordinary sessions.
type ReviewerReturns struct {
	sessions SessionReader
	repo     ReviewReturnRepository
}

// NewReviewerReturns constructs the reviewer-only return capability.
func NewReviewerReturns(sessions SessionReader, repo ReviewReturnRepository) (*ReviewerReturns, error) {
	if sessions == nil || repo == nil {
		return nil, ErrInvalid
	}
	return &ReviewerReturns{sessions: sessions, repo: repo}, nil
}

// ReturnForSubject records a return or recovers exactly the same intent.
func (s *ReviewerReturns) ReturnForSubject(ctx context.Context, c redisstore.Credential, expected, id string, i ReturnIntent) (ReviewReturnReceipt, error) {
	if !i.Valid() {
		return ReviewReturnReceipt{}, ErrInvalid
	}
	actor, err := s.actor(ctx, c, expected, id)
	if err != nil {
		return ReviewReturnReceipt{}, err
	}
	r, err := s.repo.ReturnReview(ctx, actor, id, i)
	if err != nil {
		return ReviewReturnReceipt{}, err
	}
	if ctx.Err() != nil {
		return ReviewReturnReceipt{}, errors.Join(ErrCommitUnknown, ctx.Err())
	}
	if !r.Valid() || r.Reviewer != expected || r.ReviewID != id || r.ReturnIntent != i {
		return ReviewReturnReceipt{}, ErrStore
	}
	return r, nil
}

// ReturnReceiptForSubject checks the original key under current reviewer access.
func (s *ReviewerReturns) ReturnReceiptForSubject(ctx context.Context, c redisstore.Credential, expected, id, key string) (ReviewReturnReceipt, error) {
	if !problem.ValidKey(key) {
		return ReviewReturnReceipt{}, ErrInvalid
	}
	actor, err := s.actor(ctx, c, expected, id)
	if err != nil {
		return ReviewReturnReceipt{}, err
	}
	r, err := s.repo.ReviewReturnReceipt(ctx, actor, id, key)
	if err != nil {
		return ReviewReturnReceipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return ReviewReturnReceipt{}, err
	}
	if !r.Valid() || r.Reviewer != expected || r.ReviewID != id || r.RequestKey != key {
		return ReviewReturnReceipt{}, ErrStore
	}
	return r, nil
}

func (s *ReviewerReturns) actor(ctx context.Context, c redisstore.Credential, expected, id string) (identity.ProblemActor, error) {
	if s == nil || s.sessions == nil || s.repo == nil || ctx == nil || !identity.ValidID(expected) || !identity.ValidID(id) {
		return identity.ProblemActor{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return identity.ProblemActor{}, err
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
