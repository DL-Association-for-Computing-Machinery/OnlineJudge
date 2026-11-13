package postgres

import (
	"context"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres/submissiondb"
)

var (
	ErrRejudgeOperator = errors.New("rejudge requires an authorized database login")
	ErrRejudgeState    = errors.New("submission is not finished in the expected generation")
	ErrRejudgeUnknown  = errors.New("rejudge outcome unknown; read the original request key")
	rejudgeID          = regexp.MustCompile(`^[a-f0-9]{32}$`)
	rejudgeDigest      = regexp.MustCompile(`^[a-f0-9]{64}$`)
	rejudgeVersion     = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
)

// RejudgeIntent selects a finished submission without allowing new source or material.
type RejudgeIntent struct {
	ID, Key, Version, Digest, Reason string
	PreviousGeneration               int64
}

// Valid checks the maintenance request before any database or file access.
func (i RejudgeIntent) Valid() bool {
	return rejudgeID.MatchString(i.ID) && submission.ValidKey(i.Key) && rejudgeVersion.MatchString(i.Version) &&
		rejudgeDigest.MatchString(i.Digest) && i.PreviousGeneration > 0 && i.PreviousGeneration < math.MaxInt64 &&
		len(i.Reason) > 0 && len(i.Reason) <= 240 && utf8.ValidString(i.Reason) && strings.TrimSpace(i.Reason) == i.Reason &&
		!strings.ContainsFunc(i.Reason, unicode.IsControl)
}

// RejudgeReceipt is the immutable audit of a request, not the current judgment.
type RejudgeReceipt struct {
	Operator     string    `json:"operator"`
	Key          string    `json:"request_key"`
	SubmissionID string    `json:"submission_id"`
	Generation   int64     `json:"judge_generation"`
	Version      string    `json:"material_version"`
	Manifest     string    `json:"manifest_sha256"`
	Reason       string    `json:"reason"`
	RequestedAt  time.Time `json:"requested_at"`
}

func rejudgeOperator(ctx context.Context, q *submissiondb.Queries, database string) (string, error) {
	row, err := q.RejudgeSession(ctx)
	if err != nil {
		return "", err
	}
	if row.DatabaseName != database || database == "" {
		return "", ErrRestoreDatabase
	}
	if !row.Allowed {
		return "", ErrRejudgeOperator
	}
	return row.Operator, nil
}

func rejudgeReceipt(row submissiondb.SubmissionRejudge) RejudgeReceipt {
	return RejudgeReceipt{Operator: row.Operator, Key: row.RequestKey, SubmissionID: row.SubmissionID,
		Generation: row.PreviousGeneration + 1, Version: row.MaterialVersion, Manifest: row.ManifestSha256,
		Reason: row.Reason, RequestedAt: row.RequestedAt.Time.UTC()}
}

// ReadRejudge resolves an ambiguous outcome under the original authenticated login.
func ReadRejudge(ctx context.Context, pool *pgxpool.Pool, database, key string) (RejudgeReceipt, error) {
	if !submission.ValidKey(key) {
		return RejudgeReceipt{}, submission.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return RejudgeReceipt{}, err
	}
	defer conn.Release()
	q := submissiondb.New(conn)
	operator, err := rejudgeOperator(ctx, q, database)
	if err != nil {
		return RejudgeReceipt{}, err
	}
	row, err := q.ReadRejudge(ctx, submissiondb.ReadRejudgeParams{Operator: operator, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return RejudgeReceipt{}, submission.ErrNotFound
	}
	if err != nil {
		return RejudgeReceipt{}, err
	}
	return rejudgeReceipt(row), nil
}

// Rejudge verifies immutable files outside the scheduling transaction, then adds
// one fair queue entry and its audit atomically. verify must check each object's
// digest in the same immutable store that the controller uses.
func Rejudge(ctx context.Context, pool *pgxpool.Pool, database string, i RejudgeIntent,
	verify func(context.Context, string, string, []byte) error) (RejudgeReceipt, error) {
	var zero RejudgeReceipt
	if !i.Valid() || verify == nil {
		return zero, submission.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return zero, err
	}
	defer conn.Release()
	q := submissiondb.New(conn)
	operator, err := rejudgeOperator(ctx, q, database)
	if err != nil {
		return zero, err
	}
	find := func(q *submissiondb.Queries) (RejudgeReceipt, error) {
		row, err := q.ReadRejudge(ctx, submissiondb.ReadRejudgeParams{Operator: operator, RequestKey: i.Key})
		if err != nil {
			return zero, err
		}
		if row.SubmissionID != i.ID || row.PreviousGeneration != i.PreviousGeneration || row.MaterialVersion != i.Version ||
			row.ManifestSha256 != i.Digest || row.Reason != i.Reason {
			return zero, submission.ErrConflict
		}
		return rejudgeReceipt(row), nil
	}
	if receipt, err := find(q); !errors.Is(err, pgx.ErrNoRows) {
		return receipt, err
	}
	raw, err := readRejudgeMaterial(ctx, q, i)
	if err != nil {
		return zero, err
	}
	if err := verify(ctx, i.Version, i.Digest, raw); err != nil {
		return zero, submission.ErrMaterial
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, err
	}
	defer rollback(tx)
	q = submissiondb.New(tx)
	// Serialize admissions before taking the version clock, then the task.
	// All successful writers use this lock, including same-key requests.
	limits, err := lockBacklog(ctx, q)
	if err != nil {
		return zero, err
	}
	current, err := rejudgeOperator(ctx, q, database)
	if err != nil {
		return zero, err
	}
	if current != operator {
		return zero, ErrRejudgeOperator
	}
	if receipt, err := find(q); !errors.Is(err, pgx.ErrNoRows) {
		return receipt, err
	}
	if _, err := readRejudgeMaterial(ctx, q, i); err != nil {
		return zero, err
	}
	floor, err := q.LockTrainingClock(ctx, i.Version)
	if err != nil {
		return zero, err
	}
	task, err := q.LockRejudgeTarget(ctx, i.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, submission.ErrNotFound
	}
	if err != nil {
		return zero, err
	}
	if task.MaterialVersion != i.Version || task.ManifestSha256 != i.Digest {
		return zero, submission.ErrMaterial
	}
	if task.ContextID != submission.PersonalTrainingScope || task.State != "finished" || !task.Completed || task.JudgeGeneration != i.PreviousGeneration {
		return zero, ErrRejudgeState
	}
	if err := checkBacklog(ctx, q, limits, task.SubjectID); err != nil {
		return zero, err
	}
	pending, err := q.MaxQueuedTrainingRank(ctx, submissiondb.MaxQueuedTrainingRankParams{VersionID: i.Version, SubjectID: task.SubjectID})
	if err != nil {
		return zero, err
	}
	rank := max(floor, pending)
	if rank == math.MaxInt64 {
		return zero, ErrRejudgeState
	}
	if current, err := rejudgeOperator(ctx, q, database); err != nil || current != operator {
		return zero, errors.Join(ErrRejudgeOperator, err)
	}
	changed, err := q.EnqueueRejudge(ctx, submissiondb.EnqueueRejudgeParams{
		SubmissionID: i.ID, PreviousGeneration: i.PreviousGeneration, VersionID: i.Version, Rank: rank + 1})
	if err != nil {
		return zero, err
	}
	if changed != 1 {
		return zero, ErrRejudgeState
	}
	row, err := q.InsertRejudge(ctx, submissiondb.InsertRejudgeParams{Operator: operator, RequestKey: i.Key, SubmissionID: i.ID,
		PreviousGeneration: i.PreviousGeneration, MaterialVersion: i.Version, ManifestSha256: i.Digest, Reason: i.Reason})
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, ErrRejudgeUnknown
	}
	return rejudgeReceipt(row), nil
}

func readRejudgeMaterial(ctx context.Context, q *submissiondb.Queries, i RejudgeIntent) ([]byte, error) {
	row, err := q.GetMaterial(ctx, i.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, submission.ErrMaterial
	}
	if err != nil {
		return nil, err
	}
	raw, sum, err := CanonicalManifest(row.Manifest)
	if err != nil || row.ManifestSha256 != i.Digest || sum != i.Digest {
		return nil, submission.ErrMaterial
	}
	return raw, nil
}
