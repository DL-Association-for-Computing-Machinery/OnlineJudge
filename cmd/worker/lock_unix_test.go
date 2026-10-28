//go:build darwin || linux

package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestSlotLockProcess(t *testing.T) {
	if path := os.Getenv("OJ_TEST_LOCK_HELPER"); path != "" {
		lock, err := acquireLock(path)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		if err := lock.dirty(); err != nil {
			t.Fatal(err)
		}
		os.Stdout.WriteString("locked\n")
		bufio.NewReader(os.Stdin).ReadString('\n')
		return // Simulated abrupt exit: leave the persistent marker.
	}
	path := filepath.Join(t.TempDir(), "slot")
	cmd := exec.Command(os.Args[0], "-test.run=^TestSlotLockProcess$")
	cmd.Env = append(os.Environ(), "OJ_TEST_LOCK_HELPER="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("helper: %q %v", line, err)
	}
	if l, err := acquireLock(path); err == nil {
		l.Close()
		t.Fatal("second process acquired slot")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	stdin.Close()
	if l, err := acquireLock(path); err == nil {
		l.Close()
		t.Fatal("crashed slot reused")
	}
	// Another fixed slot remains usable even while this slot is quarantined.
	secondary, _ := workerhost.ParseM0Slot("secondary")
	other, err := acquireLock(filepath.Join(filepath.Dir(path), secondary.LockName()))
	if err != nil {
		t.Fatal(err)
	}
	other.Close()
	// Only this test's private fixture is reset, simulating operator recovery.
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	l, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.dirty(); err != nil {
		t.Fatal(err)
	}
	if err := l.clean(); err != nil {
		t.Fatal(err)
	}
	l.Close()
	l, err = acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
}

func TestUnsafeLockFiles(t *testing.T) {
	for _, kind := range []string{"public", "symlink", "hardlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "lock")
			var err error
			switch kind {
			case "public":
				err = os.WriteFile(path, nil, 0644)
			case "directory":
				err = os.Mkdir(path, 0700)
			default:
				target := filepath.Join(dir, "target")
				if err := os.WriteFile(target, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink" {
					err = os.Symlink(target, path)
				} else {
					err = os.Link(target, path)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if l, err := acquireLock(path); err == nil {
				l.Close()
				t.Fatal("unsafe lock accepted")
			}
		})
	}
}
