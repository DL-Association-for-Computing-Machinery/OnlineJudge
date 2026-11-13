package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func runMailPayloadSweep(args []string) error {
	usage := errors.New("usage: migrate mail-payload-sweep <1-512> [cursor]")
	if len(args) < 1 || len(args) > 2 {
		return usage
	}
	limit, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil || limit < 1 || limit > 512 {
		return usage
	}
	var cursor string
	if len(args) == 2 {
		cursor = args[1]
		if !identity.ValidID(cursor) {
			return usage
		}
	}
	dsn := os.Getenv("OJ_DATABASE_URL")
	if dsn == "" {
		return errors.New("OJ_DATABASE_URL is required")
	}
	failure := errors.New("mail payload sweep failed; check database access and schema compatibility")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return failure
	}
	defer pool.Close()
	if db.CheckSchema(ctx, pool) != nil {
		return failure
	}
	batch, err := identitypg.SweepExpiredEmailPayloads(ctx, pool, cursor, int32(limit))
	if err != nil {
		return failure
	}
	result := struct {
		NextCursor string `json:"next_cursor"`
		Removed    int64  `json:"removed"`
	}{NextCursor: batch.NextCursor, Removed: batch.Removed}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		return errors.New("mail payload sweep result could not be written")
	}
	return nil
}
