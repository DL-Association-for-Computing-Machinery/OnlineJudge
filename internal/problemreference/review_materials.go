package problemreference

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ReviewMaterialSnapshot is the detached, private authorization observation.
// It can select objects but does not authorize releasing their bytes.
type ReviewMaterialSnapshot struct {
	Evidence     ReviewEvidence
	Owner        string
	RoleRevision int64
	Manifest     problem.ReviewMaterialManifest
}

func (s ReviewMaterialSnapshot) validFor(actor identity.ProblemActor, id string) bool {
	return s.Evidence.Valid() && s.Evidence.Reviewer == actor.SubjectID && s.Evidence.ReviewID == id &&
		identity.ValidID(s.Owner) && s.Owner != actor.SubjectID && s.RoleRevision > 0
}

// ReviewMaterialRepository rechecks current reviewer authority and all frozen
// material bindings in one short transaction.
type ReviewMaterialRepository interface {
	ReviewMaterialSnapshot(context.Context, identity.ProblemActor, string) (ReviewMaterialSnapshot, error)
}

// ReviewMaterials is the complete whitelist returned to the assigned reviewer.
// The solution is source text; this path never compiles or executes it.
type ReviewMaterials struct {
	Evidence         ReviewEvidence         `json:"evidence"`
	Statement        problem.Preview        `json:"statement"`
	StandardSolution problem.ReviewSolution `json:"standard_solution"`
}

// Valid checks that the response projects the same draft and formal evidence.
func (m ReviewMaterials) Valid() bool {
	total := len(m.StandardSolution.Source)
	for _, dep := range m.StandardSolution.Dependencies {
		total += len(dep.Source)
	}
	return m.Evidence.Valid() && m.Statement.DraftID == m.Evidence.DraftID &&
		m.Statement.Version == m.Evidence.ContentVersion && len(m.StandardSolution.Source) > 0 &&
		int64(total) <= problem.SmallReferenceLimits().SourceBytes
}

// MaxReviewMaterialResponse bounds JSON expansion of the authorized material.
const MaxReviewMaterialResponse = 7 << 20

// ReviewMaterialReader releases bytes only after two fresh observations and
// two successful authorization transactions for the exact item.
type ReviewMaterialReader struct {
	sessions SessionReader
	repo     ReviewMaterialRepository
	objects  problem.ReferenceObjectReader
	acquire  func() (func(), error)
}

// NewReviewMaterialReader shares the configured draft runtime's object budget.
func NewReviewMaterialReader(sessions SessionReader, repo ReviewMaterialRepository, objects problem.ReferenceObjectReader, acquire func() (func(), error)) (*ReviewMaterialReader, error) {
	if sessions == nil || repo == nil || objects == nil || acquire == nil {
		return nil, ErrInvalid
	}
	return &ReviewMaterialReader{sessions: sessions, repo: repo, objects: objects, acquire: acquire}, nil
}

// MaterialsForSubject reads the fixed statement and formal source, then
// rechecks the actual credential, reviewer grant, assignment and evidence.
func (s *ReviewMaterialReader) MaterialsForSubject(ctx context.Context, c redisstore.Credential, expected, id string) (ReviewMaterials, error) {
	var zero ReviewMaterials
	if s == nil || s.sessions == nil || s.repo == nil || s.objects == nil || s.acquire == nil ||
		ctx == nil || !identity.ValidID(expected) || !identity.ValidID(id) {
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
	before, err := s.repo.ReviewMaterialSnapshot(ctx, actor, id)
	if err != nil {
		return zero, err
	}
	if !before.validFor(actor, id) {
		return zero, ErrStore
	}
	release, err := s.acquire()
	if err != nil {
		return zero, err
	}
	if release == nil {
		return zero, ErrStore
	}
	defer release()
	preview, source, err := before.Manifest.Read(ctx, s.objects)
	if err != nil {
		return zero, err
	}
	afterActor, err := observeActor(ctx, s.sessions, c)
	if err != nil {
		return zero, err
	}
	if afterActor.SubjectID != expected {
		return zero, ErrSubjectChanged
	}
	if afterActor != actor {
		return zero, identity.ErrConflict
	}
	after, err := s.repo.ReviewMaterialSnapshot(ctx, afterActor, id)
	if err != nil {
		return zero, err
	}
	if !after.validFor(actor, id) || after.Owner != before.Owner || after.RoleRevision != before.RoleRevision ||
		after.Evidence != before.Evidence || !after.Manifest.Equal(before.Manifest) {
		return zero, ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	m := ReviewMaterials{Evidence: before.Evidence, Statement: preview, StandardSolution: source}
	if !m.Valid() {
		return zero, ErrStore
	}
	return m, nil
}
