package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailbudget"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func runMailBudget(action string, args []string) error {
	usage := errors.New("usage: migrate mail-budget-init <budget-database> or mail-budget-provision <budget-database> <ceiling> --fresh-key")
	provision := action == "mail-budget-provision"
	var ceiling int64
	if provision {
		if len(args) != 3 || args[2] != "--fresh-key" {
			return usage
		}
		var err error
		ceiling, err = strconv.ParseInt(args[1], 10, 64)
		if err != nil || ceiling < 1 || ceiling > mailbudget.MaxReservations {
			return usage
		}
	} else if action != "mail-budget-init" || len(args) != 1 {
		return usage
	}
	if args[0] == "" {
		return usage
	}
	dsn := os.Getenv("OJ_MAIL_BUDGET_DATABASE_URL")
	if dsn == "" {
		return errors.New("OJ_MAIL_BUDGET_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	failed := errors.New("mail budget maintenance not confirmed; inspect the independent ledger")
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return failed
	}
	defer pool.Close()
	var database string
	var application bool
	if err := pool.QueryRow(ctx, `SELECT pg_catalog.current_database(), EXISTS (
		SELECT 1 FROM pg_catalog.pg_class WHERE relname IN ('identity_credentials', 'goose_db_version')
	)`).Scan(&database, &application); err != nil {
		return failed
	}
	if database != args[0] || application {
		return errors.New("mail budget destination refused: confirm a dedicated database")
	}
	if provision {
		protector, err := emailtoken.LoadProtector(os.Getenv("OJ_EMAIL_PAYLOAD_KEYS_FILE"))
		if err != nil {
			return errors.New("mail payload key file unavailable")
		}
		if err := mailbudget.Provision(ctx, pool, protector, ceiling); err != nil {
			return failed
		}
	} else if err := mailbudget.Initialize(ctx, pool); err != nil {
		return failed
	}
	fmt.Println("mail budget maintenance completed")
	return nil
}
