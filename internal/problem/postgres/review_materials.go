package postgres

import (
	"context"
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres/problemdb"
	"github.com/jackc/pgx/v5"
)

// ReadBoundReviewDraft reads immutable metadata in a caller-owned transaction.
// The caller must have locked live review authority and the item that binds the
// server-selected draft and owner before calling. It grants no permission.
func ReadBoundReviewDraft(ctx context.Context, tx pgx.Tx, id, owner string) (problem.Draft, error) {
	if ctx == nil || tx == nil || !identity.ValidID(id) || !identity.ValidID(owner) {
		return problem.Draft{}, problem.ErrInvalid
	}
	q := problemdb.New(tx)
	row, err := q.DraftByOwner(ctx, problemdb.DraftByOwnerParams{ID: id, Owner: owner})
	if errors.Is(err, pgx.ErrNoRows) {
		return problem.Draft{}, problem.ErrNotFound
	}
	if err != nil {
		return problem.Draft{}, storageError{err}
	}
	return readDraft(ctx, q, row)
}
