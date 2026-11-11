package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func executeReference(ctx context.Context, args []string, logger *slog.Logger) error {
	if len(args) != 3 || os.Getenv(dualModeEnv) != "" || os.Getenv("OJ_WORKER_M0_SLOT") != "" {
		return errors.New("usage: worker run-reference <request-id> <source-id> <expected-plan-sha256>; shared single host slot required")
	}
	input := problemreference.ClaimInput{RequestID: args[0], SourceID: args[1], PlanSHA256: args[2]}
	slot, err := referenceClaimSlot(input)
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
	// Large material reads must not queue a renewal behind their connection.
	renewal, err := workerclient.NewAcquirer(c.endpoint, c.roots, c.cert)
	if err != nil {
		return err
	}
	defer renewal.Close()
	lock, err := lockSlotMode(slot, false)
	if err != nil {
		return err
	}
	defer lock.Close()
	claim, err := client.ClaimReference(ctx, input)
	if err != nil {
		return err
	}
	fetch := func(ctx context.Context, claim problemreference.WorkerClaim) (referenceExecutionMaterials, error) {
		materials, err := client.FetchReferenceForExecution(ctx, claim)
		if err != nil {
			return referenceExecutionMaterials{}, err
		}
		return referenceExecutionMaterials{snapshot: materials.ReferenceSnapshot, close: materials.Close}, nil
	}
	err = runClaimedReference(ctx, lock, claim, fetch, renewal.RenewReference, startClaimedReference)
	if err == nil {
		logger.Info("参考执行完成，停止已确认；未提交验证报告")
	}
	return err
}

func startClaimedReference(ctx context.Context, sourceID string) (validationHost, error) {
	_, tier, err := problemvalidation.ResolveSource(sourceID)
	if err != nil {
		return nil, err
	}
	var host *workerhost.Host
	if tier == 0 {
		host, err = workerhost.StartM0Slot(ctx, workerhost.M0Slot{})
	} else {
		slot, e := workerhost.ReferenceSlotForTier(tier)
		if e != nil {
			return nil, e
		}
		host, err = workerhost.StartReferenceSlot(ctx, slot)
	}
	if host == nil {
		return nil, err
	}
	return host, err
}

type referenceExecutionMaterials struct {
	snapshot *problem.ReferenceSnapshot
	close    func() error
}

func runClaimedReference(ctx context.Context, lock *slotLock, claim problemreference.WorkerClaim,
	fetch func(context.Context, problemreference.WorkerClaim) (referenceExecutionMaterials, error),
	renew func(context.Context, problemreference.WorkerClaim) error,
	start func(context.Context, string) (validationHost, error)) error {
	_, tier, err := problemvalidation.ResolveSource(claim.SourceID)
	if err != nil {
		return err
	}
	// Close publishes the exact bound Host, including an ambiguous start
	// acknowledgement. Stop may be requested while the start call is in flight.
	startupDone := make(chan struct{})
	var host validationHost
	var marked bool
	work := func(ctx context.Context) (err error) {
		startupFinished := false
		defer func() {
			if !startupFinished {
				close(startupDone)
			}
		}()
		materials, err := fetch(ctx, claim)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, materials.close()) }()
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := lock.dirty(); err != nil {
			return err
		}
		marked = true
		startup, cancel := context.WithTimeout(ctx, 20*time.Second)
		host, err = start(startup, claim.SourceID)
		cancel()
		close(startupDone)
		startupFinished = true
		if host == nil {
			return errors.New("reference sandbox startup unconfirmed")
		}
		if err != nil {
			return err
		}
		g := execution.NewCachedGoJudge(host, host.Verify, host)
		if tier != 0 {
			g, err = execution.NewReferenceGoJudgeForTier(host, host.Verify, host, tier)
			if err != nil {
				return err
			}
		}
		report, err := problemvalidation.New(g).ValidateFrozen(ctx, materials.snapshot, claim.SourceID, claim.PlanSHA256)
		if err != nil {
			return err
		}
		if report.Verdict != "AC" {
			return errors.New("reference validation did not pass")
		}
		return nil
	}
	stop := func(ctx context.Context) error {
		select {
		case <-startupDone:
		case <-ctx.Done():
			return ctx.Err()
		}
		if !marked {
			return nil
		}
		if host == nil {
			return execution.ErrStopUnconfirmed
		}
		return host.Stop(ctx)
	}
	out := execution.RunReferenceLease(ctx, func(ctx context.Context) error { return renew(ctx, claim) }, work, stop)
	var cleanErr error
	if marked && out.StopErr == nil {
		cleanErr = lock.clean()
	}
	return errors.Join(out.WorkErr, out.StopErr, cleanErr)
}
