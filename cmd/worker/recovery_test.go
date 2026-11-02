package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecoveryLockAndFence(t *testing.T) {
	for _, scenario := range []string{"success", "failure", "canceled", "clean-slot-failure", "write-failure", "clean-failure"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "slot")
			lock, err := acquireLock(path)
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "clean-slot-failure" {
				if err := lock.dirty(); err != nil {
					t.Fatal(err)
				}
			}
			lock.Close()
			lock, err = acquireLockMode(path, true)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "write-failure" {
				lock.Close()
			}
			called := false
			err = recoverLocked(ctx, lock, func(ctx context.Context) error {
				called = true
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 20*time.Second {
					t.Fatal("missing recovery budget")
				}
				if other, err := acquireLockMode(path, true); err == nil {
					other.Close()
					t.Fatal("concurrent recovery acquired slot")
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "active\n" {
					t.Fatal("termination began before persistent fence")
				}
				switch scenario {
				case "failure", "clean-slot-failure":
					return errors.New("private stop detail")
				case "canceled":
					cancel()
				case "clean-failure":
					lock.Close()
				}
				return nil
			})
			if (err == nil) != (scenario == "success") || (err != nil && strings.Contains(err.Error(), "private")) {
				t.Fatalf("recovery outcome: %v", err)
			}
			if called == (scenario == "write-failure") {
				t.Fatal("termination call did not respect marker failure")
			}
			lock.Close()
			ordinary, err := acquireLock(path)
			if ordinary != nil {
				ordinary.Close()
			}
			if (err == nil) != (scenario == "success") {
				t.Fatalf("ordinary reuse after recovery: %v", err)
			}
		})
	}
}

func TestRecoveryArguments(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	id := strings.Repeat("a", 64)
	for _, args := range [][]string{{"unknown"}, {"recover"}, {"recover", "short"}, {"recover", id, "extra"}} {
		if err := dispatch(context.Background(), args, logger, nil); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
	t.Setenv("OJ_SANDBOX_EXPERIMENT", "")
	if err := dispatch(context.Background(), []string{"recover", id}, logger, nil); err == nil {
		t.Fatal("recovery without opt-in accepted")
	}
	t.Setenv("OJ_SANDBOX_EXPERIMENT", "1")
	t.Setenv("OJ_WORKER_M0_SLOT", "unknown")
	if err := dispatch(context.Background(), []string{"recover", id}, logger, nil); err == nil {
		t.Fatal("arbitrary recovery slot accepted")
	}
}

type syncFailureFile struct {
	*os.File
	syncs int
}

func (f *syncFailureFile) Sync() error {
	f.syncs++
	if f.syncs == 2 { // dirty succeeds, post-truncate Sync fails, restored dirty succeeds.
		return errors.New("injected sync failure")
	}
	return f.File.Sync()
}

func TestRecoveryRestoresFenceAfterCleanSyncFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slot")
	lock, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	f := &syncFailureFile{File: lock.file.(*os.File)}
	lock.file = f
	if err := recoverLocked(context.Background(), lock, func(context.Context) error { return nil }); err == nil {
		t.Fatal("failed clean sync reported success")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "active\n" || f.syncs != 3 {
		t.Fatal("post-truncate sync failure did not restore persistent quarantine")
	}
	lock.Close()
	if ordinary, err := acquireLock(path); err == nil {
		ordinary.Close()
		t.Fatal("ordinary worker reused slot after failed clean sync")
	}
}
