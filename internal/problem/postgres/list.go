package postgres

import (
	"context"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres/problemdb"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListOwn reads bounded owner-scoped summaries under current author locks.
func (s *Store) ListOwn(ctx context.Context, actor identity.ProblemActor, after *problem.ListPosition) ([]problem.ImportReceipt, error) {
	if ctx == nil || (after != nil && !after.Valid()) {
		return nil, problem.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.begin(ctx, actor)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	q := problemdb.New(tx)
	rows := []problemdb.ListOwnDraftsFirstRow{}
	if after == nil {
		rows, err = q.ListOwnDraftsFirst(ctx, problemdb.ListOwnDraftsFirstParams{Owner: actor.SubjectID, RowLimit: problem.OwnDraftPageSize + 1})
	} else {
		var later []problemdb.ListOwnDraftsAfterRow
		later, err = q.ListOwnDraftsAfter(ctx, problemdb.ListOwnDraftsAfterParams{Owner: actor.SubjectID, CreatedAt: pgtype.Timestamptz{Time: after.CreatedAt, Valid: true}, ID: after.ID, RowLimit: problem.OwnDraftPageSize + 1})
		for _, row := range later {
			rows = append(rows, problemdb.ListOwnDraftsFirstRow(row))
		}
	}
	if err != nil {
		return nil, storageError{err}
	}
	if len(rows) > problem.OwnDraftPageSize+1 {
		return nil, storageError{problem.ErrInvalid}
	}
	result := make([]problem.ImportReceipt, 0, len(rows))
	for _, row := range rows {
		if !row.CreatedAt.Valid || row.CreatedAt.InfinityModifier != pgtype.Finite {
			return nil, storageError{problem.ErrInvalid}
		}
		r := problem.ImportReceipt{DraftID: row.ID, RequestKey: row.RequestKey, SourceID: row.SourceID, Version: row.ContentVersion, ImportRevision: row.RequestSha256, State: row.State, ValidationState: row.ValidationState, CreatedAt: row.CreatedAt.Time.UTC()}
		if !r.Valid() {
			return nil, storageError{problem.ErrInvalid}
		}
		result = append(result, r)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, storageError{err}
	}
	return result, nil
}
