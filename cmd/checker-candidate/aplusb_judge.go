package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
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
	output := f.String("export", "", "new private directory for transport materials")
	expected := f.String("expected-manifest", "", "independently reviewed judge manifest SHA-256")
	ubuntu := f.Bool("ubuntu-aplusb", false, "select the fixed Ubuntu guest runtime")
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
	tier := experiment.ReferenceSmall
	if *ubuntu {
		tier = experiment.ReferenceAPlusBUbuntu
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
	if *output != "" || *expected != "" {
		defer func() { _ = prepared.Close() }()
		if !absoluteClean(*output) || !problem.ValidImportRevision(*expected) {
			return errors.New("export requires an absolute new directory and trusted judge manifest")
		}
		var root *os.Root
		manifest, err := problemvalidation.ExportAPlusBMaterialsForTier(ctx, prepared, *plan, *candidate, *expected, limits, tier, func(hash string, data []byte) error {
			if root == nil {
				if err := os.Mkdir(*output, 0700); err != nil {
					return err
				}
				root, err = os.OpenRoot(*output)
				if err != nil {
					return err
				}
			}
			return writeExportFile(root, hash, data)
		})
		if root != nil {
			defer func() { _ = root.Close() }()
		}
		if err != nil {
			return err
		}
		if root == nil {
			return errors.New("no materials exported")
		}
		if err := writeExportFile(root, "manifest.json", manifest.Canonical()); err != nil {
			return err
		}
		directory, err := root.Open(".")
		if err != nil {
			return err
		}
		if err := errors.Join(directory.Sync(), directory.Close()); err != nil {
			return err
		}
		_, err = io.WriteString(out, filepath.Join(*output, "manifest.json")+"\n")
		return err
	}
	manifest, err := problemvalidation.PrepareAPlusBJudgeManifestForTier(ctx, prepared, *plan, *candidate, limits, tier)
	if err := errors.Join(err, prepared.Close(), ctx.Err()); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(struct {
		Manifest problemvalidation.APlusBJudgeManifest `json:"manifest"`
		SHA256   string                                `json:"sha256"`
	}{manifest, manifest.Digest()})
}

func writeExportFile(root *os.Root, name string, data []byte) error {
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return errors.Join(err, f.Sync(), f.Close())
}
