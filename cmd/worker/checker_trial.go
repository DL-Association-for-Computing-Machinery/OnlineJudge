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
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func validateCheckerCandidate(ctx context.Context, args []string, logger *slog.Logger) (err error) {
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || os.Getenv(dualModeEnv) != "" {
		return errors.New("checker trial requires the explicit, single reference slot")
	}
	f := flag.NewFlagSet("validate-checker-candidate", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	bundle := f.String("bundle", "", "private problem bundle")
	sourceID := f.String("source-id", "library-checker:aplusb", "fixed A+B or unionfind source ID")
	lockPath := f.String("source-lock", "", "trusted fixed source lock")
	output := f.String("output", "", "new private report directory")
	plan := f.String("expected-plan", "", "trusted formal plan SHA-256")
	candidate := f.String("expected-candidate", "", "trusted candidate SHA-256")
	caps := judging.TextLimits{}
	f.IntVar(&caps.Bytes, "bytes", 0, "explicit candidate answer and stdout cap")
	f.IntVar(&caps.TokenBytes, "token-bytes", 0, "explicit candidate token cap")
	start := f.Bool("start-stopped-slot", false, "explicitly start the fixed stopped reference slot")
	if f.Parse(args) != nil || f.NArg() != 0 || !*start || (*sourceID != "library-checker:aplusb" && *sourceID != "library-checker:unionfind") || !canonicalAbsolute(*bundle) ||
		!canonicalAbsolute(*lockPath) || !canonicalAbsolute(*output) || !referenceOutputOutside(*bundle, *output) {
		return errors.New("invalid checker trial arguments")
	}
	lockBytes, err := readValidationLock(*lockPath)
	if err != nil {
		return err
	}
	prepared, err := problem.LoadBundle(*bundle, *sourceID, lockBytes)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, prepared.Close()) }()
	preflightCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	trial, err := problemvalidation.PrepareCheckerTrialWithLifetime(preflightCtx, ctx, prepared, *plan, *candidate, caps)
	cancel()
	defer func() { err = errors.Join(err, trial.Close()) }()
	if err := errors.Join(err, ctx.Err()); err != nil {
		return err
	}
	if err := os.Mkdir(*output, 0700); err != nil {
		return errors.New("report directory must be new")
	}
	if err := os.WriteFile(filepath.Join(*output, "INCOMPLETE"), []byte("Checker trial incomplete; no admission.\n"), 0600); err != nil {
		return err
	}
	slot := workerhost.ReferenceSlot{}
	lock, err := lockSlotMode(slot.LockName(), false)
	if err != nil {
		return err
	}
	defer lock.Close()
	report, err := runCheckerTrialLocked(ctx, trial, lock, openCheckerTrialHost)
	if report.Schema == "" {
		return err
	}
	report.Slot = slot.ContainerName()
	if writeErr := persistPrivateValidationReport(*output, report, report.Complete); writeErr != nil {
		return errors.New("checker trial report persistence failed")
	}
	if err != nil || report.Verdict != "AC" {
		return errors.New("checker trial did not pass; inspect the private report")
	}
	logger.Info("候选检查器试跑通过，未准入生产", "candidate_sha256", report.CandidateSHA256, "points", len(report.Points))
	return nil
}

func openCheckerTrialHost(ctx context.Context) (validationHost, error) {
	host, err := workerhost.StartReference(ctx)
	if host == nil {
		return nil, err
	}
	return host, err
}

func runCheckerTrialLocked(ctx context.Context, trial *problemvalidation.CheckerTrial, lock *slotLock,
	open func(context.Context) (validationHost, error)) (report problemvalidation.CheckerTrialReport, err error) {
	if err := lock.dirty(); err != nil {
		return report, err
	}
	startup, cancel := context.WithTimeout(ctx, 20*time.Second)
	host, err := open(startup)
	cancel()
	if host == nil {
		return report, errors.New("checker trial startup not confirmed; keep slot quarantined")
	}
	if err == nil {
		report, err = trial.Run(ctx, execution.NewReferenceGoJudge(host, host.Verify, host))
	}
	stop, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	stopErr := host.Stop(stop)
	cancel()
	report.StopDiagnostic = referenceStopDiagnostic(workerhost.StopDetails(stopErr))
	var cleanErr error
	if stopErr == nil {
		cleanErr = lock.clean()
	}
	err = report.Finalize(errors.Join(err, ctx.Err(), trial.Close()), stopErr, cleanErr)
	return report, err
}
