package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

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
