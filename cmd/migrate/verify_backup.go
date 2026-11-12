package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/backup"
)

func runVerifyBackup(args []string) error {
	flags := flag.NewFlagSet("verify-backup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	digest := flags.String("manifest-sha256", "", "trusted SHA-256 from the source completion manifest")
	timeout := flags.Duration("timeout", 10*time.Minute, "whole verification timeout")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 || *digest == "" || *timeout <= 0 || *timeout > 2*time.Hour {
		return errors.New("usage: migrate verify-backup -manifest-sha256 <trusted-digest> [-timeout <duration>] <package-directory>")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	receipt, err := backup.VerifyTransfer(ctx, flags.Arg(0), *digest)
	if ctx.Err() != nil {
		return errors.New("backup verification interrupted or timed out")
	}
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(receipt); err != nil {
		return errors.New("backup verification receipt unavailable")
	}
	return nil
}
