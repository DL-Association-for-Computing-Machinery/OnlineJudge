package postgres

import (
	"context"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
)

// DiscoverReviewAssignments rechecks reviewer authority before reading one
// statement snapshot. A later item read must still reauthorize that assignment.
func (s *ReviewStore) DiscoverReviewAssignments(ctx context.Context, actor identity.ProblemActor, after problemreference.ReviewDiscoveryPosition) (problemreference.ReviewDiscoverySnapshot, error) {
	var zero problemreference.ReviewDiscoverySnapshot
	if s == nil || s.pool == nil || s.auth == nil || ctx == nil || !actor.Valid() {
		return zero, problemreference.ErrInvalid
	}
	if (after.ID == "" && after.RoleRevision != 0) ||
		(after.ID != "" && (!identity.ValidID(after.ID) || after.RoleRevision <= 0)) {
		return zero, problemreference.ErrReviewCursor
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, storageError{err}
	}
	defer rollback(tx)
	role, err := s.auth.LockReviewer(ctx, tx, actor)
	if err != nil {
		return zero, err
	}
	if role <= 0 {
		return zero, storageError{problemreference.ErrInvalid}
	}
	if after.RoleRevision != 0 && after.RoleRevision != role {
		return zero, problemreference.ErrReviewCursor
	}
	rows, err := referencedb.New(tx).DiscoverReviewAssignments(ctx, referencedb.DiscoverReviewAssignmentsParams{
		ReviewerID: actor.SubjectID, RoleRevision: role, AfterID: after.ID,
	})
	if err != nil {
		return zero, storageError{err}
	}
	page, err := projectReviewDiscovery(rows, role, after.ID)
	if err != nil {
		return zero, err
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return page, nil
}

func projectReviewDiscovery(rows []referencedb.DiscoverReviewAssignmentsRow, role int64, after string) (problemreference.ReviewDiscoverySnapshot, error) {
	const limit = problemreference.ReviewDiscoveryLimit
	var zero problemreference.ReviewDiscoverySnapshot
	if len(rows) > limit+1 {
		return zero, storageError{problemreference.ErrInvalid}
	}
	page := problemreference.ReviewDiscoverySnapshot{Items: make([]problemreference.ReviewDiscoveryItem, 0, min(limit, len(rows)))}
	last := after
	for n, row := range rows {
		if !identity.ValidID(row.ReviewID) || row.ReviewID <= last || int(row.Position) != n+1 {
			return zero, storageError{problemreference.ErrInvalid}
		}
		if n == limit {
			page.Next = problemreference.ReviewDiscoveryPosition{ID: last, RoleRevision: role}
			break
		}
		last = row.ReviewID
		if row.IsCurrent {
			item := problemreference.ReviewDiscoveryItem{ReviewID: row.ReviewID, SourceID: row.SourceID,
				ImportRevision: row.ImportRevision, AssignmentRevision: row.AssignmentRevision}
			if !item.Valid() {
				return zero, storageError{problemreference.ErrInvalid}
			}
			page.Items = append(page.Items, item)
		}
	}
	return page, nil
}
