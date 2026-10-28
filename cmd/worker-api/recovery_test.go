package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecoveryRetriesWithoutLeakingErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	var starts []time.Time
	const delay = 20 * time.Millisecond
	runRecovery(ctx, logger, func(context.Context) (int, error) {
		starts = append(starts, time.Now())
		switch len(starts) {
		case 1:
			return 2, errors.New("sensitive database connection detail")
		case 2:
			return 1, nil
		default:
			cancel()
			return 0, ctx.Err()
		}
	}, delay)
	if len(starts) != 3 {
		t.Fatalf("calls=%d", len(starts))
	}
	for i := 1; i < len(starts); i++ {
		if starts[i].Sub(starts[i-1]) < delay {
			t.Fatal("recovery retried without its delay")
		}
	}
	output := logs.String()
	if strings.Contains(output, "sensitive") || strings.Count(output, "level=WARN") != 1 ||
		!strings.Contains(output, "recovered=2") || !strings.Contains(output, "recovered=1") {
		t.Fatalf("unexpected recovery log: %s", output)
	}
}

func TestRecoveryStopCancelsAndDrains(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	stop := startRecovery(context.Background(), slog.Default(), func(ctx context.Context) (int, error) {
		calls.Add(1)
		if _, ok := ctx.Deadline(); !ok {
			t.Error("scan has no time budget")
		}
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-release
		return 0, ctx.Err()
	})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("startup did not scan immediately")
	}
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel active scan")
	}
	select {
	case <-done:
		t.Fatal("stop returned before active scan drained")
	default:
	}
	release <- struct{}{}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not join active scan")
	}
	if calls.Load() != 1 {
		t.Fatal("another scan began after cancellation")
	}
}

func TestCancelledRecoveryDoesNotScan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runRecovery(ctx, slog.Default(), func(context.Context) (int, error) {
		t.Fatal("cancelled controller scanned")
		return 0, nil
	}, time.Hour)
}
