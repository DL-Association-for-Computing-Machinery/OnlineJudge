package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
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
	return executeReferenceSelection(ctx, slot, logger, func(ctx context.Context, client *workerclient.Acquirer) (*problemreference.WorkerClaim, error) {
		claim, err := client.ClaimReference(ctx, input)
		return &claim, err
	})
}

func executeNextReference(ctx context.Context, args []string, logger *slog.Logger) error {
	if len(args) < 1 || len(args) > 2 || os.Getenv(dualModeEnv) != "" || os.Getenv("OJ_WORKER_M0_SLOT") != "" {
		return errors.New("usage: worker run-next-reference <source-id> [after-id]; shared single host slot required")
	}
	input := problemreference.DispatchInput{SourceID: args[0]}
	if len(args) == 2 {
		input.After = args[1]
	}
	if !input.Valid() {
		return problemreference.ErrInvalid
	}
	slot, err := referenceSlotForSource(input.SourceID)
	if err != nil {
		return err
	}
	return executeReferenceSelection(ctx, slot, logger, func(ctx context.Context, client *workerclient.Acquirer) (*problemreference.WorkerClaim, error) {
		result, err := client.DispatchReference(ctx, input)
		if err == nil && result.Claim == nil {
			logger.Info("该页没有可领取的参考验证申请", "next_after", result.After)
		}
		return result.Claim, err
	})
}

func executeReferenceSelection(ctx context.Context, slot string, logger *slog.Logger,
	selectClaim func(context.Context, *workerclient.Acquirer) (*problemreference.WorkerClaim, error)) error {
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
	fetch := func(ctx context.Context, claim problemreference.WorkerClaim) (referenceExecutionMaterials, error) {
		materials, err := client.FetchReferenceForExecution(ctx, claim)
		if err != nil {
			return referenceExecutionMaterials{}, err
		}
		frozen := referenceExecutionMaterials{snapshot: materials.ReferenceSnapshot, close: materials.Close}
		if claim.SourceID == problemvalidation.ConvolutionSourceID {
			comparison, err := client.FetchDifferential(ctx, claim)
			switch {
			case err == nil:
				frozen.comparison = &comparison
			case errors.Is(err, problemreference.ErrDifferentialNotPrepared):
				// Only an authenticated observation of a historical NULL plan
				// permits the original formal-only execution.
			default:
				return referenceExecutionMaterials{}, errors.Join(err, materials.Close())
			}
		}
		return frozen, nil
	}
	return runSelectionUnderSlot(ctx, func() (*slotLock, error) { return lockSlotMode(slot, false) },
		func(ctx context.Context) (*problemreference.WorkerClaim, error) { return selectClaim(ctx, client) },
		func(ctx context.Context, lock *slotLock, claim problemreference.WorkerClaim) error {
			err := runClaimedReference(ctx, lock, claim, logger, fetch, renewal.RenewReference, startClaimedReference, client.ReportReference)
			if err == nil {
				logger.Info("参考执行通过，停止已确认，报告已接纳")
			}
			return err
		})
}

func runSelectionUnderSlot(ctx context.Context, acquire func() (*slotLock, error),
	selectClaim func(context.Context) (*problemreference.WorkerClaim, error),
	run func(context.Context, *slotLock, problemreference.WorkerClaim) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lock, err := acquire()
	if err != nil {
		return err
	}
	defer lock.Close()
	claim, err := selectClaim(ctx)
	if err != nil || claim == nil {
		return err
	}
	return run(ctx, lock, *claim)
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
	snapshot   *problem.ReferenceSnapshot
	close      func() error
	comparison *problemvalidation.ConvolutionSources
}

func runClaimedReference(ctx context.Context, lock *slotLock, claim problemreference.WorkerClaim, logger *slog.Logger,
	fetch func(context.Context, problemreference.WorkerClaim) (referenceExecutionMaterials, error),
	renew func(context.Context, problemreference.WorkerClaim) error,
	start func(context.Context, string) (validationHost, error),
	send func(context.Context, problemreference.WorkerReportInput) error) error {
	profile, tier, err := problemvalidation.ResolveSource(claim.SourceID)
	if err != nil {
		return err
	}
	// Close publishes the exact bound Host, including an ambiguous start
	// acknowledgement. Stop may be requested while the start call is in flight.
	startupDone := make(chan struct{})
	var host validationHost
	var marked bool
	var report problemvalidation.Report
	var comparison *problemvalidation.ConvolutionReport
	var comparisonPlan problemvalidation.ConvolutionPlan
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
		if materials.comparison != nil {
			if tier != experiment.ReferenceConvolution {
				return problemreference.ErrInvalid
			}
			comparisonPlan, err = materials.comparison.DraftPlan(claim.PlanSHA256)
			if err != nil {
				return err
			}
		}
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
		if materials.comparison != nil {
			var done context.CancelFunc
			ctx, done = context.WithTimeout(ctx, profile.Wall)
			defer done()
		}
		report, err = problemvalidation.New(g).ValidateFrozen(ctx, materials.snapshot, claim.SourceID, claim.PlanSHA256)
		if err != nil || report.Verdict != "AC" || materials.comparison == nil {
			return err
		}
		result, err := problemvalidation.RunSmallConvolution(ctx, *materials.comparison, func() (execution.Sandbox, error) {
			return execution.NewConvolutionComparisonGoJudge(host, host.Verify, host)
		})
		// A joined mismatch/cleanup error is not determinate.
		switch err {
		case nil:
			result.Status = "passed"
		case problemvalidation.ErrConvolutionMismatch:
			result.Status = "mismatch"
		default:
			return err
		}
		comparison = &result
		return ctx.Err()
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
	if err := errors.Join(out.WorkErr, out.StopErr, cleanErr, ctx.Err()); err != nil {
		// The lease joins Work before returning, so these diagnostics are final.
		logClaimedReferenceFailure(logger, claim.SourceID, report.ExecutionDiagnostic, marked, out.StopErr, cleanErr)
		return err
	}
	report.Slot = (workerhost.M0Slot{}).ContainerName()
	if tier != 0 {
		slot, err := workerhost.ReferenceSlotForTier(tier)
		if err != nil {
			return err
		}
		report.Slot = slot.ContainerName()
	}
	report.ContainerID, report.StartedAt = host.Identity()
	if err := report.Finalize(nil, nil, nil); err != nil {
		return err
	}
	var input problemreference.WorkerReportInput
	if comparison == nil {
		input, err = problemreference.FreezeWorkerReport(claim, report)
	} else {
		comparison.StopConfirmed = true
		comparison.ContainerID, comparison.StartedAt = report.ContainerID, report.StartedAt
		input, err = problemreference.FreezeWorkerReportWithComparison(claim, report, comparisonPlan, *comparison)
	}
	if err != nil {
		return err
	}
	if err := sendFrozenReferenceReport(ctx, input, send); err != nil {
		return err
	}
	if report.Verdict != "AC" {
		return errors.New("reference validation did not pass")
	}
	if comparison != nil && comparison.Status == "mismatch" {
		return problemvalidation.ErrConvolutionMismatch
	}
	return nil
}

func logClaimedReferenceFailure(logger *slog.Logger, sourceID string, diagnostic *problemvalidation.ExecutionDiagnostic, marked bool, stopErr, cleanErr error) {
	if logger == nil {
		return
	}
	stopState := "not_attempted"
	switch {
	case stopErr != nil:
		stopState = "unconfirmed"
	case marked:
		stopState = "confirmed"
	}
	attrs := []slog.Attr{slog.String("source_id", sourceID), slog.String("stop_state", stopState), slog.Bool("slot_cleanup_failed", cleanErr != nil)}
	if diagnostic != nil {
		attrs = append(attrs, slog.Any("execution_diagnostic", diagnostic))
	}
	if stop := workerhost.StopDetails(stopErr); stop != nil {
		attrs = append(attrs, slog.Any("stop_diagnostic", stop))
	}
	logger.LogAttrs(context.Background(), slog.LevelError, "参考执行未完成", attrs...)
}

func sendFrozenReferenceReport(ctx context.Context, input problemreference.WorkerReportInput, send func(context.Context, problemreference.WorkerReportInput) error) error {
	for attempt := 0; attempt < 3 && ctx.Err() == nil; attempt++ {
		err := send(ctx, input)
		if !errors.Is(err, workerclient.ErrReportUnknown) {
			return err
		}
	}
	return workerclient.ErrReportUnknown
}
