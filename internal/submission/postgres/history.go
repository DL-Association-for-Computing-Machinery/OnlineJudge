package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres/submissiondb"
)

const historyPageSize = 20

// HistoryPosition orders the next page by the last visible timestamp and ID.
type HistoryPosition struct {
	CreatedAt time.Time
	ID        string
}

// HistoryPage contains summaries only. Next is nil at the end of the list.
type HistoryPage struct {
	Items []submission.Receipt
	Next  *HistoryPosition
}

// ListHistory reads the authenticated caller's subject and scope, which must be
// supplied by a trusted caller. Pages are independent snapshots, so later
// inserts or state changes need a fresh first page to be observed reliably.
func (s *Store) ListHistory(ctx context.Context, subject, scope string, after *HistoryPosition) (HistoryPage, error) {
	if s == nil || s.pool == nil || ctx == nil || len(subject) == 0 || len(subject) > 128 ||
		len(scope) == 0 || len(scope) > 128 || (after != nil &&
		(after.CreatedAt.IsZero() || after.CreatedAt.Nanosecond()%1000 != 0 || !validHistoryID(after.ID))) {
		return HistoryPage{}, submission.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	q := submissiondb.New(s.pool)
	var rows []submissiondb.ListHistoryFirstRow
	if after == nil {
		var err error
		rows, err = q.ListHistoryFirst(ctx, submissiondb.ListHistoryFirstParams{SubjectID: subject, ContextID: scope})
		if err != nil {
			return HistoryPage{}, err
		}
	} else {
		later, err := q.ListHistoryAfter(ctx, submissiondb.ListHistoryAfterParams{SubjectID: subject, ContextID: scope,
			CursorTime: pgtype.Timestamptz{Time: after.CreatedAt, Valid: true}, CursorID: after.ID})
		if err != nil {
			return HistoryPage{}, err
		}
		rows = make([]submissiondb.ListHistoryFirstRow, 0, len(later))
		for _, row := range later {
			rows = append(rows, submissiondb.ListHistoryFirstRow(row))
		}
	}
	page := HistoryPage{Items: make([]submission.Receipt, 0, min(len(rows), historyPageSize))}
	for _, row := range rows[:min(len(rows), historyPageSize)] {
		if !row.CreatedAt.Valid || row.CreatedAt.InfinityModifier != pgtype.Finite || !validHistoryID(row.ID) {
			return HistoryPage{}, errors.New("invalid stored submission history")
		}
		page.Items = append(page.Items, receipt(submissiondb.GetReceiptRow{ID: row.ID, MaterialVersion: row.MaterialVersion,
			Language: row.Language, CreatedAt: row.CreatedAt, State: row.State, Verdict: row.Verdict}))
	}
	if len(rows) > historyPageSize {
		last := page.Items[historyPageSize-1]
		page.Next = &HistoryPosition{CreatedAt: last.CreatedAt, ID: last.ID}
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
