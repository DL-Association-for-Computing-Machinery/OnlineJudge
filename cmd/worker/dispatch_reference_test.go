package main

import (
	"context"
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestReferenceDispatchDoesNotSelectWithoutHostSlotOrRunUnknown(t *testing.T) {
	dir := t.TempDir()
	judge := (workerhost.M0Slot{}).LockName()
	target := (workerhost.ReferenceSlot{}).LockName()
	guard := func() (*slotLock, error) { return lockHostSlotAt(dir, target, false, false) }
	calls := 0
	selectClaim := func(context.Context) (*problemreference.WorkerClaim, error) {
		calls++
		rejectHostSlot(t, dir, judge, false, false)
		rejectHostSlot(t, dir, target, true, false)
		return nil, workerclient.ErrClaimUnknown
	}
	run := func(context.Context, *slotLock, problemreference.WorkerClaim) error {
		t.Fatal("unknown dispatch reached execution")
		return nil
	}
	lock := takeHostSlot(t, dir, judge, false, false)
	if err := runSelectionUnderSlot(t.Context(), guard, selectClaim, run); err == nil || calls != 0 {
		t.Fatal("selection reached the network with a busy host")
	}
	lock.Close()
	if err := runSelectionUnderSlot(t.Context(), guard, selectClaim, run); !errors.Is(err, workerclient.ErrClaimUnknown) || calls != 1 {
		t.Fatal("unknown dispatch was retried or masked", err)
	}
	takeHostSlot(t, dir, judge, false, false).Close()
}
