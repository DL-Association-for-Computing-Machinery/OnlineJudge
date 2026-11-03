package problem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
)

// UnionFindReferenceLimits bounds the independently admitted disk snapshot.
// It does not expand the existing small-material PrepareReference protocol.
func UnionFindReferenceLimits() ReferenceLimits {
	return ReferenceLimits{SourceBytes: 64 << 10, InputBytes: 4 << 20, AnswerBytes: 256 << 10, TotalBytes: 64 << 20, Cases: 64}
}

var referenceSnapshotActive atomic.Bool

type referenceSpan struct{ offset, size int64 }

// ReferenceSnapshot owns one fully verified anonymous disk file. At most one
// may be active in this process, and its Prepared remains occupied until Close.
// No original bundle file is read after preparation returns successfully.
type ReferenceSnapshot struct {
	mu     sync.Mutex
	file   *os.File
	ctx    context.Context
	owner  *Prepared
	plan   ReferencePlan
	spans  []referenceSpan // source, then each ordered input/answer pair
	closed bool
	err    error
}

// PrepareUnionFindReference freezes the complete source-locked unionfind draft.
// Every object is fully hashed using the same opened source before returning.
func (p *Prepared) PrepareUnionFindReference(ctx context.Context, limits ReferenceLimits) (_ *ReferenceSnapshot, err error) {
	if p == nil || ctx == nil || limits != UnionFindReferenceLimits() {
		return nil, ErrInvalid
	}
	return p.prepareDiskReference(ctx, "library-checker:unionfind", diskReferenceAdmission{"data_structure/unionfind", limits})
}

// PrepareDiskReference selects the complete source's fixed admission internally.
// No caller-supplied layout or budget can widen the shared snapshot protocol.
func (p *Prepared) PrepareDiskReference(ctx context.Context) (*ReferenceSnapshot, error) {
	if p == nil || ctx == nil {
		return nil, ErrInvalid
	}
	a, err := referenceDiskAdmission(p.content.SourceID)
	if err != nil {
		return nil, err
	}
	return p.prepareDiskReference(ctx, p.content.SourceID, a)
}

func (p *Prepared) prepareDiskReference(ctx context.Context, sourceID string, admission diskReferenceAdmission) (_ *ReferenceSnapshot, err error) {
	if err := p.acquire(); err != nil {
		return nil, err
	}
	if !referenceSnapshotActive.CompareAndSwap(false, true) {
		p.release()
		return nil, ErrBusy
	}
	s := &ReferenceSnapshot{ctx: ctx, owner: p}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.Close())
		}
	}()
	plan, d, err := p.referenceMetadata(ctx, admission.limits, sourceID, admission.path)
	if err != nil {
		return nil, err
	}
	s.plan = plan
	f, err := os.CreateTemp("", "oj-reference-snapshot-*")
	if err != nil {
		return nil, errors.New("reference snapshot unavailable")
	}
	s.file = f
	if err := os.Remove(f.Name()); err != nil {
		return nil, errors.New("reference snapshot anonymity unavailable")
	}
	refs := []fileReference{d.Provenance.OriginalFiles["source/"+admission.path+"/sol/correct.cpp"]}
	for _, c := range d.TestCases {
		refs = append(refs, c.Input, c.Answer)
	}
	var offset int64
	for _, ref := range refs {
		if err := s.append(ctx, ref); err != nil {
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

func (s *ReferenceSnapshot) append(ctx context.Context, ref fileReference) error {
	f, err := s.owner.openFile(ref.Path, ref.Size)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(s.file, h), io.LimitReader(referenceContextReader{ctx, f}, ref.Size+1))
	closeErr := f.Close()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if copyErr != nil || closeErr != nil {
		return errors.New("reference snapshot copy failed")
	}
	if n != ref.Size || hex.EncodeToString(h.Sum(nil)) != ref.SHA256 {
		return materialstore.ErrIntegrity
	}
	return nil
}

// Plan returns detached identities only; byte ranges and private data stay owned.
func (s *ReferenceSnapshot) Plan() ReferencePlan {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.plan
	p.Cases = append([]ReferenceCase(nil), p.Cases...)
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
	return s.read(ctx, 2+2*index)
}

// ReadCase loads only one frozen input and answer. Callers release these slices
// before loading the next point; this API does not retain them internally.
func (s *ReferenceSnapshot) ReadCase(ctx context.Context, index int) ([]byte, []byte, error) {
	if index < 0 || index >= len(s.plan.Cases) {
		return nil, nil, ErrInvalid
	}
	in, err := s.read(ctx, 1+2*index)
	if err != nil {
		return nil, nil, err
	}
	answer, err := s.read(ctx, 2+2*index)
	if err != nil {
		return nil, nil, err
	}
	return in, answer, nil
}

func (s *ReferenceSnapshot) read(ctx context.Context, index int) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		s.owner.release()
		referenceSnapshotActive.Store(false)
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
