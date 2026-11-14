package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"regexp"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func dispatch(ctx context.Context, args []string, logger *slog.Logger, drain <-chan struct{}) error {
	if len(args) == 0 {
		return run(ctx, logger, drain)
	}
	if os.Getenv("OJ_EXPERIMENT_MATERIAL_VERSION") == experiment.UbuntuVersion && args[0] != "recover" {
		return errors.New("the Ubuntu M0 fixture supports only the training Worker and its recovery")
	}
	if args[0] == "validate-reference" {
		return validateReference(ctx, args[1:], logger)
	}
	if args[0] == "validate-checker-candidate" {
		return validateCheckerCandidate(ctx, args[1:], logger)
	}
	if args[0] == "validate-package-quality" {
		return validatePackageQuality(ctx, args[1:])
	}
	if args[0] == "judge-aplusb-isolated" {
		return runCheckerCommand(ctx, args[1:], logger, true)
	}
	if args[0] == "recover-aplusb-ubuntu" {
		return recoverAPlusBUbuntu(ctx, args[1:], logger)
	}
	if args[0] == "validate-convolution-differential" {
		return validateConvolutionDifferential(ctx, args[1:], logger)
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
	if !originalContainerID.MatchString(id) {
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
	ubuntu := false
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
		fixture, fixtureErr := m0Fixture(os.Getenv, slot)
		if fixtureErr != nil {
			return fixtureErr
		}
		ubuntu = fixture.Version() == experiment.UbuntuVersion
		lock, err = lockM0Runtime(slot, true, os.Getenv(dualModeEnv), ubuntu)
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := recoverLocked(ctx, lock, func(ctx context.Context) error {
		if reference {
			return workerhost.RecoverReferenceSlot(ctx, referenceSlot, id)
		}
		if ubuntu {
			return workerhost.RecoverUbuntuM0(ctx, id)
		}
		return workerhost.RecoverM0Slot(ctx, slot, id)
	}); err != nil {
		return err
	}
	logger.Info("当前沙箱停止已确认，槽隔离标记已清；仅在旧控制操作已排空时重启并重新核验环境")
	return nil
}

var originalContainerID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func recoverAPlusBUbuntu(ctx context.Context, args []string, logger *slog.Logger) error {
	if len(args) != 1 || !originalContainerID.MatchString(args[0]) ||
		os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || os.Getenv(dualModeEnv) != "" {
		return errors.New("recover-aplusb-ubuntu requires experiment opt-in, the original full ID and the single reference slot")
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	err := workerhost.CheckUbuntuAPlusBHost(probe)
	cancel()
	if err != nil {
		return err
	}
	slot, err := workerhost.ReferenceSlotForTier(experiment.ReferenceAPlusBUbuntu)
	if err != nil {
		return err
	}
	lock, err := lockReferenceRuntime(slot, true)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := recoverLocked(ctx, lock, func(ctx context.Context) error {
		return workerhost.RecoverReferenceSlot(ctx, slot, args[0])
	}); err != nil {
		return err
	}
	logger.Info("Ubuntu A+B 沙箱停止已确认，槽隔离标记已清")
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
