package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
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

func (h *leasedReferenceHost) Identity() (string, string) {
	return strings.Repeat("a", 64), "2026-11-11T00:00:00Z"
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
		return referenceExecutionMaterials{snapshot: s, close: s.Close}, nil
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
			var firstReport []byte
			reports := 0
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			err := runClaimedReference(ctx, lock, claim, nil, func(ctx context.Context, _ problemreference.WorkerClaim) (referenceExecutionMaterials, error) {
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
			}, func(_ context.Context, input problemreference.WorkerReportInput) error {
				checkHeld()
				reports++
				if h.stops != 1 {
					t.Error("reported before stop")
				}
				if reports == 1 {
					firstReport = bytes.Clone(input.Evidence)
					return workerclient.ErrReportUnknown
				}
				if !bytes.Equal(firstReport, input.Evidence) || input.Token != claim.Token {
					t.Error("retry changed frozen report binding")
				}
				return nil
			})
			lock.Close()
			if (err == nil) != (kind == "confirmed") {
				t.Fatal("unexpected lifecycle result", err)
			}
			if (kind == "confirmed" && reports != 2) || (kind != "confirmed" && reports != 0) {
				t.Fatal("unexpected report sends", reports)
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

type referenceUploadTimeoutHost struct {
	referenceHost
}

func (h *referenceUploadTimeoutHost) UploadFile(ctx context.Context, data []byte) (string, error) {
	if h.requests == 1 {
		return "", errors.Join(context.DeadlineExceeded, errors.New("PRIVATE-upload-detail"))
	}
	return h.referenceHost.UploadFile(ctx, data)
}

func TestClaimedReferenceLogsPointUploadAndStopOutcome(t *testing.T) {
	claim, fetch := claimedReferenceFixture(t, []byte("PRIVATE-source"))
	for _, stopState := range []string{"confirmed", "unconfirmed"} {
		t.Run(stopState, func(t *testing.T) {
			dir := t.TempDir()
			lock := takeHostSlot(t, dir, (workerhost.M0Slot{}).LockName(), false, false)
			defer lock.Close()
			h := &referenceUploadTimeoutHost{}
			if stopState == "unconfirmed" {
				h.stopErr = errors.Join(context.DeadlineExceeded, errors.New("PRIVATE-stop-detail"))
			}
			var log bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&log, nil))
			err := runClaimedReference(t.Context(), lock, claim, logger, fetch,
				func(context.Context, problemreference.WorkerClaim) error { return nil },
				func(context.Context, string) (validationHost, error) { return h, nil },
				func(context.Context, problemreference.WorkerReportInput) error {
					t.Error("failed work sent a report")
					return nil
				})
			if !errors.Is(err, context.DeadlineExceeded) || h.stops != 1 || !h.independent {
				t.Fatalf("upload failure did not preserve its cause and require an independent stop: %v", err)
			}
			if errors.Is(err, execution.ErrStopUnconfirmed) != (stopState == "unconfirmed") {
				t.Fatalf("wrong returned stop classification: %v", err)
			}
			var event struct {
				Level      string                                 `json:"level"`
				Source     string                                 `json:"source_id"`
				Stop       string                                 `json:"stop_state"`
				Execution  *problemvalidation.ExecutionDiagnostic `json:"execution_diagnostic"`
				StopDetail *workerhost.StopDiagnostic             `json:"stop_diagnostic"`
			}
			if err := json.Unmarshal(log.Bytes(), &event); err != nil {
				t.Fatal("expected exactly one structured failure event", err)
			}
			if event.Level != "ERROR" || event.Source != claim.SourceID || event.Stop != stopState || event.Execution == nil || len(event.Execution.Entries) == 0 {
				t.Fatalf("missing execution or stop evidence: %+v", event)
			}
			first := event.Execution.Entries[0]
			if first.Operation != "run" || first.Point != 1 || first.Phase != "upload" || first.Role != "input" || first.Code != "deadline" {
				t.Fatalf("wrong upload failure location: %+v", first)
			}
			if stopState == "unconfirmed" {
				if event.StopDetail == nil || event.StopDetail.Code != "deadline" {
					t.Fatal("missing stop deadline evidence")
				}
			} else if event.StopDetail != nil {
				t.Fatal("confirmed stop logged a failure diagnostic")
			}
			for _, private := range []string{"PRIVATE", claim.RequestID, claim.PlanSHA256, claim.Token, "compiled", "upload-1"} {
				if strings.Contains(log.String(), private) {
					t.Fatal("structured event leaked private execution data")
				}
			}
		})
	}
}
