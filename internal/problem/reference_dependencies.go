package problem

import "context"

// Only the explicitly reviewed source closure is compiled, not every original
// header in the imported bundle (which also contains generators and checkers).
func bindReferenceDependencies(plan *ReferencePlan, d document, admission diskReferenceAdmission) ([]fileReference, error) {
	if len(admission.dependencies) == 0 {
		return nil, nil
	}
	sourceBytes := plan.Source.Size
	total := sourceBytes + plan.Definition.Size + plan.Hashes.Size
	for _, c := range plan.Cases {
		total += c.Input.Size + c.Answer.Size
	}
	refs := make([]fileReference, 0, len(admission.dependencies))
	for _, dependency := range admission.dependencies {
		ref, ok := d.Provenance.OriginalFiles[dependency.original]
		if !ok || ref.Size < 1 || ref.Size > admission.limits.SourceBytes-sourceBytes || ref.Size > admission.limits.TotalBytes-total {
			return nil, ErrReferenceUnsupported
		}
		sourceBytes += ref.Size
		total += ref.Size
		refs = append(refs, ref)
		plan.Dependencies = append(plan.Dependencies, ReferenceDependency{Path: dependency.target, Object: ref.object()})
	}
	plan.SourcePath = admission.entry
	return refs, nil
}

// ReadDependencies returns fresh copies from the same frozen anonymous file.
// Close/cancellation cannot produce a successful partial compilation bundle.
func (s *ReferenceSnapshot) ReadDependencies(ctx context.Context) ([]ReferenceSourceFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx == nil || s.closed {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	files := make([]ReferenceSourceFile, 0, len(s.plan.Dependencies))
	for i, dependency := range s.plan.Dependencies {
		raw, err := s.readLocked(ctx, 1+i)
		if err != nil {
			return nil, err
		}
		files = append(files, ReferenceSourceFile{Path: dependency.Path, Data: raw})
	}
	return files, nil
}
