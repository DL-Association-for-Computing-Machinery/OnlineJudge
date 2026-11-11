package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
)

// AuthorReport reads one immutable terminal report through its original
// owner/key. Current author and draft ownership locks last through projection.
func (s *Store) AuthorReport(ctx context.Context, actor identity.ProblemActor, key string) (problemreference.AuthorReport, error) {
	var zero problemreference.AuthorReport
	if ctx == nil || !problem.ValidKey(key) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, _, err := s.begin(ctx, actor)
	if err != nil {
		return zero, err
	}
	defer rollback(tx)
	q := referencedb.New(tx)
	r, found, err := existing(ctx, q, actor, key, "")
	if err != nil {
		return zero, err
	}
	if !found || r.State != problemreference.Reported {
		return zero, problemreference.ErrNotFound
	}
	draft, err := s.access.LockOwnDraftIdentity(ctx, tx, actor, r.DraftID)
	if errors.Is(err, problem.ErrNotFound) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, err
	}
	i := r.Specification.Identity
	if draft.ID != r.DraftID || draft.Owner != actor.SubjectID || draft.SourceID != i.SourceID || draft.Version != i.Version ||
		draft.ImportRevision != i.ImportRevision {
		return zero, problemreference.ErrNotFound
	}
	expected, err := loadExpectedPlan(ctx, q, r)
	if err != nil {
		return zero, storageError{err}
	}
	saved, err := q.ReferenceWorkerReport(ctx, r.ID)
	if err != nil {
		return zero, storageError{err}
	}
	view, err := problemreference.ProjectAuthorReport(r, expected, saved.PlanSha256, saved.EvidenceSha256, saved.Evidence)
	if err != nil {
		return zero, storageError{err}
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return view, nil
}
