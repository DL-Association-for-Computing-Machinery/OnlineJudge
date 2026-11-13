package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func verifyTrial(parent context.Context, args []string, out, diagnostics io.Writer) error {
	f := flag.NewFlagSet("checker-candidate verify-trial", flag.ContinueOnError)
	f.SetOutput(diagnostics)
	bundle := f.String("bundle", "", "absolute path to the private problem bundle")
	sourceID := f.String("source-id", "library-checker:aplusb", "fixed A+B, unionfind or Z algorithm source ID")
	lock := f.String("source-lock", "", "trusted checked-in source lock file")
	directory := f.String("report-dir", "", "absolute path to the completed private report directory")
	plan := f.String("expected-plan", "", "independently trusted formal plan SHA-256")
	candidate := f.String("expected-candidate", "", "independently trusted candidate SHA-256")
	reportSHA := f.String("expected-report", "", "independently trusted report.json byte SHA-256")
	caps := judging.TextLimits{}
	f.IntVar(&caps.Bytes, "bytes", 0, "explicit candidate answer and stdout cap")
	f.IntVar(&caps.TokenBytes, "token-bytes", 0, "explicit candidate token cap")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("invalid checker trial verification flags")
	}
	if f.NArg() != 0 || !absoluteClean(*bundle) || !absoluteClean(*lock) || !absoluteClean(*directory) ||
		!problem.ValidImportRevision(*plan) || !problem.ValidImportRevision(*candidate) || !problem.ValidImportRevision(*reportSHA) || !caps.Valid() {
		f.Usage()
		return errors.New("explicit limits, paths and three trusted digests required")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	archive, err := openTrialArchive(*directory)
	if err != nil {
		return err
	}
	defer func() { _ = archive.Close() }()
	raw, err := readCompletedTrial(archive)
	if err != nil {
		return err
	}
	sourceLock, err := readPinnedLock(*lock)
	if err != nil {
		return err
	}
	prepared, err := problem.LoadBundle(*bundle, *sourceID, sourceLock)
	if err != nil {
		return err
	}
	trial, err := problemvalidation.PrepareCheckerTrial(ctx, prepared, *plan, *candidate, caps)
	if err != nil {
		return errors.Join(err, prepared.Close())
	}
	slot, err := workerhost.ReferenceSlotForTier(trial.Tier())
	if err != nil {
		return errors.Join(err, trial.Close(), prepared.Close())
	}
	verified, err := trial.VerifyPassingReport(raw, *reportSHA, slot.ContainerName())
	if err := errors.Join(err, trial.Close(), prepared.Close(), ctx.Err()); err != nil {
		return err
	}
	if err := errors.Join(checkTrialMarker(archive), ctx.Err()); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(verified)
}

func absoluteClean(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}

func openTrialArchive(path string) (*os.Root, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return nil, errors.New("completed trial directory unavailable")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, errors.New("completed trial directory unavailable")
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = root.Close()
		return nil, errors.New("completed trial directory changed")
	}
	return root, nil
}

func checkTrialMarker(root *os.Root) error {
	if _, err := root.Lstat("INCOMPLETE"); !errors.Is(err, os.ErrNotExist) {
		return errors.New("checker trial completion marker still present or unavailable")
	}
	return nil
}

func readCompletedTrial(root *os.Root) ([]byte, error) {
	if err := checkTrialMarker(root); err != nil {
		return nil, err
	}
	const name = "report.json"
	const max = problemvalidation.MaxCheckerTrialReportBytes
	expected, err := root.Lstat(name)
	if err != nil || !expected.Mode().IsRegular() || expected.Size() < 1 || expected.Size() > max {
		return nil, errors.New("unsupported checker trial report")
	}
	// Root keeps path resolution inside the archived directory. It can follow
	// internal links, so also compare the file identity with nonfollowing stats.
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("checker trial report unavailable")
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(expected, info) || info.Size() != expected.Size() {
		return nil, errors.New("unsupported checker trial report")
	}
	raw, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || len(raw) > max || int64(len(raw)) != info.Size() {
		return nil, errors.New("checker trial report could not be read in full")
	}
	current, err := root.Lstat(name)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) || current.Size() != info.Size() {
		return nil, errors.New("checker trial report changed")
	}
	return raw, nil
}
