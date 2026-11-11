package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func validateConvolutionDifferential(ctx context.Context, args []string, logger *slog.Logger) error {
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv(dualModeEnv) != "" || os.Getenv("OJ_WORKER_M0_SLOT") != "" {
		return errors.New("convolution differential requires the explicitly enabled shared single host slot")
	}
	flags := flag.NewFlagSet("validate-convolution-differential", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("source-root", "", "local root of reviewed convolution source files")
	lockPath := flags.String("source-lock", "", "reviewed fixed LC lock")
	output := flags.String("output", "", "new private report directory")
	start := flags.Bool("start-stopped-slot", false, "explicitly start the pre-existing owned convolution slot")
	if flags.Parse(args) != nil || flags.NArg() != 0 || !*start || !canonicalAbsolute(*root) ||
		!canonicalAbsolute(*lockPath) || !canonicalAbsolute(*output) || !referenceOutputOutside(*root, *output) {
		return errors.New("invalid convolution differential arguments or report directory")
	}
	lockBytes, err := readValidationLock(*lockPath)
	if err != nil {
		return err
	}
	sources, err := problemvalidation.LoadConvolutionSources(*root, lockBytes)
	if err != nil {
		return err
	}
	slot, err := workerhost.ReferenceSlotForTier(experiment.ReferenceConvolution)
	if err != nil {
		return err
	}
	if err := os.Mkdir(*output, 0700); err != nil {
		return errors.New("report directory must be new")
	}
	if err := os.WriteFile(filepath.Join(*output, "INCOMPLETE"), []byte("Convolution differential incomplete; publication blocked.\n"), 0600); err != nil {
		return errors.New("could not mark convolution evidence incomplete")
	}
	lock, err := lockSlotMode(slot.LockName(), false)
	if err != nil {
		return err
	}
	defer lock.Close()
	report, err := runConvolutionLocked(ctx, sources, lock, func(ctx context.Context) (validationHost, error) {
		host, err := workerhost.StartReferenceSlot(ctx, slot)
		if host == nil {
			return nil, err
		}
		return host, err
	})
	if report.Schema != "" {
		if e := persistPrivateValidationReport(*output, report, report.Status != "incomplete"); e != nil {
			return errors.New("convolution report persistence failed; inspect private output")
		}
	}
	if err != nil {
		return errors.New("convolution differential did not pass; inspect private output")
	}
	logger.Info("小规模卷积对拍通过；发布仍阻断", "cases", report.CandidateEqual, "corpus_sha256", report.CorpusSHA256)
	return nil
}

func runConvolutionLocked(ctx context.Context, sources problemvalidation.ConvolutionSources, lock *slotLock,
	start func(context.Context) (validationHost, error)) (problemvalidation.ConvolutionReport, error) {
	return runConvolutionTrialLocked(ctx, sources, lock, start, problemvalidation.RunSmallConvolution)
}

func runConvolutionTrialLocked(ctx context.Context, sources problemvalidation.ConvolutionSources, lock *slotLock,
	start func(context.Context) (validationHost, error),
	run func(context.Context, problemvalidation.ConvolutionSources, func() (execution.Sandbox, error)) (problemvalidation.ConvolutionReport, error)) (problemvalidation.ConvolutionReport, error) {
	report := problemvalidation.NewConvolutionReport(sources)
	if err := ctx.Err(); err != nil {
		report.FailureStage = "context"
		return report, err
	}
	if err := lock.dirty(); err != nil {
		report.FailureStage = "quarantine"
		return report, err
	}
	startup, cancel := context.WithTimeout(ctx, 20*time.Second)
	host, err := start(startup)
	cancel()
	if err != nil || host == nil {
		report.FailureStage = "startup"
	}
	if host == nil {
		return report, errors.New("convolution sandbox preflight failed; slot remains quarantined")
	}
	if err == nil {
		report, err = run(ctx, sources, func() (execution.Sandbox, error) {
			return execution.NewConvolutionComparisonGoJudge(host, host.Verify, host)
		})
	}
	report.ContainerID, report.StartedAt = host.Identity()
	stop, done := context.WithTimeout(context.Background(), 15*time.Second)
	stopErr := host.Stop(stop)
	done()
	report.StopDiagnostic = referenceStopDiagnostic(workerhost.StopDetails(stopErr))
	var cleanErr error
	if stopErr == nil {
		report.StopConfirmed = true
		cleanErr = lock.clean()
	}
	if stopErr == nil && cleanErr == nil && ctx.Err() == nil && report.FailureStage != "startup" {
		switch err {
		case nil:
			report.Status = "passed"
		case problemvalidation.ErrConvolutionMismatch:
			report.Status = "mismatch"
		default:
			report.Status = "failed"
		}
	}
	return report, errors.Join(err, stopErr, cleanErr, ctx.Err())
}
