package main

import (
	"context"
	"log/slog"
	"time"
)

// startRecovery owns one scanner per controller. Joining it before closing the
// pool prevents shutdown from abandoning an in-flight recovery transaction.
func startRecovery(parent context.Context, logger *slog.Logger, recover func(context.Context) (int, error)) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runRecovery(ctx, logger, recover, time.Second)
	}()
	return func() {
		cancel()
		<-done
	}
}

func runRecovery(ctx context.Context, logger *slog.Logger, recover func(context.Context) (int, error), delay time.Duration) {
	for ctx.Err() == nil {
		scan, cancel := context.WithTimeout(ctx, 5*time.Second)
		count, err := recover(scan)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			// A batch may commit a prefix before failing. Database errors can
			// contain connection details; expose only the safe progress count.
			logger.Warn("Task recovery scan failed; retry scheduled", "recovered", count)
		} else if count > 0 {
			logger.Info("Task recovery scan completed", "recovered", count)
		}
		// Delay after completion even for full or failed batches, so neither
		// backlog nor an unavailable database creates an unbounded hot loop.
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
