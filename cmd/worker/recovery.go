package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"regexp"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func dispatch(ctx context.Context, args []string, logger *slog.Logger, drain <-chan struct{}) error {
	if len(args) == 0 {
		return run(ctx, logger, drain)
	}
	if args[0] == "validate-reference" {
		return validateReference(ctx, args[1:], logger)
	}
	if args[0] == "claim-reference" {
		return claimReference(ctx, args[1:], logger)
	}
	if args[0] == "run-reference" {
		return executeReference(ctx, args[1:], logger)
	}
	if args[0] == "run-next-reference" {
		return executeNextReference(ctx, args[1:], logger)
	}
	reference := args[0] == "recover-reference"
	sourceID, id := "library-checker:unionfind", ""
	if reference && len(args) == 3 {
		sourceID, id = args[1], args[2]
	} else if len(args) == 2 && (reference || args[0] == "recover") {
		id = args[1]
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(id) {
		return errors.New("usage: worker recover <original-id> | recover-reference [<admitted-source-id>] <original-id>")
	}
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" {
		return errors.New("explicit OJ_SANDBOX_EXPERIMENT=1 required")
	}
	if reference && (os.Getenv("OJ_WORKER_M0_SLOT") != "" || os.Getenv(dualModeEnv) != "") {
		return errors.New("reference recovery has its own fixed slot")
	}
	slot, err := workerhost.ParseM0Slot(os.Getenv("OJ_WORKER_M0_SLOT"))
	if err != nil {
		return err
	}
	var lock *slotLock
	var referenceSlot workerhost.ReferenceTierSlot
	if reference {
		_, tier, resolveErr := problemvalidation.ResolveSource(sourceID)
		if resolveErr != nil || tier == 0 {
			return errors.New("unsupported reference recovery source")
		}
		referenceSlot, err = workerhost.ReferenceSlotForTier(tier)
		if err != nil {
			return err
		}
		lock, err = lockSlotMode(referenceSlot.LockName(), true)
	} else {
		lock, err = lockM0Execution(slot, true, os.Getenv(dualModeEnv))
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := recoverLocked(ctx, lock, func(ctx context.Context) error {
		if reference {
			return workerhost.RecoverReferenceSlot(ctx, referenceSlot, id)
		}
		return workerhost.RecoverM0Slot(ctx, slot, id)
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
