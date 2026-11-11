package problem

import (
	"context"
	"slices"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

const convolutionDraftPath = "convolution/convolution_mod"

// FrozenConvolutionSources contains only the two pinned program roles from
// one previously authorized draft. It is neither a plan nor an execution grant.
type FrozenConvolutionSources struct {
	Oracle    string `json:"-"`
	Candidate string `json:"-"`
}

// ConvolutionSolverObjects identifies the two reviewed programs in the trusted lock.
type ConvolutionSolverObjects struct {
	Oracle, Candidate materialstore.Object
}

// FreezeConvolutionSources binds the full persisted draft to the selected
// import before reading only the two reviewed solver objects. The caller
// supplies trusted import identity and must recheck live authority before
// persisting a plan. No official input or answer is opened.
func (d Draft) FreezeConvolutionSources(ctx context.Context, id, owner string, expected ImportIdentity, pins ConvolutionSolverObjects, store ReferenceObjectReader) (FrozenConvolutionSources, error) {
	var zero FrozenConvolutionSources
	if ctx == nil || store == nil || expected.SourceID != "library-checker:convolution_mod" {
		return zero, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	content, err := d.BindReferenceContent(id, owner, expected)
	if err != nil {
		return zero, err
	}
	doc, err := parseDocument(content.Document, content.SourceID)
	if err != nil || doc.Provenance.SourcePath != convolutionDraftPath {
		return zero, ErrInvalid
	}
	// Check both logical roles against the original document before any I/O.
	var refs [2]fileReference
	objects := [2]materialstore.Object{pins.Oracle, pins.Candidate}
	for i, solver := range []string{"naive.cpp", "correct.cpp"} {
		name := "source/" + convolutionDraftPath + "/sol/" + solver
		ref, ok := doc.Provenance.OriginalFiles[name]
		if !ok || ref.Size < 1 || ref.Size > 64<<10 || ref.object() != objects[i] {
			return zero, ErrInvalid
		}
		index, found := slices.BinarySearchFunc(content.References, "original/"+name, func(ref Reference, name string) int {
			return strings.Compare(ref.Name, name)
		})
		if !found || content.References[index].Object != ref.object() {
			return zero, ErrInvalid
		}
		refs[i] = ref
	}
	if refs[0].SHA256 == refs[1].SHA256 {
		return zero, ErrInvalid
	}
	open := persistedReferenceOpen(store)
	oracle, err := open.read(ctx, refs[0])
	if err != nil {
		return zero, err
	}
	candidate, err := open.read(ctx, refs[1])
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return FrozenConvolutionSources{Oracle: string(oracle), Candidate: string(candidate)}, nil
}
