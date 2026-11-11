package problemreference

import (
	"context"
	"errors"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// StatementChecklistVersion fixes the human declaration recorded by the receipt.
const StatementChecklistVersion = "oj.statement-semantics.v1"

// StatementConfirmationIntent identifies the assigned review and explicit checklist declaration.
// Sending this version affirms all four checks: meaning, constraints, I/O and sample explanations.
type StatementConfirmationIntent struct {
	RequestKey         string `json:"request_key"`
	AssignmentRevision int64  `json:"assignment_revision"`
	ImportRevision     string `json:"import_revision"`
	EvidenceSHA256     string `json:"evidence_sha256"`
	ChecklistVersion   string `json:"checklist_version"`
}

// Valid checks the declaration and its expected evidence identity.
func (i StatementConfirmationIntent) Valid() bool {
	return problem.ValidKey(i.RequestKey) && i.AssignmentRevision > 0 &&
		problem.ValidImportRevision(i.ImportRevision) && problem.ValidImportRevision(i.EvidenceSHA256) &&
		i.ChecklistVersion == StatementChecklistVersion
}

// Matches checks the exact revision shown to the reviewer.
func (i StatementConfirmationIntent) Matches(e ReviewEvidence) bool {
	return i.AssignmentRevision == e.AssignmentRevision && i.ImportRevision == e.ImportRevision &&
		i.EvidenceSHA256 == e.EvidenceSHA256
}

// StatementConfirmationReceipt is the reviewer's private, original-key receipt.
type StatementConfirmationReceipt struct {
	Reviewer string `json:"-"`
	ReviewID string `json:"review_id"`
	StatementConfirmationIntent
	ConfirmedAt time.Time `json:"confirmed_at"`
}

// Valid checks receipt shape independently of the repository's authorization.
func (r StatementConfirmationReceipt) Valid() bool {
	return identity.ValidID(r.Reviewer) && identity.ValidID(r.ReviewID) &&
		r.StatementConfirmationIntent.Valid() && !r.ConfirmedAt.IsZero()
}

// StatementConfirmationRepository owns the final authorization and append-only record.
type StatementConfirmationRepository interface {
	ReviewMaterialRepository
	RecordStatementConfirmation(context.Context, identity.ProblemActor, string, StatementConfirmationIntent, ReviewMaterialSnapshot) (StatementConfirmationReceipt, error)
	StatementConfirmationReceipt(context.Context, identity.ProblemActor, string, string) (StatementConfirmationReceipt, error)
}

// StatementConfirmations verifies objects before a distinct, reauthorized write.
type StatementConfirmations struct {
	sessions SessionReader
	repo     StatementConfirmationRepository
	objects  problem.ReferenceObjectReader
	acquire  func() (func(), error)
}

// NewStatementConfirmations uses the same material reader and slot as review preview.
func NewStatementConfirmations(sessions SessionReader, repo StatementConfirmationRepository, objects problem.ReferenceObjectReader, acquire func() (func(), error)) (*StatementConfirmations, error) {
	if sessions == nil || repo == nil || objects == nil || acquire == nil {
		return nil, ErrInvalid
	}
	return &StatementConfirmations{sessions: sessions, repo: repo, objects: objects, acquire: acquire}, nil
}

// ConfirmForSubject confirms all items in the explicit checklist, or replays the original intent.
func (s *StatementConfirmations) ConfirmForSubject(ctx context.Context, c redisstore.Credential, expected, id string, i StatementConfirmationIntent) (StatementConfirmationReceipt, error) {
	var zero StatementConfirmationReceipt
	if !i.Valid() {
		return zero, ErrInvalid
	}
	actor, err := s.actor(ctx, c, expected, id)
	if err != nil {
		return zero, err
	}
	prior, err := s.repo.StatementConfirmationReceipt(ctx, actor, id, i.RequestKey)
	switch {
	case err == nil:
		if prior.StatementConfirmationIntent != i {
			return zero, ErrConflict
		}
		return prior, nil
	case !errors.Is(err, ErrNotFound):
		return zero, err
	}
	before, err := s.repo.ReviewMaterialSnapshot(ctx, actor, id)
	if err != nil {
		return zero, err
	}
	if !before.validFor(actor, id) {
		return zero, ErrStore
	}
	if !i.Matches(before.Evidence) {
		return zero, ErrReviewChanged
	}
	if err := s.verifyStatement(ctx, before); err != nil {
		return zero, err
	}
	after, err := s.actor(ctx, c, expected, id)
	if err != nil {
		return zero, err
	}
	if after != actor {
		return zero, ErrReviewChanged
	}
	return s.repo.RecordStatementConfirmation(ctx, after, id, i, before)
}

func (s *StatementConfirmations) verifyStatement(ctx context.Context, before ReviewMaterialSnapshot) error {
	release, err := s.acquire()
	if err != nil {
		return err
	}
	if release == nil {
		return ErrStore
	}
	defer release()
	_, err = before.Manifest.ReadStatement(ctx, s.objects)
	return err
}

// ReceiptForSubject recovers a confirmation without checking objects or writing again.
func (s *StatementConfirmations) ReceiptForSubject(ctx context.Context, c redisstore.Credential, expected, id, key string) (StatementConfirmationReceipt, error) {
	if !problem.ValidKey(key) {
		return StatementConfirmationReceipt{}, ErrInvalid
	}
	actor, err := s.actor(ctx, c, expected, id)
	if err != nil {
		return StatementConfirmationReceipt{}, err
	}
	return s.repo.StatementConfirmationReceipt(ctx, actor, id, key)
}

func (s *StatementConfirmations) actor(ctx context.Context, c redisstore.Credential, expected, id string) (identity.ProblemActor, error) {
	if s == nil || s.sessions == nil || s.repo == nil || s.objects == nil || s.acquire == nil ||
		ctx == nil || !identity.ValidID(expected) || !identity.ValidID(id) {
		return identity.ProblemActor{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return identity.ProblemActor{}, err
	}
	actor, err := observeActor(ctx, s.sessions, c)
	if err == nil && actor.SubjectID != expected {
		return identity.ProblemActor{}, ErrSubjectChanged
	}
	return actor, err
}
