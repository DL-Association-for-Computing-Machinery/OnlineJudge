package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func runMailSendRate(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: migrate mail-send-rate <spacing-ms>")
	}
	spacing, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil || spacing < 100 || spacing > 60000 {
		return errors.New("mail send spacing must be 100 to 60000 milliseconds")
	}
	dsn := os.Getenv("OJ_DATABASE_URL")
	if dsn == "" {
		return errors.New("OJ_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return errors.New("mail send rate database unavailable")
	}
	defer pool.Close()
	if err := db.CheckSchema(ctx, pool); err != nil {
		return err
	}
	if err := mailqueue.ConfigureSendRate(ctx, pool, int32(spacing)); err != nil {
		return errors.New("mail send rate update not confirmed")
	}
	fmt.Println("mail send rate configured")
	return nil
}

func runMailQueueUp(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: migrate mail-queue-up")
	}
	dsn := os.Getenv("OJ_DATABASE_URL")
	if dsn == "" {
		return errors.New("OJ_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := mailqueue.MigrateUp(ctx, pool); err != nil {
		return err
	}
	fmt.Println("mail queue migration completed")
	return nil
}
