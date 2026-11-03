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
	if args[0] == "validate-reference" {
		return validateReference(ctx, args[1:], logger)
	}
	if len(args) != 2 || (args[0] != "recover" && args[0] != "recover-reference") || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(args[1]) {
		return errors.New("usage: worker [recover|recover-reference <original-full-container-id>]")
	}
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" {
		return errors.New("explicit OJ_SANDBOX_EXPERIMENT=1 required")
	}
	reference := args[0] == "recover-reference"
	if reference && os.Getenv("OJ_WORKER_M0_SLOT") != "" {
		return errors.New("reference recovery has its own fixed slot")
	}
	slot, err := workerhost.ParseM0Slot(os.Getenv("OJ_WORKER_M0_SLOT"))
	if err != nil {
		return err
	}
	var lock *slotLock
	if reference {
		lock, err = lockSlotMode(workerhost.ReferenceSlot{}.LockName(), true)
	} else {
		lock, err = lockM0Mode(slot, true)
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := recoverLocked(ctx, lock, func(ctx context.Context) error {
		if reference {
			return workerhost.RecoverReference(ctx, args[1])
		}
		return workerhost.RecoverM0Slot(ctx, slot, args[1])
	}); err != nil {
		return err
	}
	logger.Info("当前沙箱停止已确认，槽隔离标记已清；仅在旧控制操作已排空时重启并重新核验环境")
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
