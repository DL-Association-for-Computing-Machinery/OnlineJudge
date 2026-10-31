package password

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

type admissionResult struct {
	record Record
	verify Verification
	err    error
}

func admissionCall(h *BoundedHasher, verify bool, ctx context.Context, record Record) admissionResult {
	if verify {
		v, err := h.Verify(ctx, []byte("password"), record)
		return admissionResult{verify: v, err: err}
	}
	r, err := h.Create(ctx, []byte("password"))
	return admissionResult{record: r, err: err}
}

func startAdmission(h *BoundedHasher, verify bool, ctx context.Context, record Record) <-chan admissionResult {
	result := make(chan admissionResult, 1)
	go func() { result <- admissionCall(h, verify, ctx, record) }()
	return result
}

func awaitAdmission(t *testing.T, ch <-chan admissionResult) admissionResult {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("password call did not finish")
		return admissionResult{}
	}
}

func awaitAdmissionSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("password derivation did not enter")
	}
}

func awaitWaiting(t *testing.T, h *BoundedHasher, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for len(h.waiting) != count {
		if time.Now().After(deadline) {
			t.Fatalf("waiting calls = %d, want %d", len(h.waiting), count)
		}
		runtime.Gosched()
	}
}

func assertAdmissionError(t *testing.T, got admissionResult, want error) {
	t.Helper()
	if !errors.Is(got.err, want) || !reflect.DeepEqual(got.record, Record{}) || got.verify != (Verification{}) {
		t.Fatalf("expected zero result and %v, got error=%v record_zero=%v verify=%v",
			want, got.err, reflect.DeepEqual(got.record, Record{}), got.verify)
	}
}

func boundedForTest(t *testing.T, h *Hasher, limits AdmissionLimits) *BoundedHasher {
	t.Helper()
	b, err := NewBounded(h, limits)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAdmissionConfigurationFailsClosed(t *testing.T) {
	valid := AdmissionLimits{MaxInFlight: 1}
	for _, h := range []*Hasher{nil, {}} {
		if b, err := NewBounded(h, valid); b != nil || !errors.Is(err, ErrAdmission) {
			t.Fatal("accepted uninitialized hasher")
		}
	}
	for _, limits := range []AdmissionLimits{
		{}, {MaxInFlight: -1}, {MaxInFlight: 1, MaxWaiting: -1},
		{MaxInFlight: 1, MaxWaiting: 1}, {MaxInFlight: 1, MaxQueueWait: -1},
	} {
		if b, err := NewBounded(testHasher(t), limits); b != nil || !errors.Is(err, ErrAdmission) {
			t.Fatalf("accepted limits %+v", limits)
		}
	}
	for _, h := range []*BoundedHasher{nil, {}} {
		for _, verify := range []bool{false, true} {
			assertAdmissionError(t, admissionCall(h, verify, context.Background(), Record{}), ErrAdmission)
		}
	}
	b := boundedForTest(t, testHasher(t), valid)
	for _, verify := range []bool{false, true} {
		assertAdmissionError(t, admissionCall(b, verify, nil, Record{}), ErrAdmission)
	}
}

func TestAdmissionRejectsBeforeQueueOrDerivation(t *testing.T) {
	h := testHasher(t)
	h.derive = func([]byte, []byte, Parameters) []byte { t.Fatal("unexpected derivation"); return nil }
	b := boundedForTest(t, h, AdmissionLimits{MaxInFlight: 1, MaxWaiting: 1, MaxQueueWait: time.Second})
	if err := b.slots.Acquire(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	defer b.slots.Release(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, verify := range []bool{false, true} {
		assertAdmissionError(t, admissionCall(b, verify, ctx, Record{}), context.Canceled)
	}
	if r, err := b.Create(context.Background(), nil); !errors.Is(err, ErrPassword) || !reflect.DeepEqual(r, Record{}) {
		t.Fatal("invalid password was admitted")
	}
	assertAdmissionError(t, admissionCall(b, true, context.Background(), Record{}), ErrRecord)
	if len(b.waiting) != 0 {
		t.Fatal("invalid input occupied queue capacity")
	}
}

func TestAdmissionInFlightBoundAndSharedCreateVerify(t *testing.T) {
	h := testHasher(t)
	entered, release := make(chan struct{}, 3), make(chan struct{})
	var active, peak atomic.Int32
	h.derive = func(_ []byte, _ []byte, p Parameters) []byte {
		n := active.Add(1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		entered <- struct{}{}
		<-release
		active.Add(-1)
		return make([]byte, p.DigestBytes)
	}
	t.Cleanup(func() { close(release) })
	b := boundedForTest(t, h, AdmissionLimits{MaxInFlight: 2})
	r := vectorRecord(t)
	r.Digest = make([]byte, len(r.Digest))
	first := startAdmission(b, false, context.Background(), r)
	second := startAdmission(b, true, context.Background(), r)
	awaitAdmissionSignal(t, entered)
	awaitAdmissionSignal(t, entered)
	for _, verify := range []bool{false, true} {
		assertAdmissionError(t, admissionCall(b, verify, context.Background(), r), ErrBusy)
	}
	if peak.Load() != 2 || active.Load() != 2 {
		t.Fatal("shared in-flight bound was not exercised")
	}
	release <- struct{}{}
	release <- struct{}{}
	if got := awaitAdmission(t, first); got.err != nil || len(got.record.Digest) == 0 {
		t.Fatal("Create did not succeed")
	}
	if got := awaitAdmission(t, second); got.err != nil || !got.verify.Match || !got.verify.NeedsUpgrade {
		t.Fatal("Verify did not preserve legacy match")
	}
	if !b.slots.TryAcquire(2) {
		t.Fatal("successful calls leaked calculation slots")
	}
	b.slots.Release(2)
}

func TestAdmissionQueueFullTimeoutAndRecovery(t *testing.T) {
	h := testHasher(t)
	b := boundedForTest(t, h, AdmissionLimits{MaxInFlight: 1, MaxWaiting: 1, MaxQueueWait: time.Second})
	if err := b.slots.Acquire(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	queued := startAdmission(b, false, context.Background(), Record{})
	awaitWaiting(t, b, 1)
	assertAdmissionError(t, admissionCall(b, true, context.Background(), vectorRecord(t)), ErrBusy)
	assertAdmissionError(t, awaitAdmission(t, queued), ErrQueueTimeout)
	if len(b.waiting) != 0 || b.slots.TryAcquire(1) {
		t.Fatal("timeout leaked a waiter or released an unowned slot")
	}
	b.slots.Release(1)
	if got := admissionCall(b, false, context.Background(), Record{}); got.err != nil {
		t.Fatalf("timeout prevented future work: %v", got.err)
	}
}

func TestAdmissionCoreFailuresReleaseSlots(t *testing.T) {
	h := testHasher(t)
	h.salt = func([]byte) error { return errors.New("synthetic random failure") }
	b := boundedForTest(t, h, AdmissionLimits{MaxInFlight: 1})
	assertAdmissionError(t, admissionCall(b, false, context.Background(), Record{}), ErrRandom)
	for range 2 {
		got := admissionCall(b, true, context.Background(), vectorRecord(t))
		if got.err != nil || !got.verify.Match {
			t.Fatal("random failure leaked slot or changed verification")
		}
	}
	wrong, err := b.Verify(context.Background(), []byte("wrong"), vectorRecord(t))
	if err != nil || wrong != (Verification{}) {
		t.Fatal("wrong password behavior changed")
	}
	if !b.slots.TryAcquire(1) {
		t.Fatal("verification leaked slot")
	}
	b.slots.Release(1)
}
