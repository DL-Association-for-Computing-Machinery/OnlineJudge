package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func localSPJRuntimePin(out io.Writer) error {
	_, err := fmt.Fprintln(out, experiment.LocalSPJRuntimeSHA256())
	return err
}

func exportLocalText(parent context.Context, args []string, out, diagnostics io.Writer) error {
	f := flag.NewFlagSet("export-local-text", flag.ContinueOnError)
	f.SetOutput(diagnostics)
	bundle := f.String("bundle", "", "private frozen bundle")
	lock := f.String("source-lock", "", "trusted source lock")
	report := f.String("report-dir", "", "verified complete checker trial directory")
	source := f.String("source-id", "", "source ID from the pinned bundle")
	reportSHA := f.String("expected-report", "", "trusted report SHA-256")
	reviewProfile := f.String("review-profile", "", "formal reference profile SHA-256 of the publication review")
	packageReview := f.Bool("package-review", false, "use the fixed local package review profile")
	qualityDir := f.String("quality-dir", "", "completed package quality directory")
	qualitySHA := f.String("expected-quality", "", "trusted quality report SHA-256")
	rendering := f.String("rendering-file", "", "completed package rendering receipt")
	renderingSHA := f.String("expected-rendering", "", "trusted rendering receipt SHA-256")
	spjReport := f.String("spj-report", "", "completed local SPJ evidence file")
	spjSHA := f.String("expected-spj-report", "", "trusted local SPJ evidence SHA-256")
	dest := f.String("export", "", "new private material directory")
	if f.Parse(args) != nil || f.NArg() != 0 || !filepath.IsAbs(*bundle) || !filepath.IsAbs(*lock) ||
		!filepath.IsAbs(*report) || !filepath.IsAbs(*dest) || filepath.Clean(*dest) != *dest || !exportOutsideInputs(*dest, *bundle, *report) {
		return errors.New("absolute input paths and a new export directory outside the inputs required")
	}
	raw, err := checkedfile.ReadPublic(*lock, 1<<20)
	if err != nil {
		return err
	}
	in := problemvalidation.LocalTextAdmissionInput{
		SourceID: *source, ReportDirectory: *report, ReportSHA256: *reportSHA, ReviewProfileSHA256: *reviewProfile, PackageReview: *packageReview,
		QualityDirectory: *qualityDir, QualitySHA256: *qualitySHA, RenderingFile: *rendering, RenderingSHA256: *renderingSHA,
		SPJReport: *spjReport, SPJReportSHA256: *spjSHA,
	}
	if in.PackageReview && *reviewProfile == "" {
		in.ReviewProfileSHA256 = problemvalidation.LocalTextProfile().Digest()
	}
	prepared, err := problem.LoadBundle(*bundle, in.SourceID, raw)
	if err != nil {
		return err
	}
	defer func() { _ = prepared.Close() }()
	if err := os.Mkdir(*dest, 0700); err != nil {
		return err
	}
	root, err := os.OpenRoot(*dest)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	ctx, cancel := context.WithTimeout(parent, time.Minute)
	defer cancel()
	seen := make(map[string]bool)
	admission, err := problemvalidation.VerifyLocalText(ctx, prepared, in, func(hash string, data []byte) error {
		if seen[hash] {
			return nil
		}
		seen[hash] = true
		return writeExportFile(root, hash, data)
	})
	if err != nil {
		return err
	}
	if err := writeExportFile(root, "manifest.json", admission.Manifest()); err != nil {
		return err
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	if err := errors.Join(dir.Sync(), dir.Close()); err != nil {
		return err
	}
	_, _, pin := admission.Binding()
	in.ManifestSHA256 = pin
	return json.NewEncoder(out).Encode(in)
}

func exportOutsideInputs(dest string, paths ...string) bool {
	parent, err := filepath.EvalSymlinks(filepath.Dir(dest))
	if err != nil {
		return false
	}
	resolved := filepath.Join(parent, filepath.Base(dest))
	for _, p := range paths {
		p, err := filepath.EvalSymlinks(p)
		if err != nil || p == resolved {
			return false
		}
		for dir := filepath.Dir(resolved); ; dir = filepath.Dir(dir) {
			if dir == p {
				return false
			}
			if dir == filepath.Dir(dir) {
				break
			}
		}
	}
	return true
}
