package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/jackc/pgx/v5"
)

var _ problemreference.ReviewIntakeRepository = (*Store)(nil)

// SubmitReview attaches verified historical evidence without evaluating
// review eligibility. The draft is locked before reading the report and
// writing the separate owner/key ledger. Reported requests are terminal.
func (s *Store) SubmitReview(ctx context.Context, actor identity.ProblemActor, i problemreference.ReviewIntent) (problemreference.ReviewReceipt, error) {
	var zero problemreference.ReviewReceipt
	if ctx == nil || !i.Valid() {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, auth, err := s.begin(ctx, actor)
	if err != nil {
		return zero, err
	}
	defer rollback(tx)
	draft, err := s.access.LockOwnDraftIdentity(ctx, tx, actor, i.DraftID)
	if errors.Is(err, problem.ErrNotFound) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, err
	}
	if draft.ID != i.DraftID || draft.Owner != actor.SubjectID {
		return zero, storageError{problemreference.ErrInvalid}
	}
	if draft.ImportRevision != i.ImportRevision {
		return zero, problemreference.ErrSourceChanged
	}
	q := referencedb.New(tx)
	row, err := q.ReportedReferenceForReview(ctx, referencedb.ReportedReferenceForReviewParams{
		ID: i.ReferenceRequestID, Owner: actor.SubjectID, Draft: draft.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	r, err := readRequest(row)
	if err != nil {
		return zero, err
	}
	bound := r.Specification.Identity
	if bound.SourceID != draft.SourceID || bound.Version != draft.Version || bound.ImportRevision != draft.ImportRevision {
		return zero, problemreference.ErrSourceChanged
	}
	expected, err := loadExpectedPlan(ctx, q, r)
	if err != nil {
		return zero, storageError{err}
	}
	report, err := q.ReferenceWorkerReport(ctx, r.ID)
	if err != nil {
		return zero, storageError{err}
	}
	view, err := problemreference.ProjectAuthorReport(r, expected, report.PlanSha256, report.EvidenceSha256, report.Evidence)
	if err != nil {
		return zero, storageError{err}
	}
	var differentialPlan, differentialOutcome string
	if expected.Differential != nil {
		differentialPlan = expected.Differential.SHA256
	}
	if view.Comparison != nil {
		differentialOutcome = view.Comparison.Status
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return zero, storageError{err}
	}
	stored, err := q.InsertReviewSubmission(ctx, referencedb.InsertReviewSubmissionParams{
		ID: hex.EncodeToString(id[:]), Owner: actor.SubjectID, RequestKey: i.RequestKey,
		DraftID: i.DraftID, SourceID: bound.SourceID, ContentVersion: bound.Version,
		ImportRevision: bound.ImportRevision, SourceLockSha256: bound.SourceLockSHA256,
		ReferenceRequestID: r.ID, FormalPlanSha256: expected.SHA256, ProfileSha256: r.Specification.ProfileSHA256,
		EvidenceSha256: report.EvidenceSha256, FormalVerdict: view.Formal.Verdict,
		DifferentialPlanSha256: differentialPlan, DifferentialOutcome: differentialOutcome,
		AuthGeneration: auth.Actor.AuthGeneration, RecoveryID: auth.Actor.RecoveryID, AuthorRevision: auth.RoleRevision,
	})
	wrote := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// Use a fresh statement after the uniqueness wait; an earlier snapshot
		// may not see the concurrent winner.
		stored, err = q.ReviewSubmissionByOwnerKey(ctx, referencedb.ReviewSubmissionByOwnerKeyParams{
			Owner: actor.SubjectID, RequestKey: i.RequestKey})
	}
	if err != nil {
		return zero, storageError{err}
	}
	result, err := readReviewReceipt(stored, actor.SubjectID, i.RequestKey)
	if err != nil {
		return zero, err
	}
	if result.ReviewIntent != i {
		return zero, problemreference.ErrConflict
	}
	if result.ContentVersion != bound.Version || stored.SourceID != bound.SourceID ||
		stored.SourceLockSha256 != bound.SourceLockSHA256 || stored.FormalPlanSha256 != expected.SHA256 ||
		stored.ProfileSha256 != r.Specification.ProfileSHA256 || stored.EvidenceSha256 != report.EvidenceSha256 ||
		stored.FormalVerdict != view.Formal.Verdict || stored.DifferentialPlanSha256 != differentialPlan ||
		stored.DifferentialOutcome != differentialOutcome {
		return zero, storageError{problemreference.ErrInvalid}
	}
	if err := commitTx(ctx, tx, wrote); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, errors.Join(problemreference.ErrCommitUnknown, err)
	}
	return result, nil
}

// ReviewReceipt reads an immutable intake row under current author authority.
func (s *Store) ReviewReceipt(ctx context.Context, actor identity.ProblemActor, key string) (problemreference.ReviewReceipt, error) {
	var zero problemreference.ReviewReceipt
	if ctx == nil || !problem.ValidKey(key) {
		return zero, problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, _, err := s.begin(ctx, actor)
	if err != nil {
		return zero, err
	}
	defer rollback(tx)
	row, err := referencedb.New(tx).ReviewSubmissionByOwnerKey(ctx, referencedb.ReviewSubmissionByOwnerKeyParams{
		Owner: actor.SubjectID, RequestKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return zero, problemreference.ErrNotFound
	}
	if err != nil {
		return zero, storageError{err}
	}
	result, err := readReviewReceipt(row, actor.SubjectID, key)
	if err != nil {
		return zero, err
	}
	if err := commitTx(ctx, tx, false); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return result, nil
}

func readReviewReceipt(row referencedb.ProblemReviewSubmission, owner, key string) (problemreference.ReviewReceipt, error) {
	r := problemreference.ReviewReceipt{Owner: row.OwnerSubjectID, ID: row.ID,
		ReviewIntent: problemreference.ReviewIntent{RequestKey: row.RequestKey, DraftID: row.DraftID,
			ImportRevision: row.ImportRevision, ReferenceRequestID: row.ReferenceRequestID},
		ContentVersion: row.ContentVersion, CreatedAt: row.CreatedAt.Time}
	if !r.Valid() || r.Owner != owner || r.RequestKey != key || !row.CreatedAt.Valid || row.CreatedAt.InfinityModifier != 0 {
		return problemreference.ReviewReceipt{}, storageError{problemreference.ErrInvalid}
	}
	return r, nil
}
