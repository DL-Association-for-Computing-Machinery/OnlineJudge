package problem

import (
	"context"
	"reflect"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

// ReviewMaterialManifest is a detached, bounded selection from an already
// authorized draft and its frozen formal plan. It conveys no authority.
type ReviewMaterialManifest struct {
	statement previewPlan
	solution  materialstore.Object
	libraries []previewFile
}

// PrepareReviewMaterials verifies the complete import binding before selecting
// the statement, designated samples/images, and the formal source closure.
// It never reads object bytes or reselects a solution from today's catalog.
func (d Draft) PrepareReviewMaterials(id, owner string, bound ImportIdentity, plan ReferencePlan) (ReviewMaterialManifest, error) {
	var zero ReviewMaterialManifest
	if plan.SourceID != bound.SourceID || plan.Version != bound.Version ||
		plan.ImportRevision != bound.ImportRevision || plan.SourceLockSHA256 != bound.SourceLockSHA256 ||
		plan.ValidateMaterials() != nil {
		return zero, ErrStore
	}
	content, err := d.BindReferenceContent(id, owner, bound)
	if err != nil {
		return zero, ErrStore
	}
	d.Document, d.References = content.Document, content.References
	doc, files, err := previewFiles(d, owner, id)
	if err != nil {
		return zero, err
	}
	admission, err := referenceDiskAdmission(bound.SourceID)
	if bound.SourceID == "library-checker:aplusb" {
		admission, err = diskReferenceAdmission{path: "sample/aplusb", limits: SmallReferenceLimits()}, nil
	}
	if err != nil {
		return zero, ErrStore
	}
	derived, _, err := referenceMetadata(content, admission.limits, bound.SourceID, admission.path)
	if err != nil {
		return zero, ErrStore
	}
	if _, err := bindReferenceDependencies(&derived, doc, admission); err != nil || derived.Digest() != plan.Digest() {
		return zero, ErrStore
	}
	original := make(map[string]materialstore.Object, 1+len(plan.Dependencies))
	original["original/source/"+admission.path+"/sol/correct.cpp"] = plan.Source
	var libraries []previewFile
	for i, dep := range plan.Dependencies {
		original["original/"+admission.dependencies[i].original] = dep.Object
		libraries = append(libraries, previewFile{dep.Path, dep.Object})
	}
	for _, persisted := range content.References {
		if object, selected := original[persisted.Name]; selected && object == persisted.Object {
			delete(original, persisted.Name)
		}
	}
	if len(original) != 0 {
		return zero, ErrStore
	}
	return ReviewMaterialManifest{statement: newPreviewPlan(id, doc, files), solution: plan.Source, libraries: libraries}, nil
}

// Equal compares the exact selected identities after object reads.
func (m ReviewMaterialManifest) Equal(other ReviewMaterialManifest) bool {
	return reflect.DeepEqual(m, other)
}

// ReviewSolution contains just the formal entry and any dependencies pinned in
// the same plan. Names are fixed compiler-relative labels, not storage paths.
type ReviewSolution struct {
	Source       string             `json:"source"`
	Dependencies []ReviewDependency `json:"dependencies,omitempty"`
}

// ReviewDependency is one admitted compiler input, never a download selector.
type ReviewDependency struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// Read verifies the selected objects and returns only review-visible bytes.
// The caller must acquire a material slot and recheck authority after Read.
func (m ReviewMaterialManifest) Read(ctx context.Context, store ReferenceObjectReader) (Preview, ReviewSolution, error) {
	if !m.validSolutionRead(ctx, store) {
		return Preview{}, ReviewSolution{}, ErrInvalid
	}
	preview, err := m.ReadStatement(ctx, store)
	if err != nil {
		return Preview{}, ReviewSolution{}, err
	}
	solution, err := m.ReadSolution(ctx, store)
	if err != nil {
		return Preview{}, ReviewSolution{}, err
	}
	return preview, solution, nil
}

// ReadStatement verifies only the selected statement, samples and images.
// The caller holds the material slot and must recheck authority after this read.
func (m ReviewMaterialManifest) ReadStatement(ctx context.Context, store ReferenceObjectReader) (Preview, error) {
	return readPreview(ctx, store, m.statement)
}

// ReadSolution verifies only the pinned solution closure. The caller supplies
// the same material slot and post-read authorization as for Read.
func (m ReviewMaterialManifest) ReadSolution(ctx context.Context, store ReferenceObjectReader) (ReviewSolution, error) {
	var zero ReviewSolution
	if !m.validSolutionRead(ctx, store) {
		return zero, ErrInvalid
	}
	source, err := readPreviewText(ctx, store, m.solution)
	if err != nil {
		return zero, err
	}
	solution := ReviewSolution{Source: source}
	for _, dep := range m.libraries {
		source, err := readPreviewText(ctx, store, dep.object)
		if err != nil {
			return zero, err
		}
		solution.Dependencies = append(solution.Dependencies, ReviewDependency{Name: dep.name, Source: source})
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return solution, nil
}

func (m ReviewMaterialManifest) validSolutionRead(ctx context.Context, store ReferenceObjectReader) bool {
	return ctx != nil && store != nil && m.solution.Size >= 1 && m.solution.Size <= SmallReferenceLimits().SourceBytes &&
		validHash(m.solution.SHA256)
}
