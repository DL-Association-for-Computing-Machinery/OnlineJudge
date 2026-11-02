package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"regexp"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func dispatch(ctx context.Context, args []string, logger *slog.Logger, drain <-chan struct{}) error {
	if len(args) == 0 {
		return run(ctx, logger, drain)
	}
	if len(args) != 2 || args[0] != "recover" || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(args[1]) {
		return errors.New("usage: worker [recover <original-full-container-id>]")
	}
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" {
		return errors.New("explicit OJ_SANDBOX_EXPERIMENT=1 required")
	}
	slot, err := workerhost.ParseM0Slot(os.Getenv("OJ_WORKER_M0_SLOT"))
	if err != nil {
		return err
	}
	lock, err := lockM0Mode(slot, true)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := recoverLocked(ctx, lock, func(ctx context.Context) error {
		return workerhost.RecoverM0Slot(ctx, slot, args[1])
	}); err != nil {
		return err
	}
	logger.Info("Worker 槽隔离已解除，沙箱保持停止；重启前须重新核验环境")
	return nil
}

func recoverLocked(ctx context.Context, lock *slotLock, confirm func(context.Context) error) error {
	// A clean slot is fenced too: every failed/ambiguous recovery stays closed.
	if err := lock.dirty(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := confirm(ctx); err != nil {
		return errors.New("M0 recovery unconfirmed; slot remains quarantined")
	}
	if ctx.Err() != nil {
		return errors.New("M0 recovery canceled; slot remains quarantined")
	}
	return lock.clean()
}
