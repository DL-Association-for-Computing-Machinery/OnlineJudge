package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres/submissiondb"
)

// BacklogLimits counts queued and running submissions, across contexts.
// Values must be explicitly chosen; these are not production capacity targets.
type BacklogLimits struct {
	Global  int64
	Subject int64
}

// Valid rejects unbounded, negative or inconsistent admission configuration.
func (l BacklogLimits) Valid() bool { return l.Global > 0 && l.Subject > 0 && l.Subject <= l.Global }

// ConfigureBacklog atomically changes the database's sole admission policy.
// Only an explicit administrator operation calls this; API startup is read-only.
// Lowering limits never removes accepted work or changes historical receipts.
func ConfigureBacklog(ctx context.Context, pool *pgxpool.Pool, limits BacklogLimits) error {
	if !limits.Valid() {
		return submission.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return submissiondb.New(pool).SetBacklogPolicy(ctx, submissiondb.SetBacklogPolicyParams{
		GlobalLimit: limits.Global, SubjectLimit: limits.Subject,
	})
}

func lockBacklog(ctx context.Context, q *submissiondb.Queries) (BacklogLimits, error) {
	row, err := q.LockBacklogPolicy(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return BacklogLimits{}, submission.ErrAdmissionPolicy
	}
	if err != nil {
		return BacklogLimits{}, err
	}
	limits := BacklogLimits{Global: row.GlobalLimit, Subject: row.SubjectLimit}
	if !limits.Valid() {
		return BacklogLimits{}, submission.ErrAdmissionPolicy
	}
	return limits, nil
}

func checkBacklog(ctx context.Context, q *submissiondb.Queries, limits BacklogLimits, subject string) error {
	row, err := q.CountBacklog(ctx, submissiondb.CountBacklogParams{Maximum: limits.Global, Subject: subject})
	if err != nil {
		return err
	}
	if row.Total >= limits.Global || row.Personal >= limits.Subject {
		return submission.ErrBacklog
	}
	return nil
}

func acceptedReceipt(ctx context.Context, q *submissiondb.Queries, key submissiondb.GetReceiptParams, fingerprint string) (submission.Receipt, bool, error) {
	row, err := q.GetReceipt(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return submission.Receipt{}, false, nil
	}
	if err != nil {
		return submission.Receipt{}, false, err
	}
	if row.RequestSha256 != fingerprint {
		return submission.Receipt{}, true, submission.ErrConflict
	}
	return receipt(row), true, nil
}
