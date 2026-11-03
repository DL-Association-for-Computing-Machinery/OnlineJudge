package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres/problemdb"
	"github.com/jackc/pgx/v5"
)

// Receipt uses the existing owner/key index and current author transaction. It
// never queries materials or the current configured import candidates.
func (s *Store) Receipt(ctx context.Context, actor identity.ProblemActor, key string) (problem.ImportReceipt, error) {
	if ctx == nil || !problem.ValidKey(key) {
		return problem.ImportReceipt{}, problem.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.begin(ctx, actor)
	if err != nil {
		return problem.ImportReceipt{}, err
	}
	defer rollback(tx)
	row, err := problemdb.New(tx).DraftByKeyForShare(ctx, problemdb.DraftByKeyForShareParams{Owner: actor.SubjectID, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return problem.ImportReceipt{}, problem.ErrNotFound
	}
	if err != nil {
		return problem.ImportReceipt{}, storageError{err}
	}
	if !row.CreatedAt.Valid || row.State != "private-draft" || row.ValidationState != "not-run" || !problem.ValidImportRevision(row.RequestSha256) {
		return problem.ImportReceipt{}, storageError{problem.ErrInvalid}
	}
	result := problem.ImportReceipt{DraftID: row.ID, RequestKey: row.RequestKey, SourceID: row.SourceID, Version: row.ContentVersion,
		ImportRevision: row.RequestSha256, State: row.State, ValidationState: row.ValidationState, CreatedAt: row.CreatedAt.Time}
	if err = tx.Commit(ctx); err != nil {
		return problem.ImportReceipt{}, storageError{err}
	}
	return result, nil
}
