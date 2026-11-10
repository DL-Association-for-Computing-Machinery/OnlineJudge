//go:build darwin || linux

package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

const (
	hostLockName = "host-execution.lock"
	dualModeEnv  = "OJ_WORKER_M0_DUAL_EXPERIMENT"
	dualMarker   = "active-dual-m0\n"
)

func lockM0Execution(slot workerhost.M0Slot, recovery bool, dualMode string) (*slotLock, error) {
	switch dualMode {
	case "", "1":
		return lockHostSlot(slot.LockName(), recovery, dualMode == "1")
	default:
		return nil, errors.New("unsupported M0 dual experiment setting")
	}
}

func lockHostSlot(name string, recovery, dual bool) (*slotLock, error) {
	dir, err := ownershipDirectory()
	if err != nil {
		return nil, err
	}
	return lockHostSlotAt(dir, name, recovery, dual)
}

func isM0Lock(name string) bool {
	secondary, _ := workerhost.ParseM0Slot("secondary")
	return name == (workerhost.M0Slot{}).LockName() || name == secondary.LockName()
}

func lockHostSlotAt(dir, name string, recovery, dual bool) (_ *slotLock, err error) {
	names := workerhost.ExecutionLockNames()
	if !slices.Contains(names[:], name) || (dual && !isM0Lock(name)) {
		return nil, errors.New("invalid canonical execution slot")
	}
	host, err := openOwnershipLock(filepath.Join(dir, hostLockName), true)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = host.Close()
		}
	}()
	how := syscall.LOCK_EX
	if dual {
		how = syscall.LOCK_SH
	}
	if err := flockOwnership(host, how, false); err != nil {
		return nil, errors.New("host execution slot unavailable")
	}
	// A crashed process loses flock but leaves a dirty per-slot marker. A
	// different target must not turn that crash into new execution capacity.
	for _, peer := range names {
		if peer != name {
			if err := inspectPeerLock(filepath.Join(dir, peer), dual && isM0Lock(peer), recovery && !dual); err != nil {
				return nil, err
			}
		}
	}
	slot, err := lockSlotFile(filepath.Join(dir, name), recovery)
	if err != nil {
		return nil, err
	}
	if dual && recovery && !allowsDualMarker(slot, true) {
		_ = slot.Close()
		return nil, errors.New("dual experiment cannot recover a single-slot marker")
	}
	return &slotLock{file: slot, host: host, dual: dual}, nil
}

func inspectPeerLock(path string, parallelM0, allowDirty bool) error {
	f, err := openOwnershipLock(path, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if parallelM0 {
		// A busy empty slot might belong to an older executable which has not
		// written its marker. Only an exact dual marker proves peer opt-in.
		lockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if lockErr != nil && !errors.Is(lockErr, syscall.EWOULDBLOCK) {
			return errors.New("cannot check another M0 slot")
		}
		if !allowsDualMarker(f, lockErr == nil) {
			return errors.New("another M0 slot requires single-slot recovery")
		}
		return nil
	}
	if err := flockOwnership(f, syscall.LOCK_EX, allowDirty); err != nil {
		return errors.New("another execution slot is active or quarantined")
	}
	return nil
}

func allowsDualMarker(f *os.File, allowEmpty bool) bool {
	var raw [len(dualMarker) + 1]byte
	n, err := f.ReadAt(raw[:], 0)
	return err == io.EOF && ((allowEmpty && n == 0) || string(raw[:n]) == dualMarker)
}
