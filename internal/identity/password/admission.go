package password

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sync/semaphore"
)

var (
	ErrAdmission    = errors.New("invalid password admission configuration")
	ErrBusy         = errors.New("password calculation admission full")
	ErrQueueTimeout = errors.New("password calculation queue wait expired")
)

// AdmissionLimits bounds work for one BoundedHasher instance. All account flows
// must share that instance to obtain a process-wide bound. These limits do not
// coordinate API processes or choose a production CPU/memory budget.
type AdmissionLimits struct {
	MaxInFlight  int
	MaxWaiting   int
	MaxQueueWait time.Duration
}

// BoundedHasher synchronously wraps password work with shared admission.
// A canceled running call retains its slot until the underlying work returns.
// Construct it once with NewBounded; the zero value fails closed. Do not copy it
// or mutate caller-owned password/record slices until the call returns.
type BoundedHasher struct {
	hasher  *Hasher
	slots   *semaphore.Weighted
	waiting chan struct{}
	wait    time.Duration
}

// NewBounded requires an initialized Hasher and explicit admission limits.
// MaxInFlight must be positive; MaxWaiting zero disables queuing. An enabled
// queue requires positive MaxQueueWait. No limit receives a default.
func NewBounded(hasher *Hasher, limits AdmissionLimits) (*BoundedHasher, error) {
	salt, digest := hasher.RecordSizeBounds()
	if salt == 0 || digest == 0 || limits.MaxInFlight <= 0 || limits.MaxWaiting < 0 ||
		limits.MaxQueueWait < 0 || (limits.MaxWaiting > 0 && limits.MaxQueueWait == 0) {
		return nil, ErrAdmission
	}
	return &BoundedHasher{
		hasher: hasher, slots: semaphore.NewWeighted(int64(limits.MaxInFlight)),
		waiting: make(chan struct{}, limits.MaxWaiting), wait: limits.MaxQueueWait,
	}, nil
}

func (h *BoundedHasher) check(ctx context.Context, password []byte) error {
	if h == nil || h.hasher == nil || h.slots == nil || h.waiting == nil || ctx == nil {
		return ErrAdmission
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return h.hasher.validatePassword(password)
}

// acquire transfers exactly one calculation slot only on success. Waiting
// capacity is released before calculation begins, including on every failure.
func (h *BoundedHasher) acquire(ctx context.Context) error {
	if h.slots.TryAcquire(1) {
		return nil
	}
	select {
	case h.waiting <- struct{}{}:
		defer func() { <-h.waiting }()
	default:
		if err := ctx.Err(); err != nil {
			return err
		}
		return ErrBusy
	}
	queue, cancel := context.WithTimeout(ctx, h.wait)
	defer cancel()
	if err := h.slots.Acquire(queue, 1); err != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return ErrQueueTimeout
	}
	return nil
}

// Create validates cheap input before admission and creates a current record.
// Queue expiry is separate from caller cancellation/deadline. Once calculation
// starts only the original context is checked, after synchronous work returns.
// Cancellation discards the result; it cannot interrupt Argon2 or salt I/O.
func (h *BoundedHasher) Create(ctx context.Context, password []byte) (Record, error) {
	if err := h.check(ctx, password); err != nil {
		return Record{}, err
	}
	if err := h.acquire(ctx); err != nil {
		return Record{}, err
	}
	defer h.slots.Release(1)
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	record, err := h.hasher.Create(password)
	if canceled := ctx.Err(); canceled != nil {
		clear(record.Salt)
		clear(record.Digest)
		return Record{}, canceled
	}
	return record, err
}

// Verify validates the complete record before admission. Caller cancellation
// discards both Match and NeedsUpgrade, including a correct password result.
// This is not a login, rate-limit, authorization, or anti-enumeration workflow.
func (h *BoundedHasher) Verify(ctx context.Context, password []byte, record Record) (Verification, error) {
	if err := h.check(ctx, password); err != nil {
		return Verification{}, err
	}
	if _, err := h.hasher.CheckRecord(record); err != nil {
		return Verification{}, err
	}
	if err := h.acquire(ctx); err != nil {
		return Verification{}, err
	}
	defer h.slots.Release(1)
	if err := ctx.Err(); err != nil {
		return Verification{}, err
	}
	result, err := h.hasher.Verify(password, record)
	if canceled := ctx.Err(); canceled != nil {
		return Verification{}, canceled
	}
	return result, err
}
