package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

var errJudgeSnapshotUsage = errors.New("usage: migrate judge-snapshot <expected-database> <acceptance-window-minutes (1..60)>")

func runJudgeSnapshot(args []string, getenv func(string) string, output io.Writer) error {
	if len(args) != 2 || args[0] == "" {
		return errJudgeSnapshotUsage
	}
	minutes, err := strconv.Atoi(args[1])
	if err != nil || minutes < 1 || minutes > 60 || strconv.Itoa(minutes) != args[1] {
		return errJudgeSnapshotUsage
	}
	dsn := getenv("OJ_JUDGE_SNAPSHOT_DATABASE_URL")
	if dsn == "" {
		return errors.New("OJ_JUDGE_SNAPSHOT_DATABASE_URL is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return postgres.ErrJudgeSnapshotUnavailable
	}
	defer pool.Close()
	if err := db.CheckSchema(ctx, pool); err != nil {
		return postgres.ErrJudgeSnapshotUnavailable
	}
	snapshot, err := postgres.ObserveJudgeOperations(ctx, pool, args[0], minutes)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(output).Encode(snapshot); err != nil {
		return postgres.ErrJudgeSnapshotUnavailable
	}
	return nil
}
