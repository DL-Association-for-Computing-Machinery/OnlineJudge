package problem

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

// ValidateMaterials checks fixed source layout and budgets before any object I/O.
// The caller must separately authenticate the canonical plan digest/source lock.
func (p ReferencePlan) ValidateMaterials() error {
	a, err := referenceDiskAdmission(p.SourceID)
	if p.SourceID == "library-checker:aplusb" {
		a, err = diskReferenceAdmission{path: "sample/aplusb", limits: SmallReferenceLimits()}, nil
	}
	if err != nil || !validHash(p.Version) || !validHash(p.ImportRevision) || !validHash(p.SourceLockSHA256) ||
		p.SourcePath != a.entry || len(p.Dependencies) != len(a.dependencies) || len(p.Cases) == 0 || len(p.Cases) > a.limits.Cases || p.SourceData != nil {
		return ErrReferenceUnsupported
	}
	if _, ok := upstreamPlanSize(p); !ok {
		return ErrReferenceUnsupported
	}
	var total, source int64
	check := func(o materialstore.Object, max int64) bool {
		if !validHash(o.SHA256) || o.Size < 0 || o.Size > max || o.Size > a.limits.TotalBytes-total {
			return false
		}
		total += o.Size
		return true
	}
	if p.Source.Size < 1 || p.Definition.Size < 1 || p.Hashes.Size < 1 || !check(p.Source, a.limits.SourceBytes) ||
		!check(p.Definition, maxReferenceCatalog) || !check(p.Hashes, maxReferenceCatalog) {
		return ErrReferenceUnsupported
	}
	source = p.Source.Size
	for i, d := range p.Dependencies {
		if d.Path != a.dependencies[i].target || d.Object.Size < 1 || !check(d.Object, a.limits.SourceBytes-source) {
			return ErrReferenceUnsupported
		}
		source += d.Object.Size
	}
	for i, c := range p.Cases {
		if c.ID != fmt.Sprintf("case-%04d", i+1) || c.InputData != nil || c.AnswerData != nil || !check(c.Input, a.limits.InputBytes) || !check(c.Answer, a.limits.AnswerBytes) {
			return ErrReferenceUnsupported
		}
	}
	return nil
}

// MaterialObjects returns only the bounded plan's admitted private objects.
func (p ReferencePlan) MaterialObjects() ([]materialstore.Object, error) {
	if err := p.ValidateMaterials(); err != nil {
		return nil, err
	}
	objects := []materialstore.Object{p.Definition, p.Hashes, p.Source}
	for _, d := range p.Dependencies {
		objects = append(objects, d.Object)
	}
	for _, c := range p.Cases {
		objects = append(objects, c.Input, c.Answer)
	}
	return objects, nil
}

// FreezeMaterials independently validates all bytes and the complete catalogue
// into the shared bounded anonymous snapshot. It does not rederive import
// provenance or grant execution; the caller pins the trusted expected digest.
func (p ReferencePlan) FreezeMaterials(ctx context.Context, reader func(context.Context, materialstore.Object) (io.ReadCloser, error)) (*ReferenceSnapshot, error) {
	return p.freezeMaterials(ctx, ctx, reader)
}

// FreezeMaterialsWithDownloadDeadline limits acquisition without shortening the
// lifetime of the completed file. The caller must supply a lease-supervised
// context and close the snapshot. Its lifetime is fixed at construction; a
// canceled snapshot cannot be rebound. This method does not grant authority.
func (p ReferencePlan) FreezeMaterialsWithDownloadDeadline(ctx context.Context, deadline time.Time, reader func(context.Context, materialstore.Object) (io.ReadCloser, error)) (*ReferenceSnapshot, error) {
	if ctx == nil || deadline.IsZero() {
		return nil, ErrInvalid
	}
	download, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	return p.freezeMaterials(download, ctx, reader)
}

func (p ReferencePlan) freezeMaterials(ctx, lifetime context.Context, reader func(context.Context, materialstore.Object) (io.ReadCloser, error)) (*ReferenceSnapshot, error) {
	if ctx == nil || reader == nil {
		return nil, ErrInvalid
	}
	objects, err := p.MaterialObjects()
	if err != nil {
		return nil, err
	}
	// Detach before crossing the I/O callback boundary.
	p.Cases = append([]ReferenceCase(nil), p.Cases...)
	p.Dependencies = append([]ReferenceDependency(nil), p.Dependencies...)
	open := referenceOpen(func(ctx context.Context, ref fileReference) (io.ReadCloser, error) { return reader(ctx, ref.object()) })
	ref := func(o materialstore.Object) fileReference { return fileReference{SHA256: o.SHA256, Size: o.Size} }
	return freezeSnapshot(ctx, lifetime, open, nil, func() (ReferencePlan, []fileReference, error) {
		info, err := open.read(ctx, ref(p.Definition))
		if err != nil {
			return ReferencePlan{}, nil, err
		}
		hashes, err := open.read(ctx, ref(p.Hashes))
		if err != nil {
			return ReferencePlan{}, nil, err
		}
		upstream, ok := upstreamPlanSize(p)
		catalog, digests, err := referenceCatalog(info, hashes, upstream)
		if !ok || err != nil || len(catalog) != upstream {
			return ReferencePlan{}, nil, ErrReferenceUnsupported
		}
		for i, c := range catalog {
			if p.Cases[i].Input.SHA256 != digests[c.name+".in"] || p.Cases[i].Answer.SHA256 != digests[c.name+".out"] {
				return ReferencePlan{}, nil, ErrReferenceUnsupported
			}
		}
		refs := make([]fileReference, 0, len(objects)-2)
		for _, o := range objects[2:] {
			refs = append(refs, ref(o))
		}
		return p, refs, nil
	})
}
