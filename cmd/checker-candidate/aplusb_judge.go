package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func aplusbJudgeManifest(parent context.Context, args []string, out, diagnostics io.Writer) error {
	f := flag.NewFlagSet("checker-candidate aplusb-judge-manifest", flag.ContinueOnError)
	f.SetOutput(diagnostics)
	bundle := f.String("bundle", "", "absolute path to the private A+B bundle")
	lock := f.String("source-lock", "", "trusted checked-in source lock file")
	plan := f.String("expected-plan", "", "independently trusted formal plan SHA-256")
	candidate := f.String("expected-candidate", "", "independently trusted candidate SHA-256")
	var limits judging.TextLimits
	f.IntVar(&limits.Bytes, "bytes", 0, "explicit checker byte cap")
	f.IntVar(&limits.TokenBytes, "token-bytes", 0, "explicit checker token cap")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("invalid judge manifest arguments")
	}
	if f.NArg() != 0 || !absoluteClean(*bundle) || !absoluteClean(*lock) || !limits.Valid() ||
		!problem.ValidImportRevision(*plan) || !problem.ValidImportRevision(*candidate) {
		return errors.New("explicit limits, paths and trusted digests required")
	}
	raw, err := readPinnedLock(*lock)
	if err != nil {
		return err
	}
	prepared, err := problem.LoadBundle(*bundle, "library-checker:aplusb", raw)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	manifest, err := problemvalidation.PrepareAPlusBJudgeManifest(ctx, prepared, *plan, *candidate, limits)
	if err := errors.Join(err, prepared.Close(), ctx.Err()); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(struct {
		Manifest problemvalidation.APlusBJudgeManifest `json:"manifest"`
		SHA256   string                                `json:"sha256"`
	}{manifest, manifest.Digest()})
}
