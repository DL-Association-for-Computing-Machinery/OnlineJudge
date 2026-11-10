package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestReferenceClaimHoldsSharedHostAndTargetThroughRequest(t *testing.T) {
	dir := t.TempDir()
	input := problemreference.ClaimInput{RequestID: strings.Repeat("1", 32), SourceID: "library-checker:unionfind", PlanSHA256: strings.Repeat("a", 64)}
	slot, err := referenceClaimSlot(input)
	if err != nil || slot != (workerhost.ReferenceSlot{}).LockName() {
		t.Fatal("claim did not select the fixed unionfind slot", err)
	}
	judge := (workerhost.M0Slot{}).LockName()
	guard := func() (*slotLock, error) { return lockHostSlotAt(dir, slot, false, false) }
	calls := 0
	claim := func(_ context.Context, got problemreference.ClaimInput) (problemreference.WorkerClaim, error) {
		calls++
		if got != input {
			t.Fatal("claim changed operator selection")
		}
		rejectHostSlot(t, dir, judge, false, false)
		rejectHostSlot(t, dir, slot, true, false)
		if calls == 2 {
			return problemreference.WorkerClaim{}, workerclient.ErrClaimUnknown
		}
		return problemreference.WorkerClaim{RequestID: got.RequestID, SourceID: got.SourceID, PlanSHA256: got.PlanSHA256,
			Token: strings.Repeat("b", 64), Until: time.Now().Add(time.Minute)}, nil
	}
	held := takeHostSlot(t, dir, judge, false, false)
	if err := claimWithHostSlot(t.Context(), input, guard, claim); err == nil || calls != 0 {
		t.Fatal("claim reached network without a free host slot")
	}
	held.Close()
	if err := claimWithHostSlot(t.Context(), input, guard, claim); err != nil || calls != 1 {
		t.Fatal("expected one guarded claim", err)
	}
	if err := claimWithHostSlot(t.Context(), input, guard, claim); !errors.Is(err, workerclient.ErrClaimUnknown) || calls != 2 {
		t.Fatal("unknown response was retried or suppressed", err)
	}
	takeHostSlot(t, dir, judge, false, false).Close()
}

func TestReferenceClaimSelectsFixedSlot(t *testing.T) {
	names := workerhost.ExecutionLockNames()
	for source, want := range map[string]string{
		"aplusb": names[0], "unionfind": names[2], "convolution_mod": names[4],
		"zalgorithm": names[3], "suffixarray": names[3], "point_add_range_sum": names[3],
		"primality_test": names[3], "staticrmq": names[3], "static_range_sum": names[3], "associative_array": names[3],
	} {
		got, err := referenceClaimSlot(problemreference.ClaimInput{RequestID: strings.Repeat("1", 32),
			SourceID: "library-checker:" + source, PlanSHA256: strings.Repeat("a", 64)})
		if got != want || err != nil {
			t.Fatalf("wrong slot for %s: %s (%v)", source, got, err)
		}
	}
	got, err := referenceClaimSlot(problemreference.ClaimInput{RequestID: strings.Repeat("1", 32),
		SourceID: "library-checker:unknown", PlanSHA256: strings.Repeat("a", 64)})
	if got != "" || !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatal("unsupported source was assigned a physical slot")
	}
}
