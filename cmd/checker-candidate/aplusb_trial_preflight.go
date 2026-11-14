package main

import (
	"context"
	"errors"
	"flag"
	"io"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func aplusbTrialPreflight(ctx context.Context, args []string, out, diagnostics io.Writer) error {
	f := flag.NewFlagSet("checker-candidate aplusb-trial-preflight", flag.ContinueOnError)
	f.SetOutput(diagnostics)
	id := f.String("container-id", "", "original full ID of the dedicated stopped candidate slot")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return errors.New("invalid candidate preflight arguments")
	}
	if f.NArg() != 0 || *id == "" {
		return errors.New("candidate preflight requires the original full container ID")
	}
	if err := workerhost.CheckAPlusBTrialAMD64Slot(ctx, *id); err != nil {
		return err
	}
	_, err := io.WriteString(out, "stopped candidate slot configuration matched; execution and admission not checked\n")
	return err
}
