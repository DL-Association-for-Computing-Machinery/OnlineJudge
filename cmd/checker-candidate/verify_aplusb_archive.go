package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func verifyAPlusBArchive(parent context.Context, args []string, out, diagnostics io.Writer) error {
	f := flag.NewFlagSet("checker-candidate verify-aplusb-archive", flag.ContinueOnError)
	f.SetOutput(diagnostics)
	bundle := f.String("bundle", "", "private A+B bundle")
	lock := f.String("source-lock", "", "trusted source lock")
	ac := f.String("ac", "", "completed AC report directory")
	wa := f.String("wa", "", "completed WA report directory")
	ole := f.String("ole", "", "completed OLE report directory")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("invalid A+B archive verification flags")
	}
	if f.NArg() != 0 || !absoluteClean(*bundle) || !absoluteClean(*lock) ||
		!absoluteClean(*ac) || !absoluteClean(*wa) || !absoluteClean(*ole) {
		return errors.New("absolute bundle, source-lock and three completed archive directories required")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	var reports [3][]byte
	var roots [3]*os.Root
	for i, path := range []string{*ac, *wa, *ole} {
		root, err := openTrialArchive(path)
		if err != nil {
			return err
		}
		roots[i] = root
		defer func() { _ = root.Close() }()
		reports[i], err = readCompletedTrial(root)
		if err != nil {
			return err
		}
	}
	sourceLock, err := readPinnedLock(*lock)
	if err != nil {
		return err
	}
	prepared, err := problem.LoadBundle(*bundle, "library-checker:aplusb", sourceLock)
	if err != nil {
		return err
	}
	trial, err := problemvalidation.PrepareCheckerTrial(ctx, prepared, problemvalidation.ReviewedAPlusBPlan,
		problemvalidation.ReviewedAPlusBCandidate, problemvalidation.ReviewedAPlusBLimits())
	if err != nil {
		return errors.Join(err, prepared.Close())
	}
	summary, err := trial.VerifyReviewedAPlusBTrials(reports[0], reports[1], reports[2])
	if err := errors.Join(err, trial.Close(), prepared.Close(), ctx.Err()); err != nil {
		return err
	}
	for _, root := range roots {
		if err := checkTrialMarker(root); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(summary)
}
