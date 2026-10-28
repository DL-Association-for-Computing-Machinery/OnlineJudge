// Command worker runs one exclusively owned, pinned M0 execution slot.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, logger); err != nil {
		logger.Error("Worker 已停止", "error", err.Error())
		os.Exit(1)
	}
	logger.Info("Worker 已停止，沙箱停止已确认")
}
