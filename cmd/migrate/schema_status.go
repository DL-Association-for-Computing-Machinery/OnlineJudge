package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
)

var errSchemaStatusUsage = errors.New("usage: migrate schema-status <expected-database> <migration-schema>")
var errSchemaStatusUnavailable = errors.New("schema status unavailable; check database confirmation, ledger and read permissions")

type schemaStatus struct {
	Format          string `json:"format"`
	ObservedVersion int64  `json:"observed_version"`
	RuntimeVersion  int64  `json:"runtime_version"`
	MatchesRuntime  bool   `json:"matches_runtime"`
}

func runSchemaStatus(args []string, getenv func(string) string, output io.Writer) error {
	if len(args) != 2 || !schemaStatusName(args[0]) || !schemaStatusName(args[1]) {
		return errSchemaStatusUsage
	}
	dsn := getenv("OJ_SCHEMA_STATUS_DATABASE_URL")
	if dsn == "" {
		return errors.New("OJ_SCHEMA_STATUS_DATABASE_URL is required")
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, 5*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return errSchemaStatusUnavailable
	}
	defer func() { _ = conn.Close(ctx) }()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return errSchemaStatusUnavailable
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var matches bool
	if err := tx.QueryRow(ctx, "SELECT current_database()::text = $1", args[0]).Scan(&matches); err != nil || !matches {
		return errSchemaStatusUnavailable
	}
	// Use the explicitly confirmed namespace, independent of the DSN's search_path.
	table := pgx.Identifier{args[1], "goose_db_version"}.Sanitize()
	var version int64
	if err := tx.QueryRow(ctx, "SELECT version_id FROM "+table+" ORDER BY id DESC LIMIT 1").Scan(&version); err != nil || version < 0 {
		return errSchemaStatusUnavailable
	}
	if err := tx.Commit(ctx); err != nil || ctx.Err() != nil {
		return errSchemaStatusUnavailable
	}
	report := schemaStatus{
		Format: "oj.schema-status.v1", ObservedVersion: version, RuntimeVersion: db.RuntimeSchemaVersion,
		MatchesRuntime: version == db.RuntimeSchemaVersion,
	}
	if err := json.NewEncoder(output).Encode(report); err != nil {
		return errSchemaStatusUnavailable
	}
	return nil
}

func schemaStatusName(value string) bool {
	return value != "" && len(value) <= 63 && !strings.ContainsRune(value, '\x00')
}
