package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres/submissiondb"
)

const historyPageSize = 20

// ListHistory reads the authenticated caller's subject and scope, which must be
// supplied by a trusted caller. Pages are independent snapshots, so later
// inserts or state changes need a fresh first page to be observed reliably.
func (s *Store) ListHistory(ctx context.Context, subject, scope string, after *submission.HistoryPosition) (submission.HistoryPage, error) {
	if s == nil || s.pool == nil || ctx == nil || len(subject) == 0 || len(subject) > 128 ||
		len(scope) == 0 || len(scope) > 128 || !validHistoryPosition(after) {
		return submission.HistoryPage{}, submission.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return listHistory(ctx, submissiondb.New(s.pool), subject, scope, after)
}

// ListTrainingHistory authorizes and reads an ordinary training account's page
// in the same transaction. The actor must come from a fresh ordinary session.
func ListTrainingHistory(ctx context.Context, pool *pgxpool.Pool, actor identity.TrainingActor, after *submission.HistoryPosition) (submission.HistoryPage, error) {
	if ctx == nil || pool == nil || !actor.Valid() || !validHistoryPosition(after) {
		return submission.HistoryPage{}, submission.ErrInvalid
	}
	return readTraining(ctx, pool, actor, func(ctx context.Context, q *submissiondb.Queries) (submission.HistoryPage, error) {
		return listHistory(ctx, q, actor.SubjectID, submission.PersonalTrainingScope, after)
	})
}

// ReadTrainingDetail checks the current training account and reads only the
// caller's fixed personal training scope within the same transaction.
func ReadTrainingDetail(ctx context.Context, pool *pgxpool.Pool, actor identity.TrainingActor, id string) (submission.Detail, error) {
	if ctx == nil || pool == nil || !actor.Valid() || !validHistoryID(id) {
		return submission.Detail{}, submission.ErrInvalid
	}
	return readTraining(ctx, pool, actor, func(ctx context.Context, q *submissiondb.Queries) (submission.Detail, error) {
		row, err := q.GetTrainingSubmissionDetail(ctx, submissiondb.GetTrainingSubmissionDetailParams{
			ID: id, SubjectID: actor.SubjectID, ContextID: submission.PersonalTrainingScope,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return submission.Detail{}, submission.ErrNotFound
		}
		if err != nil {
			return submission.Detail{}, err
		}
		if !row.CreatedAt.Valid || row.CreatedAt.InfinityModifier != pgtype.Finite {
			return submission.Detail{}, errors.New("invalid stored submission timestamp")
		}
		return submission.Detail{Receipt: receipt(submissiondb.GetReceiptRow{
			ID: row.ID, MaterialVersion: row.MaterialVersion, Language: row.Language,
			CreatedAt: row.CreatedAt, State: row.State, Verdict: row.Verdict,
		}), Source: row.SourceCode}, nil
	})
}

func readTraining[T any](ctx context.Context, pool *pgxpool.Pool, actor identity.TrainingActor, read func(context.Context, *submissiondb.Queries) (T, error)) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, err
	}
	defer rollback(tx)
	if err := identitypg.LockTrainingAccount(ctx, tx, actor); err != nil {
		return zero, err
	}
	result, err := read(ctx, submissiondb.New(tx))
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, err
	}
	return result, nil
}

func validHistoryPosition(after *submission.HistoryPosition) bool {
	return after == nil || (!after.CreatedAt.IsZero() && after.CreatedAt.Nanosecond()%1000 == 0 && validHistoryID(after.ID))
}

func listHistory(ctx context.Context, q *submissiondb.Queries, subject, scope string, after *submission.HistoryPosition) (submission.HistoryPage, error) {
	var rows []submissiondb.ListHistoryFirstRow
	if after == nil {
		var err error
		rows, err = q.ListHistoryFirst(ctx, submissiondb.ListHistoryFirstParams{SubjectID: subject, ContextID: scope})
		if err != nil {
			return submission.HistoryPage{}, err
		}
	} else {
		later, err := q.ListHistoryAfter(ctx, submissiondb.ListHistoryAfterParams{SubjectID: subject, ContextID: scope,
			CursorTime: pgtype.Timestamptz{Time: after.CreatedAt, Valid: true}, CursorID: after.ID})
		if err != nil {
			return submission.HistoryPage{}, err
		}
		rows = make([]submissiondb.ListHistoryFirstRow, 0, len(later))
		for _, row := range later {
			rows = append(rows, submissiondb.ListHistoryFirstRow(row))
		}
	}
	page := submission.HistoryPage{Items: make([]submission.Receipt, 0, min(len(rows), historyPageSize))}
	for _, row := range rows[:min(len(rows), historyPageSize)] {
		if !row.CreatedAt.Valid || row.CreatedAt.InfinityModifier != pgtype.Finite || !validHistoryID(row.ID) {
			return submission.HistoryPage{}, errors.New("invalid stored submission history")
		}
		page.Items = append(page.Items, receipt(submissiondb.GetReceiptRow{ID: row.ID, MaterialVersion: row.MaterialVersion,
			Language: row.Language, CreatedAt: row.CreatedAt, State: row.State, Verdict: row.Verdict}))
	}
	if len(rows) > historyPageSize {
		last := page.Items[historyPageSize-1]
		page.Next = &submission.HistoryPosition{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func validHistoryID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for i := range len(id) {
		c := id[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
