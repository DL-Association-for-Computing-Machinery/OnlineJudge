package problem

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

const (
	publicationMaterialSchema = "oj.problem-publication-materials.v1"
	// MaxPublicationStatementBytes bounds the encoded display object, including
	// JSON text escaping and base64 image expansion.
	MaxPublicationStatementBytes = 7 << 20
)

// PublicationStatement is the allowlisted contestant-facing projection.
// Markdown remains untrusted input for the restricted renderer.
type PublicationStatement struct {
	ProblemID string         `json:"problem_id"`
	Version   string         `json:"version"`
	Title     string         `json:"title"`
	Language  string         `json:"language"`
	Markdown  string         `json:"markdown"`
	Examples  []Example      `json:"examples"`
	Images    []PreviewImage `json:"images,omitempty"`
}

// PublicationMaterialPlan holds a detached selection of display and private
// formal evidence. It conveys neither publication authority nor checker admission.
type PublicationMaterialPlan struct {
	problemID string
	statement previewPlan
	formal    ReferencePlan
}

// PlanPublicationMaterials prepares a detached candidate from an authorized
// draft, an independently pinned formal plan and a server-owned problem ID.
// The problem ID must be reserved by the caller; it is not the private draft ID.
func (d Draft) PlanPublicationMaterials(problemID, id, owner string, bound ImportIdentity, plan ReferencePlan, expectedPlan string) (PublicationMaterialPlan, error) {
	if !identity.ValidID(problemID) || problemID == id || !validHash(expectedPlan) ||
		plan.ValidateMaterials() != nil || plan.Digest() != expectedPlan {
		return PublicationMaterialPlan{}, ErrInvalid
	}
	m, err := d.PrepareReviewMaterials(id, owner, bound, plan)
	if err != nil {
		return PublicationMaterialPlan{}, err
	}
	plan.Cases = slices.Clone(plan.Cases)
	plan.Dependencies = slices.Clone(plan.Dependencies)
	return PublicationMaterialPlan{problemID: problemID, statement: m.statement, formal: plan}, nil
}

// PublicationMaterials couples a display artifact to the exact frozen import
// and private formal plan. This is an internal value; only the statement object
// may later be selected by an independently authorized publication transaction.
type PublicationMaterials struct {
	ProblemID  string
	Version    string
	Import     ImportIdentity
	FormalPlan string
	Statement  materialstore.Object
}

func (p PublicationMaterialPlan) statementBytes(ctx context.Context, store ReferenceObjectReader) (PublicationMaterials, []byte, error) {
	preview, err := readPreview(ctx, store, p.statement)
	if err != nil {
		return PublicationMaterials{}, nil, ErrStore
	}
	// Hash the projected bytes as well as the frozen binding. A change in
	// the normalized image bytes or any private formal material must not
	// silently reuse a publication version.
	display := PublicationStatement{ProblemID: p.problemID, Title: preview.Title, Language: preview.Language,
		Markdown: preview.Markdown, Examples: preview.Examples, Images: preview.Images}
	formal := p.formal.Digest()
	display.Version, err = publicationVersion(display, formal)
	if err != nil {
		return PublicationMaterials{}, nil, ErrStore
	}
	raw, err := json.Marshal(display)
	if err != nil || len(raw) > MaxPublicationStatementBytes {
		return PublicationMaterials{}, nil, ErrStore
	}
	if err := ctx.Err(); err != nil {
		return PublicationMaterials{}, nil, err
	}
	return PublicationMaterials{ProblemID: p.problemID, Version: display.Version, FormalPlan: formal,
		Import: ImportIdentity{SourceID: p.formal.SourceID, Version: p.formal.Version,
			ImportRevision: p.formal.ImportRevision, SourceLockSHA256: p.formal.SourceLockSHA256},
		Statement: materialstore.Object{SHA256: hash(raw), Size: int64(len(raw))}}, raw, nil
}

func publicationVersion(display PublicationStatement, formal string) (string, error) {
	display.Version = ""
	projection, err := json.Marshal(display)
	if err != nil {
		return "", err
	}
	binding, err := json.Marshal([4]string{publicationMaterialSchema, display.ProblemID, formal, hash(projection)})
	if err != nil {
		return "", err
	}
	return hash(binding), nil
}
