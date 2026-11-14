package problemreference

import (
	"context"
	"crypto/cipher"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ReviewDiscoveryLimit is the maximum number of historical candidates in a page.
const ReviewDiscoveryLimit = 20

// ErrReviewCursor is returned for an invalid or no longer current page cursor.
var ErrReviewCursor = errors.New("invalid or expired review assignment cursor")

// ReviewDiscoveryItem is the public projection of one currently assigned item.
type ReviewDiscoveryItem struct {
	ReviewID           string `json:"review_id"`
	SourceID           string `json:"source_id"`
	ImportRevision     string `json:"import_revision"`
	AssignmentRevision int64  `json:"assignment_revision"`
}

// Valid checks the item shape, independently of assignment authorization.
func (i ReviewDiscoveryItem) Valid() bool {
	return identity.ValidID(i.ReviewID) && i.SourceID != "" && len(i.SourceID) <= 128 &&
		problem.ValidImportRevision(i.ImportRevision) && i.AssignmentRevision > 0
}

// ReviewDiscoveryPosition is an internal seek boundary. ID may identify a stale
// candidate and must not be exposed outside the sealed cursor.
type ReviewDiscoveryPosition struct {
	ID           string
	RoleRevision int64
}

// ReviewDiscoverySnapshot holds one authorized database observation.
type ReviewDiscoverySnapshot struct {
	Items []ReviewDiscoveryItem
	Next  ReviewDiscoveryPosition
}

// ReviewDiscoveryRepository reads one page under current reviewer authority.
type ReviewDiscoveryRepository interface {
	DiscoverReviewAssignments(context.Context, identity.ProblemActor, ReviewDiscoveryPosition) (ReviewDiscoverySnapshot, error)
}

// ReviewDiscoveryPage is the subject-scoped list response.
type ReviewDiscoveryPage struct {
	Assignments []ReviewDiscoveryItem `json:"assignments"`
	NextCursor  string                `json:"next_cursor"`
}

// ReviewDiscovery authorizes listing by ordinary session without reusing the
// single-item reader capability. Opening an item authorizes it separately.
type ReviewDiscovery struct {
	sessions SessionReader
	repo     ReviewDiscoveryRepository
	cursor   cipher.AEAD
}

// NewReviewDiscovery constructs the scoped assignment list capability.
func NewReviewDiscovery(sessions SessionReader, repo ReviewDiscoveryRepository, cursor cipher.AEAD) (*ReviewDiscovery, error) {
	if sessions == nil || repo == nil || cursor == nil || cursor.NonceSize() != 0 || cursor.Overhead() != 28 {
		return nil, ErrInvalid
	}
	return &ReviewDiscovery{sessions: sessions, repo: repo, cursor: cursor}, nil
}

// ListForSubject lists one page after a fresh observation of the expected subject.
func (s *ReviewDiscovery) ListForSubject(ctx context.Context, credential redisstore.Credential, expected, cursor string) (ReviewDiscoveryPage, error) {
	var zero ReviewDiscoveryPage
	if s == nil || ctx == nil || !identity.ValidID(expected) {
		return zero, ErrInvalid
	}
	actor, err := observeActor(ctx, s.sessions, credential)
	if err != nil {
		return zero, err
	}
	if actor.SubjectID != expected {
		return zero, ErrSubjectChanged
	}
	after, err := openReviewCursor(s.cursor, cursor, credential, actor)
	if err != nil {
		return zero, err
	}
	page, err := s.repo.DiscoverReviewAssignments(ctx, actor, after)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if !validDiscoverySnapshot(page, after) {
		return zero, ErrStore
	}
	next := sealReviewCursor(s.cursor, page.Next, credential, actor)
	return ReviewDiscoveryPage{Assignments: page.Items, NextCursor: next}, nil
}

func validDiscoverySnapshot(page ReviewDiscoverySnapshot, after ReviewDiscoveryPosition) bool {
	if page.Items == nil || len(page.Items) > ReviewDiscoveryLimit {
		return false
	}
	last := after.ID
	for _, item := range page.Items {
		if !item.Valid() || item.ReviewID <= last {
			return false
		}
		last = item.ReviewID
	}
	if page.Next.ID == "" {
		return page.Next.RoleRevision == 0
	}
	return identity.ValidID(page.Next.ID) && page.Next.ID > after.ID && page.Next.ID >= last &&
		page.Next.RoleRevision > 0 && (after.RoleRevision == 0 || after.RoleRevision == page.Next.RoleRevision)
}
