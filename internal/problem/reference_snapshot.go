package problem

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

// UnionFindReferenceLimits bounds the independently admitted disk snapshot.
// It does not expand the existing small-material PrepareReference protocol.
func UnionFindReferenceLimits() ReferenceLimits {
	return ReferenceLimits{SourceBytes: 64 << 10, InputBytes: 4 << 20, AnswerBytes: 256 << 10, TotalBytes: 64 << 20, Cases: 64}
}

var referenceSnapshotActive atomic.Bool

type referenceSpan struct{ offset, size int64 }

// ReferenceSnapshot owns one fully verified anonymous disk file. At most one
// may be active in this process. A bundle's Prepared remains occupied until Close.
// Neither the bundle nor the object store is read after preparation succeeds.
type ReferenceSnapshot struct {
	mu        sync.Mutex
	file      *os.File
	ctx       context.Context
	release   func()
	plan      ReferencePlan
	spans     []referenceSpan // primary source, dependencies, then ordered point pairs
	caseStart int             // compilation files precede point spans
	closed    bool
	err       error
}

// PrepareUnionFindReference freezes the complete source-locked unionfind draft.
// Every object is fully hashed using the same opened source before returning.
func (p *Prepared) PrepareUnionFindReference(ctx context.Context, limits ReferenceLimits) (_ *ReferenceSnapshot, err error) {
	if p == nil || ctx == nil || limits != UnionFindReferenceLimits() {
		return nil, ErrInvalid
	}
	return p.prepareDiskReference(ctx, "library-checker:unionfind", diskReferenceAdmission{path: "data_structure/unionfind", limits: limits})
}

// PrepareDiskReference selects the complete source's fixed admission internally.
// No caller-supplied layout or budget can widen the shared snapshot protocol.
func (p *Prepared) PrepareDiskReference(ctx context.Context) (*ReferenceSnapshot, error) {
	return p.PrepareDiskReferenceWithLifetime(ctx, ctx)
}

// PrepareDiskReferenceWithLifetime separates the deadline for freezing from
// the lifetime of the already frozen material. The caller still owns Close.
func (p *Prepared) PrepareDiskReferenceWithLifetime(ctx, lifetime context.Context) (*ReferenceSnapshot, error) {
	if p == nil || ctx == nil || lifetime == nil {
		return nil, ErrInvalid
	}
	a, err := referenceDiskAdmission(p.content.SourceID)
	if err != nil {
		return nil, err
	}
	return p.prepareDiskReferenceWithLifetime(ctx, lifetime, p.content.SourceID, a)
}

// LocalTextReferenceLimits bounds a single-file GNU++17 reference and its
// complete local text catalog. The exported judge objects have their own cap.
func LocalTextReferenceLimits() ReferenceLimits {
	return ReferenceLimits{SourceBytes: 64 << 10, InputBytes: 4 << 20, AnswerBytes: 256 << 10, TotalBytes: 48<<20 + 3*maxReferenceCatalog, Cases: 64}
}

// PrepareLocalTextReference freezes the catalog identified by the source lock
// already checked by LoadBundle. This profile admits only sol/correct.cpp as
// the compile input; source closures with additional files need another profile.
func (p *Prepared) PrepareLocalTextReference(ctx, lifetime context.Context) (*ReferenceSnapshot, error) {
	if p == nil || ctx == nil || lifetime == nil {
		return nil, ErrInvalid
	}
	d, err := parseDocument(p.content.Document, p.content.SourceID)
	if err != nil {
		return nil, err
	}
	return p.prepareDiskReferenceWithLifetime(ctx, lifetime, p.content.SourceID,
		diskReferenceAdmission{path: d.Provenance.SourcePath, limits: LocalTextReferenceLimits()})
}

func (p *Prepared) prepareDiskReference(ctx context.Context, sourceID string, admission diskReferenceAdmission) (*ReferenceSnapshot, error) {
	return p.prepareDiskReferenceWithLifetime(ctx, ctx, sourceID, admission)
}

func (p *Prepared) prepareDiskReferenceWithLifetime(ctx, lifetime context.Context, sourceID string, admission diskReferenceAdmission) (*ReferenceSnapshot, error) {
	if err := p.acquire(); err != nil {
		return nil, err
	}
	return freezeReferenceSnapshot(ctx, lifetime, p.content, sourceID, admission, p.openReference, p.release)
}

func freezeReferenceSnapshot(ctx, lifetime context.Context, content Content, sourceID string, admission diskReferenceAdmission, open referenceOpen, release func()) (_ *ReferenceSnapshot, err error) {
	return freezeSnapshot(ctx, lifetime, open, release, func() (ReferencePlan, []fileReference, error) {
		plan, d, err := referenceMetadata(content, admission.limits, sourceID, admission.path)
		if err != nil {
			return plan, nil, err
		}
		dependencies, err := bindReferenceDependencies(&plan, d, admission)
		if err != nil {
			return plan, nil, err
		}
		if err := open.validateCatalog(ctx, d, admission.limits.Cases); err != nil {
			return plan, nil, err
		}
		refs := []fileReference{d.Provenance.OriginalFiles["source/"+admission.path+"/sol/correct.cpp"]}
		refs = append(refs, dependencies...)
		for _, c := range d.TestCases {
			refs = append(refs, c.Input, c.Answer)
		}
		return plan, refs, nil
	})
}

func freezeSnapshot(ctx, lifetime context.Context, open referenceOpen, release func(), prepare func() (ReferencePlan, []fileReference, error)) (_ *ReferenceSnapshot, err error) {
	if !referenceSnapshotActive.CompareAndSwap(false, true) {
		if release != nil {
			release()
		}
		return nil, ErrBusy
	}
	s := &ReferenceSnapshot{ctx: lifetime, release: func() {
		if release != nil {
			release()
		}
		referenceSnapshotActive.Store(false)
	}}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.Close())
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	plan, refs, err := prepare()
	if err != nil {
		return nil, err
	}
	s.plan = plan
	s.caseStart = 1 + len(plan.Dependencies)
	f, err := os.CreateTemp("", "oj-reference-snapshot-*")
	if err != nil {
		return nil, errors.New("reference snapshot unavailable")
	}
	s.file = f
	if err := os.Remove(f.Name()); err != nil {
		return nil, errors.New("reference snapshot anonymity unavailable")
	}
	var offset int64
	for _, ref := range refs {
		if err := open.copy(ctx, s.file, ref); err != nil {
			return nil, err
		}
		s.spans = append(s.spans, referenceSpan{offset, ref.Size})
		offset += ref.Size
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s, nil
}

// Plan returns detached identities only; byte ranges and private data stay owned.
func (s *ReferenceSnapshot) Plan() ReferencePlan {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.plan
	p.Cases = append([]ReferenceCase(nil), p.Cases...)
	p.Dependencies = append([]ReferenceDependency(nil), p.Dependencies...)
	return p
}

// ReadSource returns only the bounded frozen source.
func (s *ReferenceSnapshot) ReadSource(ctx context.Context) ([]byte, error) {
	return s.read(ctx, 0)
}

// ReadAnswer supports complete checker-domain preflight without loading inputs.
func (s *ReferenceSnapshot) ReadAnswer(ctx context.Context, index int) ([]byte, error) {
	if index < 0 || index >= len(s.plan.Cases) {
		return nil, ErrInvalid
	}
	return s.read(ctx, s.caseStart+1+2*index)
}

// AnswerAt borrows a read-only span without exposing the backing file. Close
// invalidates all borrowed readers and synchronizes with reads in progress.
func (s *ReferenceSnapshot) AnswerAt(ctx context.Context, index int) (io.ReaderAt, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx == nil || s.closed || s.file == nil || index < 0 || index >= len(s.plan.Cases) {
		return nil, 0, ErrInvalid
	}
	if err := errors.Join(ctx.Err(), s.ctx.Err()); err != nil {
		return nil, 0, err
	}
	span := s.spans[s.caseStart+1+2*index]
	return snapshotAnswer{snapshot: s, ctx: ctx, span: span}, span.size, nil
}

// ReadInput returns one bounded frozen input without loading the answer.
func (s *ReferenceSnapshot) ReadInput(ctx context.Context, index int) ([]byte, error) {
	if index < 0 || index >= len(s.plan.Cases) {
		return nil, ErrInvalid
	}
	return s.read(ctx, s.caseStart+2*index)
}

type snapshotAnswer struct {
	snapshot *ReferenceSnapshot
	ctx      context.Context
	span     referenceSpan
}

func (a snapshotAnswer) ReadAt(p []byte, offset int64) (int, error) {
	s := a.snapshot
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || offset < 0 {
		return 0, ErrInvalid
	}
	if err := errors.Join(a.ctx.Err(), s.ctx.Err()); err != nil {
		return 0, err
	}
	if offset >= a.span.size {
		return 0, io.EOF
	}
	want := int(min(int64(len(p)), a.span.size-offset))
	n, err := s.file.ReadAt(p[:want], a.span.offset+offset)
	if e := errors.Join(a.ctx.Err(), s.ctx.Err()); e != nil {
		return 0, e
	}
	if n != want || (err != nil && err != io.EOF) {
		return n, errors.New("reference snapshot read failed")
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// ReadCase loads only one frozen input and answer. Callers release these slices
// before loading the next point; this API does not retain them internally.
func (s *ReferenceSnapshot) ReadCase(ctx context.Context, index int) ([]byte, []byte, error) {
	if index < 0 || index >= len(s.plan.Cases) {
		return nil, nil, ErrInvalid
	}
	in, err := s.read(ctx, s.caseStart+2*index)
	if err != nil {
		return nil, nil, err
	}
	answer, err := s.read(ctx, s.caseStart+1+2*index)
	if err != nil {
		return nil, nil, err
	}
	return in, answer, nil
}

func (s *ReferenceSnapshot) read(ctx context.Context, index int) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readLocked(ctx, index)
}

func (s *ReferenceSnapshot) readLocked(ctx context.Context, index int) ([]byte, error) {
	if ctx == nil || s.closed || index < 0 || index >= len(s.spans) {
		return nil, ErrInvalid
	}
	if err := errors.Join(ctx.Err(), s.ctx.Err()); err != nil {
		return nil, err
	}
	span := s.spans[index]
	raw := make([]byte, int(span.size))
	_, err := io.ReadFull(referenceContextReader{ctx, referenceContextReader{s.ctx, io.NewSectionReader(s.file, span.offset, span.size)}}, raw)
	if err != nil {
		return nil, errors.Join(errors.New("reference snapshot read failed"), ctx.Err(), s.ctx.Err())
	}
	if err := errors.Join(ctx.Err(), s.ctx.Err()); err != nil {
		return nil, err
	}
	return raw, nil
}

// Close is concurrency-safe and idempotent; failures never expose private paths.
func (s *ReferenceSnapshot) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		if s.file != nil && s.file.Close() != nil {
			s.err = errors.New("reference snapshot close failed")
		}
		if s.release != nil {
			s.release()
		}
	}
	return s.err
}

type referenceContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r referenceContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
