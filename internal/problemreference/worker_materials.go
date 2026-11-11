package problemreference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// MaterialInput binds every private read to one targeted reservation.
type MaterialInput struct {
	ClaimInput
	Token string `json:"lease_token"`
}

// Valid validates syntax without granting access.
func (i MaterialInput) Valid() bool {
	return i.ClaimInput.Valid() && problem.ValidImportRevision(i.Token)
}

// MaterialAuthorization is a current read permission, never an execution grant.
// Deadline is local monotonic time, conservatively derived from database time.
type MaterialAuthorization struct {
	Plan     ExpectedPlan
	Deadline time.Time
}

// MaterialAuthorizer rechecks all original and current authority in a short tx.
type MaterialAuthorizer interface {
	AuthorizeReferenceMaterials(context.Context, submission.NodeIdentity, MaterialInput) (MaterialAuthorization, error)
}

type differentialMaterialAuthorizer interface {
	AuthorizeDifferentialMaterials(context.Context, submission.NodeIdentity, MaterialInput) (MaterialAuthorization, error)
}

// ErrDifferentialNotPrepared is observed only after authenticating a live
// reservation whose historical formal plan has no comparison metadata.
var ErrDifferentialNotPrepared = errors.New("reference comparison plan not recorded")

// MaterialDescriptor contains only the canonical, previously frozen plan.
type MaterialDescriptor struct {
	Encoding    []byte `json:"plan_encoding"`
	RemainingMS int64  `json:"remaining_ms"`
}

// DifferentialDescriptor describes only the separately persisted comparison.
type DifferentialDescriptor struct {
	DifferentialExpectedPlan
	RemainingMS int64 `json:"remaining_ms"`
}

// ReferenceMaterials couples live reservation checks to immutable object I/O.
type ReferenceMaterials struct {
	authority MaterialAuthorizer
	objects   problem.ReferenceObjectReader
}

// ReferenceFile owns one verified object and its read deadline resources.
type ReferenceFile struct {
	reader io.ReadCloser
	ctx    context.Context
	cancel context.CancelFunc
}

// Close releases both the private snapshot and the deadline context.
func (f *ReferenceFile) Close() error { f.cancel(); return f.reader.Close() }

// Read stops using the verified object when its final read permission expires.
func (f *ReferenceFile) Read(b []byte) (int, error) {
	if err := f.ctx.Err(); err != nil {
		return 0, err
	}
	return f.reader.Read(b)
}

// NewReferenceMaterials requires both explicit authority and private storage.
func NewReferenceMaterials(a MaterialAuthorizer, o problem.ReferenceObjectReader) (*ReferenceMaterials, error) {
	if a == nil || o == nil {
		return nil, ErrInvalid
	}
	return &ReferenceMaterials{a, o}, nil
}

// Describe reveals the plan only while the full reservation remains valid.
func (s *ReferenceMaterials) Describe(ctx context.Context, node submission.NodeIdentity, i MaterialInput) (MaterialDescriptor, error) {
	a, err := s.authority.AuthorizeReferenceMaterials(ctx, node, i)
	if err != nil {
		return MaterialDescriptor{}, err
	}
	remaining := time.Until(a.Deadline).Milliseconds()
	if remaining <= 0 || remaining > 30000 {
		return MaterialDescriptor{}, ErrClaimUnavailable
	}
	return MaterialDescriptor{bytes.Clone(a.Plan.Encoding), remaining}, nil
}

// Download reads only a listed object, releases SQL locks before I/O, then
// rechecks authority before returning any private byte. The caller must Close
// the snapshot and enforce the returned deadline on actual response writes.
func (s *ReferenceMaterials) Download(ctx context.Context, node submission.NodeIdentity, i MaterialInput, object materialstore.Object) (*ReferenceFile, time.Time, error) {
	return s.download(ctx, object, func(ctx context.Context) (MaterialAuthorization, error) {
		return s.authority.AuthorizeReferenceMaterials(ctx, node, i)
	}, formalObjectAllowed)
}

func formalObjectAllowed(a MaterialAuthorization, object materialstore.Object) bool {
	var plan problem.ReferencePlan
	if json.Unmarshal(a.Plan.Encoding, &plan) != nil {
		return false
	}
	objects, err := plan.MaterialObjects()
	if err != nil {
		return false
	}
	for _, o := range objects {
		if o == object {
			return true
		}
	}
	return false
}

// DescribeDifferential is independent of the formal assignment response.
func (s *ReferenceMaterials) DescribeDifferential(ctx context.Context, node submission.NodeIdentity, i MaterialInput) (DifferentialDescriptor, error) {
	a, err := s.authorizeDifferential(ctx, node, i)
	if err != nil {
		return DifferentialDescriptor{}, err
	}
	remaining := time.Until(a.Deadline).Milliseconds()
	if remaining <= 0 || remaining > 30000 {
		return DifferentialDescriptor{}, ErrClaimUnavailable
	}
	if a.Plan.Differential == nil {
		return DifferentialDescriptor{}, ErrDifferentialNotPrepared
	}
	return DifferentialDescriptor{DifferentialExpectedPlan{Encoding: bytes.Clone(a.Plan.Differential.Encoding), SHA256: a.Plan.Differential.SHA256}, remaining}, nil
}

// DownloadDifferential accepts only one of the two pinned comparison objects
// and binds both checks to the descriptor digest the Worker observed.
func (s *ReferenceMaterials) DownloadDifferential(ctx context.Context, node submission.NodeIdentity, i MaterialInput, digest string, object materialstore.Object) (*ReferenceFile, time.Time, error) {
	return s.download(ctx, object, func(ctx context.Context) (MaterialAuthorization, error) {
		return s.authorizeDifferential(ctx, node, i)
	}, func(a MaterialAuthorization, object materialstore.Object) bool {
		if a.Plan.Differential == nil || a.Plan.Differential.SHA256 != digest {
			return false
		}
		plan, err := a.Plan.Differential.CurrentPlan(i.PlanSHA256)
		if err != nil {
			return false
		}
		objects, err := plan.MaterialObjects(i.PlanSHA256)
		return err == nil && (object == objects[0] || object == objects[1])
	})
}

func (s *ReferenceMaterials) authorizeDifferential(ctx context.Context, node submission.NodeIdentity, i MaterialInput) (MaterialAuthorization, error) {
	authority, ok := s.authority.(differentialMaterialAuthorizer)
	if !ok {
		return MaterialAuthorization{}, ErrClaimUnavailable
	}
	a, err := authority.AuthorizeDifferentialMaterials(ctx, node, i)
	if err != nil {
		return MaterialAuthorization{}, err
	}
	return a, nil
}

func sameMaterialPlans(a, b ExpectedPlan) bool {
	if a.SHA256 != b.SHA256 || !bytes.Equal(a.Encoding, b.Encoding) {
		return false
	}
	if a.Differential == nil || b.Differential == nil {
		return a.Differential == nil && b.Differential == nil
	}
	return a.Differential.SHA256 == b.Differential.SHA256 && bytes.Equal(a.Differential.Encoding, b.Differential.Encoding)
}

func (s *ReferenceMaterials) download(ctx context.Context, object materialstore.Object,
	authorize func(context.Context) (MaterialAuthorization, error), allowed func(MaterialAuthorization, materialstore.Object) bool,
) (*ReferenceFile, time.Time, error) {
	a, err := authorize(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	if !allowed(a, object) {
		return nil, time.Time{}, ErrClaimUnavailable
	}
	ioctx, cancel := context.WithDeadline(ctx, a.Deadline)
	// Ownership of cancel follows the returned reader rather than this stack.
	file, err := s.objects.Open(ioctx, object)
	if err != nil {
		cancel()
		return nil, time.Time{}, errors.New("reference material unavailable")
	}
	if file == nil {
		cancel()
		return nil, time.Time{}, errors.New("reference material unavailable")
	}
	current, e := authorize(ioctx)
	if e != nil || !sameMaterialPlans(current.Plan, a.Plan) || file.Object != object || ioctx.Err() != nil {
		_ = file.Close()
		cancel()
		if e != nil {
			return nil, time.Time{}, e
		}
		return nil, time.Time{}, ErrClaimUnavailable
	}
	deadline := a.Deadline
	if current.Deadline.Before(deadline) {
		deadline = current.Deadline
	}
	readCtx, readCancel := context.WithDeadline(ioctx, deadline)
	return &ReferenceFile{reader: file, ctx: readCtx, cancel: func() { readCancel(); cancel() }}, deadline, nil
}
