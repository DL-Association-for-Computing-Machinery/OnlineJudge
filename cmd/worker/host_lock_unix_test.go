//go:build darwin || linux

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func takeHostSlot(t *testing.T, dir, name string, recovery, dual bool) *slotLock {
	t.Helper()
	lock, err := lockHostSlotAt(dir, name, recovery, dual)
	if err != nil {
		t.Fatal(err)
	}
	return lock
}

func rejectHostSlot(t *testing.T, dir, name string, recovery, dual bool) {
	t.Helper()
	if lock, err := lockHostSlotAt(dir, name, recovery, dual); err == nil {
		lock.Close()
		t.Fatal("unsafe acquisition was accepted")
	}
}

const hostLockFixtureEnv = "OJ_TEST_HOST_EXECUTION_LOCK_DIR"

func TestHostSlotAcrossProcesses(t *testing.T) {
	names := workerhost.ExecutionLockNames()
	if dir := os.Getenv(hostLockFixtureEnv); dir != "" {
		lock, err := lockHostSlotAt(dir, names[2], false, false)
		if errors.Is(err, os.ErrNotExist) {
			os.Exit(2)
		}
		if err != nil {
			os.Exit(3)
		}
		if err := lock.dirty(); err != nil {
			os.Exit(4)
		}
		// Simulate a process exit without cleanup or Stop. The kernel releases
		// both file locks; the persisted marker must survive.
		os.Exit(0)
	}
	dir := t.TempDir()
	runChild := func() error {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHostSlotAcrossProcesses$")
		cmd.Env = append(os.Environ(), hostLockFixtureEnv+"="+dir)
		return cmd.Run()
	}
	judge := takeHostSlot(t, dir, names[0], false, false)
	err := runChild()
	judge.Close()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("reference subprocess did not contend with judge: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, names[2])); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed host acquisition touched the target slot")
	}
	if err := runChild(); err != nil {
		t.Fatalf("reference subprocess could not acquire released host: %v", err)
	}
	rejectHostSlot(t, dir, names[0], false, false)
	rejectHostSlot(t, dir, names[1], false, true)
	recovery := takeHostSlot(t, dir, names[2], true, false)
	rejectHostSlot(t, dir, names[0], false, false)
	if err := recoverLocked(t.Context(), recovery, func(context.Context) error { return errors.New("stop not confirmed") }); err == nil {
		t.Fatal("failed Stop cleared the reference fence")
	}
	recovery.Close()
	rejectHostSlot(t, dir, names[0], false, false)
	recovery = takeHostSlot(t, dir, names[2], true, false)
	if err := recoverLocked(t.Context(), recovery, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	recovery.Close()
	takeHostSlot(t, dir, names[0], false, false).Close()
}

func TestHostSlotDualExperimentIsLimitedToM0(t *testing.T) {
	dir := t.TempDir()
	names := workerhost.ExecutionLockNames()
	first := takeHostSlot(t, dir, names[0], false, true)
	rejectHostSlot(t, dir, names[1], false, true) // A busy empty peer is ambiguous.
	if err := first.dirty(); err != nil {
		t.Fatal(err)
	}
	second := takeHostSlot(t, dir, names[1], false, true)
	if err := second.dirty(); err != nil {
		t.Fatal(err)
	}
	rejectHostSlot(t, dir, names[2], false, false)
	rejectHostSlot(t, dir, names[2], true, false)
	rejectHostSlot(t, dir, names[2], false, true)
	rejectHostSlot(t, dir, names[0], true, true)
	first.Close() // Leave the known dual marker.
	recovery := takeHostSlot(t, dir, names[0], true, true)
	if err := recoverLocked(t.Context(), recovery, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	recovery.Close()
	if err := second.clean(); err != nil {
		t.Fatal(err)
	}
	second.Close()
	takeHostSlot(t, dir, names[2], false, false).Close()
}

func TestHostSlotCannotRecoverDifferentRuntimeMarker(t *testing.T) {
	name := (workerhost.M0Slot{}).LockName()
	for _, ubuntu := range []bool{false, true} {
		dir := t.TempDir()
		l, err := lockHostSlotForRuntimeAt(dir, name, false, false, ubuntu)
		if err != nil {
			t.Fatal(err)
		}
		if err := l.dirty(); err != nil {
			t.Fatal(err)
		}
		l.Close()
		before, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if wrong, err := lockHostSlotForRuntimeAt(dir, name, true, false, !ubuntu); err == nil {
			wrong.Close()
			t.Fatal("a different runtime acquired the dirty slot")
		}
		after, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(after) != string(before) {
			t.Fatal("rejected recovery changed the quarantine")
		}
		l, err = lockHostSlotForRuntimeAt(dir, name, true, false, ubuntu)
		if err != nil {
			t.Fatal(err)
		}
		if err := recoverLocked(t.Context(), l, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		l.Close()
		takeHostSlot(t, dir, name, false, false).Close()
	}
}

func TestHostSlotChecksAllLegacyMarkersAndOnlyClearsSelected(t *testing.T) {
	names := workerhost.ExecutionLockNames()
	dir := t.TempDir()
	for _, name := range names[1:] {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("active\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	rejectHostSlot(t, dir, names[0], false, false)
	rejectHostSlot(t, dir, names[0], false, true)
	rejectHostSlot(t, dir, names[1], true, true)
	for i, name := range names[1:] {
		l := takeHostSlot(t, dir, name, true, false)
		if err := recoverLocked(t.Context(), l, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		l.Close()
		if i < len(names)-2 {
			rejectHostSlot(t, dir, names[0], false, false)
		}
	}
	takeHostSlot(t, dir, names[0], false, false).Close()
}

func TestHostSlotRejectsLegacyOwnerAndInvalidLock(t *testing.T) {
	names := workerhost.ExecutionLockNames()
	dir := t.TempDir()
	legacy, err := acquireLock(filepath.Join(dir, names[0]))
	if err != nil {
		t.Fatal(err)
	}
	rejectHostSlot(t, dir, names[1], false, false)
	rejectHostSlot(t, dir, names[1], false, true)
	legacy.Close()
	for _, marker := range []string{"active\n", "active-dual", dualMarker + "junk"} {
		if err := os.WriteFile(filepath.Join(dir, names[0]), []byte(marker), 0600); err != nil {
			t.Fatal(err)
		}
		rejectHostSlot(t, dir, names[1], false, true)
		rejectHostSlot(t, dir, names[0], true, true)
	}
	if err := os.Symlink(filepath.Join(dir, names[0]), filepath.Join(dir, names[2])); err != nil {
		t.Fatal(err)
	}
	rejectHostSlot(t, dir, names[0], true, false)
	if err := os.Chmod(filepath.Join(dir, hostLockName), 0660); err != nil {
		t.Fatal(err)
	}
	rejectHostSlot(t, dir, names[2], true, false)
}

func TestReferenceEntrypointsRejectDualExperiment(t *testing.T) {
	t.Setenv("OJ_SANDBOX_EXPERIMENT", "1")
	t.Setenv(dualModeEnv, "1")
	t.Setenv("OJ_WORKER_M0_SLOT", "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, args := range [][]string{{"validate-reference"}, {"recover-reference", strings.Repeat("a", 64)}} {
		if err := dispatch(t.Context(), args, logger, nil); err == nil || !strings.Contains(err.Error(), "slot") {
			t.Fatalf("reference entry did not reject dual mode: %v", err)
		}
	}
	if _, err := lockM0Execution(workerhost.M0Slot{}, false, "yes"); err == nil {
		t.Fatal("unknown dual setting did not fail closed")
	}
}
