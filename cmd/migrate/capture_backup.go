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

func runCaptureBackup(args []string) error {
	flags := flag.NewFlagSet("capture-backup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	socket := flags.String("socket", "", "explicit Unix socket directory")
	port := flags.Uint("port", 0, "explicit PostgreSQL port")
	user := flags.String("user", "", "backup database role")
	passfile := flags.String("passfile", "", "optional dedicated libpq password file")
	timeout := flags.Duration("timeout", 10*time.Minute, "whole backup timeout")
	if err := flags.Parse(args); err != nil || flags.NArg() != 4 || *port < 1 || *port > 65535 || *timeout <= 0 || *timeout > 2*time.Hour {
		return errors.New("usage: migrate capture-backup -socket <path> -port <port> -user <role> [-passfile <path>] [-timeout <duration>] <database> <judge-root> <private-root> <new-output-directory>")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	m, err := backup.Capture(ctx, backup.CaptureConfig{
		Socket: *socket, Port: uint16(*port), User: *user, Passfile: *passfile, Database: flags.Arg(0),
		JudgeRoot: flags.Arg(1), PrivateRoot: flags.Arg(2), Output: flags.Arg(3),
	})
	if err != nil {
		if ctx.Err() != nil {
			return errors.New("backup interrupted or timed out; inspect output before use")
		}
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(m); err != nil {
		return errors.New("backup receipt unavailable; inspect the output manifest")
	}
	return nil
}
