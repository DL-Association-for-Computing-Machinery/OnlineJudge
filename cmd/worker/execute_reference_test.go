package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

type leasedReferenceHost struct {
	referenceHost
	beforeStop func()
	stops      int
}

func (h *leasedReferenceHost) Stop(ctx context.Context) error {
	h.stops++
	h.beforeStop()
	return h.referenceHost.Stop(ctx)
}

func claimedReferenceFixture(t *testing.T, source []byte) (problemreference.WorkerClaim, func(context.Context, problemreference.WorkerClaim) (referenceExecutionMaterials, error)) {
	t.Helper()
	b := problemfixture.New(t)
	b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", source)
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	plan, err := p.PrepareReference(t.Context(), problem.SmallReferenceLimits())
	if err != nil {
		t.Fatal(err)
	}
	plan.SourceData = nil
	plan.SourceLockSHA256 = problemvalidation.SourceLockSHA256
	for i := range plan.Cases {
		plan.Cases[i].InputData = nil
		plan.Cases[i].AnswerData = nil
	}
	claim := problemreference.WorkerClaim{RequestID: strings.Repeat("1", 32), SourceID: plan.SourceID, PlanSHA256: plan.Digest(), Token: strings.Repeat("a", 64), Until: time.Now().Add(time.Minute)}
	return claim, func(ctx context.Context, _ problemreference.WorkerClaim) (referenceExecutionMaterials, error) {
		s, e := plan.FreezeMaterials(ctx, func(_ context.Context, o materialstore.Object) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(b.Files["objects/sha256/"+o.SHA256])), nil
		})
		if e != nil {
			return referenceExecutionMaterials{}, e
		}
		return referenceExecutionMaterials{s, s.Close}, nil
	}
}

func TestClaimedReferenceHoldsHostUntilConfirmedStop(t *testing.T) {
	claim, fetch := claimedReferenceFixture(t, []byte("synthetic reference; fake sandbox only"))
	for _, kind := range []string{"confirmed", "start reply lost", "cancel during start", "stop unknown", "no bound host", "preflight refused"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			slot := (workerhost.M0Slot{}).LockName()
			other := (workerhost.ReferenceSlot{}).LockName()
			lock := takeHostSlot(t, dir, slot, false, false)
			checkHeld := func() {
				if peer, err := lockHostSlotAt(dir, other, false, false); err == nil {
					peer.Close()
					t.Error("competing slot entered while reference held the host")
				}
			}
			h := &leasedReferenceHost{beforeStop: checkHeld}
			if kind == "stop unknown" {
				h.stopErr = errors.New("stop unconfirmed")
			}
			fetched, started := false, false
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			err := runClaimedReference(ctx, lock, claim, func(ctx context.Context, _ problemreference.WorkerClaim) (referenceExecutionMaterials, error) {
				checkHeld()
				fetched = true
				return fetch(ctx, claim)
			}, func(context.Context, problemreference.WorkerClaim) error {
				checkHeld()
				if kind == "preflight refused" {
					return errors.New("unknown renewal")
				}
				return nil
			}, func(startup context.Context, _ string) (validationHost, error) {
				checkHeld()
				started = true
				if kind == "start reply lost" {
					return h, errors.New("ambiguous start")
				}
				if kind == "cancel during start" {
					cancel()
					<-startup.Done()
					return h, startup.Err()
				}
				if kind == "no bound host" {
					return nil, errors.New("unbound start")
				}
				return h, nil
			})
			lock.Close()
			if (err == nil) != (kind == "confirmed") {
				t.Fatal("unexpected lifecycle result", err)
			}
			wantWork := kind != "preflight refused"
			wantStop := wantWork && kind != "no bound host"
			if fetched != wantWork || started != wantWork || (h.stops == 1) != wantStop {
				t.Fatalf("wrong lifecycle: fetch=%v start=%v stop=%d", fetched, started, h.stops)
			}
			quarantined := kind == "stop unknown" || kind == "no bound host"
			if errors.Is(err, execution.ErrStopUnconfirmed) != quarantined {
				t.Fatal("wrong stop classification", err)
			}
			if quarantined {
				rejectHostSlot(t, dir, other, false, false)
			} else {
				takeHostSlot(t, dir, other, false, false).Close()
			}
		})
	}
}
