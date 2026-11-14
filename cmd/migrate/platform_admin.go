package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func runPlatformAdmin(args []string) error {
	usage := errors.New("usage: migrate platform-admin inspect --subject <id> | set --subject <id> --expected-revision <n> --enabled <true|false> --operator <name> --reason <reason>")
	if len(args) == 0 || (args[0] != "inspect" && args[0] != "set") {
		return usage
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var change identity.PlatformAdminChange
	var enabled string
	flags.StringVar(&change.SubjectID, "subject", "", "")
	if args[0] == "set" {
		flags.Int64Var(&change.ExpectedRevision, "expected-revision", -1, "")
		flags.StringVar(&enabled, "enabled", "", "")
		flags.StringVar(&change.Operator, "operator", "", "")
		flags.StringVar(&change.Reason, "reason", "", "")
	}
	if flags.Parse(args[1:]) != nil || flags.NArg() != 0 || !identity.ValidID(change.SubjectID) {
		return usage
	}
	change.Enabled = enabled == "true"
	if args[0] == "set" && ((enabled != "true" && enabled != "false") || !change.Valid()) {
		return usage
	}
	dsn := os.Getenv("OJ_DATABASE_URL")
	if dsn == "" {
		return errors.New("OJ_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	failed := errors.New("platform administrator operation not confirmed; inspect the current revision")
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return failed
	}
	defer pool.Close()
	if db.CheckSchema(ctx, pool) != nil {
		return failed
	}
	roles, err := identitypg.NewPlatformAdmins(pool)
	if err != nil {
		return failed
	}
	var role identity.PlatformAdminRole
	if args[0] == "inspect" {
		role, err = roles.Inspect(ctx, change.SubjectID)
	} else {
		role, err = roles.Change(ctx, change)
	}
	if errors.Is(err, identity.ErrConflict) {
		return errors.New("platform administrator revision conflict; inspect the current revision")
	}
	if errors.Is(err, identity.ErrNotFound) {
		return errors.New("platform administrator account or revision not found")
	}
	if err != nil || json.NewEncoder(os.Stdout).Encode(role) != nil {
		return failed
	}
	return nil
}
