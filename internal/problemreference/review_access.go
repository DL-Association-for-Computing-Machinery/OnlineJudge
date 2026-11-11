package problemreference

import (
	"context"
	"math"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ReviewAssignment is the maintenance view of the latest assignment revision.
// An empty reviewer denotes a recorded withdrawal.
type ReviewAssignment struct {
	ReviewID          string `json:"review_id"`
	Revision          int64  `json:"revision"`
	ReviewerSubjectID string `json:"reviewer_subject_id,omitempty"`
	ReviewerRevision  int64  `json:"reviewer_revision,omitempty"`
}

// ReviewAssignmentChange is a trusted maintenance CAS. Granting also requires
// the specific reviewer-role revision confirmed by the operator.
type ReviewAssignmentChange struct {
	ReviewID                 string
	ExpectedRevision         int64
	ReviewerSubjectID        string
	ExpectedReviewerRevision int64
	Operator                 string
	Reason                   string
}

// Valid checks the grant/withdrawal input without granting any permission.
func (c ReviewAssignmentChange) Valid() bool {
	if !identity.ValidID(c.ReviewID) || c.ExpectedRevision < 0 || c.ExpectedRevision == math.MaxInt64 ||
		!identity.ValidMaintenanceAttribution(c.Operator, c.Reason) {
		return false
	}
	if c.ReviewerSubjectID == "" {
		return c.ExpectedRevision > 0 && c.ExpectedReviewerRevision == 0
	}
	return identity.ValidID(c.ReviewerSubjectID) && c.ExpectedReviewerRevision > 0
}

// ReviewEvidence is a whitelist of evidence identity and recorded conclusions
// for a single assigned item. Missing comparison stays visible as missing.
// This projection does not imply satisfaction of any review policy.
type ReviewEvidence struct {
	Reviewer           string `json:"-"`
	AssignmentRevision int64  `json:"-"`
	ReviewID           string `json:"review_id"`
	DraftID            string `json:"draft_id"`
	SourceID           string `json:"source_id"`
	ContentVersion     string `json:"content_version"`
	ImportRevision     string `json:"import_revision"`
	SourceLockSHA256   string `json:"source_lock_sha256"`
	ReferenceRequestID string `json:"reference_request_id"`
	FormalPlanSHA256   string `json:"formal_plan_sha256"`
	ProfileSHA256      string `json:"profile_sha256"`
	EvidenceSHA256     string `json:"evidence_sha256"`
	FormalVerdict      string `json:"formal_verdict"`
	ComparisonPlan     string `json:"comparison_plan_sha256,omitempty"`
	ComparisonOutcome  string `json:"comparison_outcome,omitempty"`
}

// Valid checks projection shape; storage must separately authorize the reader.
func (e ReviewEvidence) Valid() bool {
	if !identity.ValidID(e.Reviewer) || e.AssignmentRevision < 1 || !identity.ValidID(e.ReviewID) ||
		!identity.ValidID(e.DraftID) || !identity.ValidID(e.ReferenceRequestID) ||
		e.SourceID == "" || len(e.SourceID) > 128 || (e.FormalVerdict != "AC" && e.FormalVerdict != "CE" && !failedPoint(e.FormalVerdict)) {
		return false
	}
	for _, h := range []string{e.ContentVersion, e.ImportRevision, e.SourceLockSHA256, e.FormalPlanSHA256, e.ProfileSHA256, e.EvidenceSHA256} {
		if !problem.ValidImportRevision(h) {
			return false
		}
	}
	if e.ComparisonPlan == "" {
		return e.ComparisonOutcome == ""
	}
	return problem.ValidImportRevision(e.ComparisonPlan) &&
		(e.ComparisonOutcome == "" || e.ComparisonOutcome == "passed" || e.ComparisonOutcome == "mismatch")
}

// ReviewEvidenceRepository checks current reviewer role and assignment in the
// same transaction used to read the immutable evidence projection.
type ReviewEvidenceRepository interface {
	ReviewEvidence(context.Context, identity.ProblemActor, string) (ReviewEvidence, error)
}

// ReviewAccess binds the reviewer read to a fresh ordinary session.
type ReviewAccess struct {
	sessions SessionReader
	repo     ReviewEvidenceRepository
}

// NewReviewAccess constructs the separate reviewer capability.
func NewReviewAccess(sessions SessionReader, repo ReviewEvidenceRepository) (*ReviewAccess, error) {
	if sessions == nil || repo == nil {
		return nil, ErrInvalid
	}
	return &ReviewAccess{sessions: sessions, repo: repo}, nil
}

// EvidenceForSubject reads a single assigned item's fixed evidence.
func (s *ReviewAccess) EvidenceForSubject(ctx context.Context, c redisstore.Credential, expected, id string) (ReviewEvidence, error) {
	var zero ReviewEvidence
	if s == nil || s.sessions == nil || s.repo == nil || ctx == nil || !identity.ValidID(expected) || !identity.ValidID(id) {
		return zero, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	actor, err := observeActor(ctx, s.sessions, c)
	if err != nil {
		return zero, err
	}
	if actor.SubjectID != expected {
		return zero, ErrSubjectChanged
	}
	e, err := s.repo.ReviewEvidence(ctx, actor, id)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if !e.Valid() || e.ReviewID != id || e.Reviewer != expected {
		return zero, ErrStore
	}
	return e, nil
}
