package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func validatePackageQuality(ctx context.Context, args []string) (err error) {
	f := flag.NewFlagSet("validate-package-quality", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	bundle := f.String("bundle", "", "private frozen package")
	source := f.String("source-id", "", "package source ID")
	lockPath := f.String("source-lock", "", "operator-selected source lock")
	output := f.String("output", "", "new private report directory")
	start := f.Bool("start-stopped-slot", false, "start the fixed stopped Ubuntu slot")
	if f.Parse(args) != nil || f.NArg() != 0 || !*start || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" ||
		os.Getenv(dualModeEnv) != "" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || !canonicalAbsolute(*bundle) ||
		!canonicalAbsolute(*lockPath) || !canonicalAbsolute(*output) || !referenceOutputOutside(*bundle, *output) {
		return errors.New("invalid local package quality arguments")
	}
	if err := workerhost.CheckUbuntuAPlusBHost(ctx); err != nil {
		return err
	}
	raw, err := checkedfile.ReadPublic(*lockPath, 1<<20)
	if err != nil {
		return err
	}
	p, err := problem.LoadBundle(*bundle, *source, raw)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, p.Close()) }()
	slot, err := workerhost.ReferenceSlotForTier(experiment.ReferenceAPlusBUbuntu)
	if err != nil {
		return err
	}
	if err := os.Mkdir(*output, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*output, "INCOMPLETE"), []byte("Quality run incomplete.\n"), 0600); err != nil {
		return err
	}
	lock, err := lockReferenceRuntime(slot, false)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := lock.dirty(); err != nil {
		return err
	}
	startup, cancel := context.WithTimeout(ctx, 20*time.Second)
	host, runErr := workerhost.StartReferenceSlot(startup, slot)
	cancel()
	if host == nil {
		return errors.Join(runErr, execution.ErrStopUnconfirmed)
	}
	var report problemvalidation.PackageQualityReport
	if runErr == nil {
		work, done := context.WithTimeout(ctx, 6*time.Minute)
		report, runErr = problemvalidation.RunPackageQuality(work, p, func() (*execution.CachedGoJudge, error) {
			return execution.NewReferenceGoJudgeForTier(host, host.Verify, host, experiment.ReferenceAPlusBUbuntu)
		})
		done()
	}
	stop, done := context.WithTimeout(context.Background(), 15*time.Second)
	stopErr := host.Stop(stop)
	done()
	var cleanErr error
	if stopErr == nil {
		cleanErr = lock.clean()
	}
	report.StopConfirmed = stopErr == nil && cleanErr == nil
	err = errors.Join(runErr, stopErr, cleanErr, ctx.Err())
	return errors.Join(err, persistPrivateValidationReport(*output, report, err == nil))
}
