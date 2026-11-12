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

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/restorecheck"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func runVerifyRestoredMaterials(args []string) error {
	flags := flag.NewFlagSet("verify-restored-materials", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	budget := flags.Duration("timeout", 10*time.Minute, "whole verification timeout")
	if err := flags.Parse(args); err != nil || flags.NArg() != 3 || *budget <= 0 || *budget > 2*time.Hour {
		return errors.New("usage: migrate verify-restored-materials [-timeout duration] <database> <judge-root> <private-root>")
	}
	dsn := os.Getenv("OJ_RESTORED_DATABASE_URL")
	if dsn == "" {
		return errRestoredDatabaseURL
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *budget)
	defer cancel()
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return errors.New("restored material database unavailable")
	}
	defer pool.Close()
	result, err := restorecheck.VerifyMaterials(ctx, pool, flags.Arg(0), flags.Arg(1), flags.Arg(2))
	if ctx.Err() != nil {
		return errors.New("restored material verification interrupted or timed out")
	}
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return errors.New("restored material verification receipt unavailable")
	}
	return nil
}
