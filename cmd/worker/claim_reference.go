package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func claimReference(ctx context.Context, args []string, logger *slog.Logger) error {
	if len(args) != 3 || os.Getenv(dualModeEnv) != "" || os.Getenv("OJ_WORKER_M0_SLOT") != "" {
		return errors.New("usage: worker claim-reference <request-id> <source-id> <expected-plan-sha256>; shared single host slot required")
	}
	input := problemreference.ClaimInput{RequestID: args[0], SourceID: args[1], PlanSHA256: args[2]}
	name, err := referenceClaimSlot(input)
	if err != nil {
		return err
	}
	c, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}
	client, err := workerclient.NewAcquirer(c.endpoint, c.roots, c.cert)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := claimWithHostSlot(ctx, input, func() (*slotLock, error) { return lockSlotMode(name, false) }, client.ClaimReference); err != nil {
		return err
	}
	logger.Info("参考验证领取已确认；本命令不执行题源")
	return nil
}

func referenceClaimSlot(input problemreference.ClaimInput) (string, error) {
	if !input.Valid() {
		return "", problemreference.ErrInvalid
	}
	_, tier, err := problemvalidation.ResolveSource(input.SourceID)
	if err != nil {
		return "", problemreference.ErrClaimUnavailable
	}
	if tier == 0 {
		return (workerhost.M0Slot{}).LockName(), nil
	}
	slot, err := workerhost.ReferenceSlotForTier(tier)
	return slot.LockName(), err
}

func claimWithHostSlot(ctx context.Context, input problemreference.ClaimInput, acquire func() (*slotLock, error),
	claim func(context.Context, problemreference.ClaimInput) (problemreference.WorkerClaim, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lock, err := acquire()
	if err != nil {
		return err
	}
	defer lock.Close()
	// No sandbox is opened: the kernel lock covers this one request, and no
	// dirty marker is written. Any future execution must retain the same lock.
	result, err := claim(ctx, input)
	if err == nil && !result.ValidFor(input) {
		return workerclient.ErrClaimUnknown
	}
	return err
}
