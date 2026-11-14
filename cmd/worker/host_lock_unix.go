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
	ubuntuMarker         = "ubuntu-active\n"
	ubuntuAPlusBMarker   = "ubuntu-aplusb-active\n"
	activeMarker         = "active\n"
	hostLockName         = "host-execution.lock"
	dualModeEnv          = "OJ_WORKER_M0_DUAL_EXPERIMENT"
	dualMarker           = "active-dual-m0\n"
	ubuntuTextDualMarker = "ubuntu-text-dual-active\n"
)

func lockM0Execution(slot workerhost.M0Slot, recovery bool, dualMode string) (*slotLock, error) {
	return lockM0Runtime(slot, recovery, dualMode, false)
}

func lockM0Runtime(slot workerhost.M0Slot, recovery bool, dualMode string, ubuntu bool) (*slotLock, error) {
	if ubuntu {
		if slot != (workerhost.M0Slot{}) || dualMode != "" {
			return nil, errors.New("the Ubuntu M0 fixture requires the single primary slot")
		}
		dir, err := ownershipDirectory()
		if err != nil {
			return nil, err
		}
		return lockHostSlotForRuntimeAt(dir, slot.LockName(), recovery, false, true)
	}
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
	return lockHostSlotForRuntimeAt(dir, name, recovery, dual, false)
}

func lockHostSlotForRuntimeAt(dir, name string, recovery, dual, ubuntu bool) (_ *slotLock, err error) {
	return lockHostSlotForMarkerAt(dir, name, recovery, dual, ubuntu, false)
}

func lockReferenceRuntime(slot workerhost.ReferenceTierSlot, recovery bool) (*slotLock, error) {
	if !slot.IsUbuntuAPlusB() {
		return lockSlotMode(slot.LockName(), recovery)
	}
	dir, err := ownershipDirectory()
	if err != nil {
		return nil, err
	}
	return lockHostSlotForMarkerAt(dir, slot.LockName(), recovery, false, false, true)
}

func lockHostSlotForMarkerAt(dir, name string, recovery, dual, ubuntu, ubuntuAPlusB bool) (_ *slotLock, err error) {
	return lockHostSlotForModes(dir, name, recovery, dual, ubuntu, ubuntuAPlusB, false)
}

func lockUbuntuTextDual(slot workerhost.ReferenceTierSlot, recovery bool) (*slotLock, error) {
	if !slot.IsUbuntuAPlusB() {
		return nil, errors.New("dual text mode requires an Ubuntu text slot")
	}
	dir, err := ownershipDirectory()
	if err != nil {
		return nil, err
	}
	return lockHostSlotForModes(dir, slot.LockName(), recovery, false, false, false, true)
}

func isUbuntuTextLock(name string) bool {
	primary, _ := workerhost.UbuntuTextSlot("primary")
	secondary, _ := workerhost.UbuntuTextSlot("secondary")
	return name == primary.LockName() || name == secondary.LockName()
}

func lockHostSlotForModes(dir, name string, recovery, dual, ubuntu, ubuntuAPlusB, ubuntuTextDual bool) (_ *slotLock, err error) {
	names := workerhost.ExecutionLockNames()
	if !slices.Contains(names[:], name) || (dual && !isM0Lock(name)) ||
		(ubuntu && (dual || ubuntuAPlusB || name != (workerhost.M0Slot{}).LockName())) ||
		(ubuntuAPlusB && (dual || name != (workerhost.ReferenceSlot{}).LockName())) ||
		(ubuntuTextDual && (dual || ubuntu || ubuntuAPlusB || !isUbuntuTextLock(name))) {
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
	if dual || ubuntuTextDual && !recovery {
		how = syscall.LOCK_SH
	}
	if err := flockOwnership(host, how, false); err != nil {
		return nil, errors.New("host execution slot unavailable")
	}
	// A crashed process loses flock but leaves a dirty per-slot marker. A
	// different target must not turn that crash into new execution capacity.
	for _, peer := range names {
		if peer != name {
			peerMode := ""
			if dual && isM0Lock(peer) {
				peerMode = dualMarker
			}
			if ubuntuTextDual && !recovery && isUbuntuTextLock(peer) {
				peerMode = ubuntuTextDualMarker
			}
			if err := inspectPeerLock(filepath.Join(dir, peer), peerMode, recovery && !dual); err != nil {
				return nil, err
			}
		}
	}
	slot, err := lockSlotFile(filepath.Join(dir, name), recovery)
	if err != nil {
		return nil, err
	}
	marker := activeMarker
	if ubuntuTextDual {
		marker = ubuntuTextDualMarker
	} else if ubuntuAPlusB {
		marker = ubuntuAPlusBMarker
	} else if ubuntu {
		marker = ubuntuMarker
	} else if dual {
		marker = dualMarker
	}
	if recovery {
		var raw [max(len(activeMarker), len(dualMarker), len(ubuntuMarker), len(ubuntuAPlusBMarker), len(ubuntuTextDualMarker)) + 1]byte
		n, readErr := slot.ReadAt(raw[:], 0)
		saved := string(raw[:n])
		// Single-slot Desktop recovery can still recover a Desktop dual marker.
		if !dual && !ubuntu && !ubuntuAPlusB && !ubuntuTextDual && saved == dualMarker {
			marker = dualMarker
		}
		if readErr != io.EOF || (saved != "" && saved != marker) {
			_ = slot.Close()
			return nil, errors.New("quarantine marker does not match the selected sandbox runtime")
		}
	}
	return &slotLock{file: slot, host: host, dual: dual, marker: marker}, nil
}

func inspectPeerLock(path, peerMode string, allowDirty bool) error {
	f, err := openOwnershipLock(path, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if peerMode != "" {
		// A busy empty slot might belong to an older executable which has not
		// written its marker. Only an exact dual marker proves peer opt-in.
		lockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if lockErr != nil && !errors.Is(lockErr, syscall.EWOULDBLOCK) {
			return errors.New("cannot check another M0 slot")
		}
		if !allowsPeerMarker(f, lockErr == nil, peerMode) {
			return errors.New("another slot has a different runtime or unknown ownership")
		}
		if peerMode == ubuntuTextDualMarker && lockErr == nil && !allowsPeerMarker(f, true, "") {
			return errors.New("stopped Ubuntu text peer requires exclusive recovery")
		}
		return nil
	}
	if err := flockOwnership(f, syscall.LOCK_EX, allowDirty); err != nil {
		return errors.New("another execution slot is active or quarantined")
	}
	return nil
}

func allowsPeerMarker(f *os.File, allowEmpty bool, marker string) bool {
	var raw [max(len(dualMarker), len(ubuntuTextDualMarker)) + 1]byte
	n, err := f.ReadAt(raw[:], 0)
	return err == io.EOF && ((allowEmpty && n == 0) || string(raw[:n]) == marker)
}
