package postgres

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
)

var _ problemreference.PlanRepository = (*Store)(nil)

// PersistExpectedPlan rechecks live authority and serializes with cancellation
// on the request row. It never changes queue state or rewrites a recorded plan.
func (s *Store) PersistExpectedPlan(ctx context.Context, actor identity.ProblemActor, prepared problemreference.PreparedPlan) (problemreference.ExpectedPlan, error) {
	original, plan := prepared.Values()
	if ctx == nil || original.Owner != actor.SubjectID || original.Authorization.Actor != actor ||
		!identity.ValidID(original.ID) || !identity.ValidID(original.DraftID) || !problem.ValidKey(original.RequestKey) ||
		original.State != problemreference.Queued || original.CancelledAt != nil ||
		!original.Specification.Valid() || !plan.ValidFor(original.Specification) {
		return problemreference.ExpectedPlan{}, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, authority, err := s.begin(ctx, actor)
	if err != nil {
		return problemreference.ExpectedPlan{}, err
	}
	defer rollback(tx)
	if authority != original.Authorization {
		return problemreference.ExpectedPlan{}, problemreference.ErrAuthorityChanged
	}
	draft, err := s.access.LockOwnDraftIdentity(ctx, tx, actor, original.DraftID)
	if err != nil {
		return problemreference.ExpectedPlan{}, err
	}
	i := original.Specification.Identity
	if draft.ID != original.DraftID || draft.Owner != actor.SubjectID || draft.SourceID != i.SourceID ||
		draft.Version != i.Version || draft.ImportRevision != i.ImportRevision {
		return problemreference.ExpectedPlan{}, problemreference.ErrSourceChanged
	}
	// Preparation does not affect admission counts: it takes the same request
	// lock as Cancel without acquiring or later waiting on the policy row.
	q := referencedb.New(tx)
	row, err := q.ReferenceRequestForUpdate(ctx, referencedb.ReferenceRequestForUpdateParams{Owner: actor.SubjectID, RequestKey: original.RequestKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return problemreference.ExpectedPlan{}, problemreference.ErrNotFound
	}
	if err != nil {
		return problemreference.ExpectedPlan{}, storageError{err}
	}
	current, err := readRequest(row)
	if err != nil {
		return problemreference.ExpectedPlan{}, err
	}
	if current.ID != original.ID || current.DraftID != original.DraftID || current.Owner != original.Owner ||
		current.RequestKey != original.RequestKey || !current.CreatedAt.Equal(original.CreatedAt) {
		return problemreference.ExpectedPlan{}, problemreference.ErrConflict
	}
	if current.State != problemreference.Queued {
		return problemreference.ExpectedPlan{}, problemreference.ErrNotQueued
	}
	if current.Authorization != authority {
		return problemreference.ExpectedPlan{}, problemreference.ErrAuthorityChanged
	}
	want, got := original.Specification, current.Specification
	if want.Identity != got.Identity || want.Protocol != got.Protocol || want.ProfileSHA256 != got.ProfileSHA256 ||
		!bytes.Equal(want.ProfileJSON, got.ProfileJSON) {
		return problemreference.ExpectedPlan{}, problemreference.ErrSourceChanged
	}
	previous, err := loadExpectedPlan(ctx, q, current)
	wrote := errors.Is(err, problemreference.ErrNotFound)
	switch {
	case wrote:
		err = q.InsertReferenceExpectedPlan(ctx, referencedb.InsertReferenceExpectedPlanParams{
			RequestID: current.ID, PlanEncoding: plan.Encoding, PlanSha256: plan.SHA256,
		})
		if err != nil {
			return problemreference.ExpectedPlan{}, storageError{err}
		}
	case err != nil:
		return problemreference.ExpectedPlan{}, err
	case previous.SHA256 != plan.SHA256 || !bytes.Equal(previous.Encoding, plan.Encoding):
		return problemreference.ExpectedPlan{}, problemreference.ErrPlanConflict
	}
	if err := commitTx(ctx, tx, wrote); err != nil {
		return problemreference.ExpectedPlan{}, err
	}
	if err := ctx.Err(); err != nil {
		if wrote {
			err = errors.Join(problemreference.ErrCommitUnknown, err)
		}
		return problemreference.ExpectedPlan{}, err
	}
	return plan, nil
}

// ExpectedPlan observes a prior write by original owner/key under current
// author authority. History remains readable after cancellation and regrant.
func (s *Store) ExpectedPlan(ctx context.Context, actor identity.ProblemActor, key string) (problemreference.ExpectedPlan, error) {
	if ctx == nil || !problem.ValidKey(key) {
		return problemreference.ExpectedPlan{}, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, _, err := s.begin(ctx, actor)
	if err != nil {
		return problemreference.ExpectedPlan{}, err
	}
	defer rollback(tx)
	q := referencedb.New(tx)
	current, found, err := existing(ctx, q, actor, key, "")
	if err != nil {
		return problemreference.ExpectedPlan{}, err
	}
	if !found {
		return problemreference.ExpectedPlan{}, problemreference.ErrNotFound
	}
	result, err := loadExpectedPlan(ctx, q, current)
	if err != nil {
		return problemreference.ExpectedPlan{}, err
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return problemreference.ExpectedPlan{}, err
	}
	if err := ctx.Err(); err != nil {
		return problemreference.ExpectedPlan{}, err
	}
	return result, nil
}

func loadExpectedPlan(ctx context.Context, q *referencedb.Queries, request problemreference.Request) (problemreference.ExpectedPlan, error) {
	row, err := q.ReferenceExpectedPlan(ctx, request.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return problemreference.ExpectedPlan{}, problemreference.ErrNotFound
	}
	if err != nil {
		return problemreference.ExpectedPlan{}, storageError{err}
	}
	result := problemreference.ExpectedPlan{Encoding: row.PlanEncoding, SHA256: row.PlanSha256}
	if !result.ValidFor(request.Specification) {
		return problemreference.ExpectedPlan{}, storageError{problemreference.ErrInvalid}
	}
	return result, nil
}
