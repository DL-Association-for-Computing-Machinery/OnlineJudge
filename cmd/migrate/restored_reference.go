package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

var errRestoredReferenceUsage = errors.New("usage: migrate restored-reference list <database> <closed-epoch> [after-id], status <database> <closed-epoch> <request-id>, or cancel <database> <closed-epoch> <request-id> --confirm-cancel")

func runRestoredReference(args []string) error {
	if len(args) < 3 || args[1] == "" || !identity.ValidRecoveryID(args[2]) {
		return errRestoredReferenceUsage
	}
	switch args[0] {
	case "list":
		if len(args) > 4 || (len(args) == 4 && !identity.ValidID(args[3])) {
			return errRestoredReferenceUsage
		}
	case "status":
		if len(args) != 4 || !identity.ValidID(args[3]) {
			return errRestoredReferenceUsage
		}
	case "cancel":
		if len(args) != 5 || !identity.ValidID(args[3]) || args[4] != "--confirm-cancel" {
			return errRestoredReferenceUsage
		}
	default:
		return errRestoredReferenceUsage
	}
	dsn := os.Getenv("OJ_RESTORED_DATABASE_URL")
	if dsn == "" {
		return errRestoredDatabaseURL
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return errors.New("restored reference database unavailable")
	}
	defer pool.Close()
	if err := db.CheckSchema(ctx, pool); err != nil {
		return errors.New("restored reference database schema check failed")
	}
	var receipt any
	switch args[0] {
	case "list":
		var after string
		if len(args) == 4 {
			after = args[3]
		}
		receipt, err = referencepg.ListRestoredReferences(ctx, pool, args[1], args[2], after)
	case "status":
		receipt, err = referencepg.InspectRestoredReference(ctx, pool, args[1], args[2], args[3])
	case "cancel":
		receipt, err = referencepg.CancelRestoredReference(ctx, pool, args[1], args[2], args[3])
	}
	if err != nil {
		return restoredReferenceError(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(receipt); err != nil {
		return errors.New("restored reference receipt unavailable; inspect status before retrying")
	}
	return nil
}

func restoredReferenceError(err error) error {
	switch {
	case errors.Is(err, identity.ErrConflict), errors.Is(err, identity.ErrRecovering):
		return errors.New("restored reference maintenance requires the confirmed closed recovery epoch")
	case errors.Is(err, problemreference.ErrCommitUnknown):
		return errors.New("restored reference cancellation outcome unknown; inspect status before retrying")
	}
	for _, known := range []error{referencepg.ErrRestoreDatabase, referencepg.ErrRestoreTerminal, problemreference.ErrNotFound, problemreference.ErrPolicy} {
		if errors.Is(err, known) {
			return known
		}
	}
	return errors.New("restored reference maintenance failed; check database access, state and locks")
}
