package postgres

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres/submissiondb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Claim atomically takes one queued task without retaining any execution-time lock.
// M0 uses FIFO; user fairness and shared quotas remain required before M1.
func (s *Store) Claim(ctx context.Context, node submission.NodeIdentity) (submission.Lease, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return submission.Lease{}, err
	}
	defer rollback(tx)
	q := submissiondb.New(tx)
	record, err := authorizeNode(ctx, q, node)
	if err != nil {
		return submission.Lease{}, err
	}
	if err := s.checkMaterial(ctx, q); err != nil {
		return submission.Lease{}, err
	}
	count, err := q.CountNodeLeases(ctx, node.ID)
	if err != nil {
		return submission.Lease{}, err
	}
	if count >= int64(record.Capacity) {
		return submission.Lease{}, submission.ErrNoTask
	}
	id, err := q.ClaimCandidate(ctx, s.version)
	if errors.Is(err, pgx.ErrNoRows) {
		return submission.Lease{}, submission.ErrNoTask
	}
	if err != nil {
		return submission.Lease{}, err
	}
	task, err := q.BeginAttempt(ctx, id)
	if err != nil {
		return submission.Lease{}, err
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return submission.Lease{}, err
	}
	token := hex.EncodeToString(random)
	until, err := q.InsertAttempt(ctx, submissiondb.InsertAttemptParams{Token: token, SubmissionID: id,
		JudgeGeneration: task.JudgeGeneration, RecoveryGeneration: task.RecoveryGeneration,
		AttemptNumber: task.AttemptNumber, NodeID: node.ID, CredentialVersion: node.CredentialVersion, LeaseSeconds: 30})
	if err != nil {
		return submission.Lease{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return submission.Lease{}, err
	}
	return submission.Lease{Token: token, SubmissionID: id, JudgeGeneration: task.JudgeGeneration,
		RecoveryGeneration: task.RecoveryGeneration, AttemptNumber: task.AttemptNumber, Until: until.Time}, nil
}

// Renew refuses expired attempts even before the recovery scanner has run.
func (s *Store) Renew(ctx context.Context, node submission.NodeIdentity, token string) (time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer rollback(tx)
	q := submissiondb.New(tx)
	if _, err := authorizeNode(ctx, q, node); err != nil {
		return time.Time{}, err
	}
	row, err := q.LockAttempt(ctx, submissiondb.LockAttemptParams{Token: token, NodeID: node.ID, CredentialVersion: node.CredentialVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, submission.ErrLease
	}
	if err != nil {
		return time.Time{}, err
	}
	if row.State != "leased" || row.TaskState != "running" || row.AttemptNumber != row.CurrentAttempt {
		return time.Time{}, submission.ErrLease
	}
	until, err := q.RenewAttempt(ctx, submissiondb.RenewAttemptParams{Token: token, LeaseSeconds: 30})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, submission.ErrLease
	}
	if err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return time.Time{}, err
	}
	return until.Time, nil
}

// Report conditionally finalizes a valid attempt. Exact repeats are acknowledged;
// conflicting evidence is refused, including after a successful response was lost.
func (s *Store) Report(ctx context.Context, node submission.NodeIdentity, token string, result submission.Result) error {
	valid := false
	for _, v := range []string{"AC", "WA", "CE", "RE", "TLE", "MLE", "OLE", "SE"} {
		valid = valid || result.Verdict == v
	}
	raw, err := hex.DecodeString(result.EvidenceSHA256)
	if !valid || err != nil || len(raw) != 32 {
		return submission.ErrResultConflict
	}
	data, _ := json.Marshal(result)
	sum := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(sum[:])
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	q := submissiondb.New(tx)
	if _, err := authorizeNode(ctx, q, node); err != nil {
		return err
	}
	row, err := q.LockAttempt(ctx, submissiondb.LockAttemptParams{Token: token, NodeID: node.ID, CredentialVersion: node.CredentialVersion})
	if errors.Is(err, pgx.ErrNoRows) {
		return submission.ErrLease
	}
	if err != nil {
		return err
	}
	if row.State == "completed" {
		if row.ResultSha256.String != fingerprint {
			return submission.ErrResultConflict
		}
		return nil
	}
	if row.State != "leased" || row.TaskState != "running" || row.AttemptNumber != row.CurrentAttempt {
		return submission.ErrLease
	}
	changed, err := q.CompleteAttempt(ctx, submissiondb.CompleteAttemptParams{Token: token, ResultSha256: pgtype.Text{String: fingerprint, Valid: true}, ResultPayload: data})
	if err != nil {
		return err
	}
	if changed != 1 {
		return submission.ErrLease
	}
	if result.Verdict == "SE" {
		err = q.RecoverTask(ctx, row.SubmissionID)
	} else {
		err = q.FinishTask(ctx, submissiondb.FinishTaskParams{SubmissionID: row.SubmissionID, Verdict: pgtype.Text{String: result.Verdict, Valid: true}})
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Recover returns at most 64 expired/revoked attempts to the queue. After three
// attempts the task becomes visibly failed, retaining all history and source.
func (s *Store) Recover(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	recovered := 0
	for range 64 {
		changed, err := s.recoverOne(ctx)
		if err != nil {
			return recovered, err
		}
		if !changed {
			return recovered, nil
		}
		recovered++
	}
	return recovered, nil
}

func (s *Store) recoverOne(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollback(tx)
	q := submissiondb.New(tx)
	row, err := q.ExpiredAttempt(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := q.ExpireAttempt(ctx, row.Token); err != nil {
		return false, err
	}
	if err := q.RecoverTask(ctx, row.SubmissionID); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func authorizeNode(ctx context.Context, q *submissiondb.Queries, node submission.NodeIdentity) (submissiondb.WorkerNode, error) {
	row, err := q.LockNode(ctx, node.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, submission.ErrNode
	}
	if err != nil {
		return row, err
	}
	if !row.Enabled || row.CredentialVersion != node.CredentialVersion {
		return row, submission.ErrNode
	}
	return row, nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
