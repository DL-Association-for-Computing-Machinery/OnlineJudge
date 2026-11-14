package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

var errTrainingAcceptanceUsage = errors.New("usage: migrate training-acceptance <expected-database> <from-utc> <until-utc> (RFC3339 UTC, at most one hour)")

func runTrainingAcceptance(args []string, getenv func(string) string, output io.Writer) error {
	if len(args) != 3 || args[0] == "" {
		return errTrainingAcceptanceUsage
	}
	from, err := time.Parse(time.RFC3339Nano, args[1])
	until, endErr := time.Parse(time.RFC3339Nano, args[2])
	if err != nil || endErr != nil || from.UTC().Format(time.RFC3339Nano) != args[1] ||
		until.UTC().Format(time.RFC3339Nano) != args[2] || !from.Before(until) || until.Sub(from) > time.Hour {
		return errTrainingAcceptanceUsage
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
	if db.CheckSchema(ctx, pool) != nil {
		return postgres.ErrJudgeSnapshotUnavailable
	}
	observation, err := postgres.ObserveTrainingAcceptance(ctx, pool, args[0], from, until)
	if err != nil {
		return err
	}
	if json.NewEncoder(output).Encode(observation) != nil {
		return postgres.ErrJudgeSnapshotUnavailable
	}
	return nil
}
