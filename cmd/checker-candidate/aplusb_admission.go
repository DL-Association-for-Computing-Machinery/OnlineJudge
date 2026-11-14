package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func reviewAPlusBAdmission(parent context.Context, args []string, out, diagnostics io.Writer) error {
	f := flag.NewFlagSet("checker-candidate review-aplusb-admission", flag.ContinueOnError)
	f.SetOutput(diagnostics)
	var in problemvalidation.APlusBAdmissionInputs
	f.StringVar(&in.ReceiptDirectory, "receipt-directory", "", "absolute directory of the existing report receipt")
	f.StringVar(&in.ReceiptSHA256, "expected-receipt", "", "independent report receipt SHA-256")
	f.StringVar(&in.HandoffSHA256, "expected-handoff", "", "independent handoff SHA-256")
	f.StringVar(&in.TargetObservationPath, "target-observation", "", "absolute path to the private target observation")
	f.StringVar(&in.TargetObservationSHA256, "expected-target-observation", "", "independent target observation SHA-256")
	f.StringVar(&in.CheckerObservationPath, "checker-observation", "", "absolute path to the private checker observation")
	f.StringVar(&in.CheckerObservationSHA256, "expected-checker-observation", "", "independent checker observation SHA-256")
	f.StringVar(&in.WorkerArtifactSHA256, "expected-worker", "", "independent Worker binary SHA-256")
	f.StringVar(&in.Target.Node, "node", "", "independently verified node label")
	f.StringVar(&in.Target.Revision, "revision", "", "independently verified Git revision")
	f.StringVar(&in.Target.Runtime, "runtime", "", "independently verified runtime profile SHA-256")
	f.StringVar(&in.Target.Slot, "slot", "", "independently verified slot")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("invalid A+B admission review flags")
	}
	if f.NArg() != 0 || !absoluteClean(in.ReceiptDirectory) ||
		!absoluteClean(in.TargetObservationPath) || !absoluteClean(in.CheckerObservationPath) {
		return errors.New("three absolute private evidence locations are required")
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	result, err := problemvalidation.ReadAPlusBAdmissionEvidence(ctx, in)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
