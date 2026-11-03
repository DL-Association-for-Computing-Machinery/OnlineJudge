// Command problem-author maintains the narrow problem author role locally.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

var errOutcomeUnknown = errors.New("role change outcome unknown")

type roleStore interface {
	AuthorRole(context.Context, string) (identity.ProblemAuthorRole, error)
	SetAuthorRole(context.Context, identity.ProblemRoleChange) (identity.ProblemAuthorRole, error)
}

type command struct {
	operation string
	change    identity.ProblemRoleChange
}

func parse(args []string) (command, error) {
	if len(args) == 0 || (args[0] != "inspect" && args[0] != "set") {
		return command{}, identity.ErrInvalid
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	f.SetOutput(io.Discard)
	subject := f.String("subject", "", "account subject ID")
	var revision, enabled, operator, reason *string
	if args[0] == "set" {
		revision = f.String("expected-revision", "", "explicit current revision")
		enabled = f.String("enabled", "", "true or false")
		operator = f.String("operator", "", "maintenance operator")
		reason = f.String("reason", "", "reason for change")
	}
	if f.Parse(args[1:]) != nil || f.NArg() != 0 || !identity.ValidID(*subject) {
		return command{}, identity.ErrInvalid
	}
	c := command{operation: args[0], change: identity.ProblemRoleChange{SubjectID: *subject}}
	if c.operation == "set" {
		n, err := strconv.ParseInt(*revision, 10, 64)
		if err != nil || (*enabled != "true" && *enabled != "false") {
			return command{}, identity.ErrInvalid
		}
		c.change.ExpectedRevision, c.change.Enabled = n, *enabled == "true"
		c.change.Operator, c.change.Reason = *operator, *reason
		if !c.change.Valid() {
			return command{}, identity.ErrInvalid
		}
	}
	return c, nil
}

func execute(ctx context.Context, c command, store roleStore, out io.Writer) error {
	var role identity.ProblemAuthorRole
	var err error
	if c.operation == "inspect" {
		role, err = store.AuthorRole(ctx, c.change.SubjectID)
	} else {
		role, err = store.SetAuthorRole(ctx, c.change)
		if err != nil && (errors.Is(err, identity.ErrStore) || (!errors.Is(err, identity.ErrInvalid) && !errors.Is(err, identity.ErrNotFound) && !errors.Is(err, identity.ErrForbidden) && !errors.Is(err, identity.ErrConflict))) {
			return errOutcomeUnknown
		}
	}
	if err != nil {
		return err
	}
	if err := json.NewEncoder(out).Encode(role); err != nil {
		if c.operation == "set" {
			return errOutcomeUnknown
		}
		return err
	}
	return nil
}

func run(parent context.Context, args []string, out io.Writer) error {
	c, err := parse(args)
	if err != nil {
		return err
	}
	dsn := os.Getenv("OJ_DATABASE_URL")
	if dsn == "" {
		return identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	pool, err := submissionpg.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.CheckSchema(ctx, pool); err != nil {
		return err
	}
	store, err := identitypg.NewProblemAccess(pool)
	if err != nil {
		return err
	}
	return execute(ctx, c, store, out)
}

func errorMessage(err error) string {
	switch {
	case errors.Is(err, errOutcomeUnknown):
		return "role change outcome unknown; inspect the subject and verify the expected revision before any further change"
	case errors.Is(err, identity.ErrConflict):
		return "role change conflict; inspect the subject and verify the expected revision before any further change"
	case errors.Is(err, identity.ErrNotFound):
		return "subject or role not found; inspect the subject before any change"
	default:
		return "role operation failed; check explicit arguments, database access and schema compatibility"
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, errorMessage(err))
		os.Exit(1)
	}
}
