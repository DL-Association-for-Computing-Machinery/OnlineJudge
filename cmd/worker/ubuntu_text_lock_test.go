//go:build darwin || linux

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestUbuntuTextDualOwnership(t *testing.T) {
	dir := t.TempDir()
	a, _ := workerhost.UbuntuTextSlot("primary")
	b, _ := workerhost.UbuntuTextSlot("secondary")
	if a.LockName() == b.LockName() || a.ContainerName() == b.ContainerName() {
		t.Fatal("Ubuntu text workers share ownership")
	}
	acquire := func(s workerhost.ReferenceTierSlot, recovery bool) (*slotLock, error) {
		return lockHostSlotForModes(dir, s.LockName(), recovery, false, false, false, true)
	}
	first, err := acquire(a, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.dirty(); err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := acquire(b, false)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := second.dirty(); err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []func() (*slotLock, error){
		func() (*slotLock, error) { return acquire(a, false) },
		func() (*slotLock, error) { return acquire(a, true) },
		func() (*slotLock, error) {
			return lockHostSlotForMarkerAt(dir, a.LockName(), false, false, false, true)
		},
		func() (*slotLock, error) { return lockHostSlotAt(dir, b.LockName(), false, true) },
	} {
		if got, err := attempt(); err == nil {
			got.Close()
			t.Fatal("a competing or recovery process entered a live scope")
		}
	}
}

func TestUbuntuTextDualQuarantine(t *testing.T) {
	dir := t.TempDir()
	a, _ := workerhost.UbuntuTextSlot("primary")
	b, _ := workerhost.UbuntuTextSlot("secondary")
	if err := os.WriteFile(filepath.Join(dir, b.LockName()), []byte(ubuntuTextDualMarker), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{a.LockName(), b.LockName()} {
		if got, err := lockHostSlotForModes(dir, name, false, false, false, false, true); err == nil {
			got.Close()
			t.Fatal("crashed slot did not fence a new dual process")
		}
	}
	if got, err := lockHostSlotAt(dir, b.LockName(), true, false); err == nil {
		got.Close()
		t.Fatal("M0 recovery accepted the Ubuntu text marker")
	}
	slot, err := lockHostSlotForModes(dir, b.LockName(), true, false, false, false, true)
	if err != nil {
		t.Fatal(err)
	}
	defer slot.Close()
	if err := slot.clean(); err != nil {
		t.Fatal(err)
	}
}
