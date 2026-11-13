package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func aplusbTargetHandoff(parent context.Context, args []string, out, diagnostics io.Writer) error {
	f := flag.NewFlagSet("checker-candidate aplusb-target-handoff", flag.ContinueOnError)
	f.SetOutput(diagnostics)
	mode := f.String("mode", "", "pack, verify, or reports")
	source := f.String("source", "", "existing private A+B material export (pack only)")
	directory := f.String("directory", "", "absolute handoff directory")
	pin := f.String("expected-handoff", "", "independently transmitted handoff SHA-256")
	var target problemvalidation.APlusBHandoffTarget
	f.StringVar(&target.Node, "node", "", "intended node label")
	f.StringVar(&target.Revision, "revision", "", "expected candidate Git revision")
	f.StringVar(&target.Runtime, "runtime", "", "expected target runtime report profile SHA-256")
	f.StringVar(&target.Slot, "slot", "", "expected slot")
	var archives, digests [3]string
	for i, role := range []string{"ac", "wa", "ole"} {
		f.StringVar(&archives[i], role, "", "completed "+role+" report directory (reports only)")
		f.StringVar(&digests[i], role+"-sha256", "", "operator-provided "+role+" report digest")
	}
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("invalid A+B handoff flags")
	}
	if f.NArg() != 0 || !absoluteClean(*directory) {
		return errors.New("an absolute handoff directory is required")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	var empty [3]string
	if *mode != "reports" && (archives != empty || digests != empty) {
		return errors.New("report flags require reports mode")
	}
	switch *mode {
	case "pack":
		if !absoluteClean(*source) || *pin != "" {
			return errors.New("pack requires an absolute private source and a new directory")
		}
		digest, err := problemvalidation.PackAPlusBHandoff(ctx, *source, *directory, target)
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, digest+"\n")
		return err
	case "verify", "reports":
		if *source != "" || !problem.ValidImportRevision(*pin) {
			return errors.New("an independently trusted handoff digest is required")
		}
		if *mode == "verify" {
			h, err := problemvalidation.VerifyAPlusBHandoff(ctx, *directory, *pin, target)
			if err != nil {
				return err
			}
			return json.NewEncoder(out).Encode(h)
		}
		var reports [3][]byte
		var check [3]func() error
		for i, path := range archives {
			if !absoluteClean(path) || !problem.ValidImportRevision(digests[i]) {
				return errors.New("three absolute report paths and independent digests are required")
			}
			root, err := openTrialArchive(path)
			if err != nil {
				return err
			}
			defer func() { _ = root.Close() }()
			reports[i], err = readCompletedTrial(root)
			if err != nil {
				return err
			}
			check[i] = func() error { return checkTrialMarker(root) }
		}
		result, err := problemvalidation.VerifyAPlusBHandoffReports(ctx, *directory, *pin, target, reports, digests)
		if err != nil {
			return err
		}
		for _, confirm := range check {
			if err := confirm(); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(result)
	default:
		return errors.New("mode must be pack, verify, or reports")
	}
}
