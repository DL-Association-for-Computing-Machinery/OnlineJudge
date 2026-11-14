// Command problem-review manages explicit reviewer and publisher grants and assignments.
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
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

type command struct {
	op         string
	id         string
	role       identity.ProblemRoleChange
	assignment problemreference.ReviewAssignmentChange
}

func parse(args []string) (command, error) {
	var c command
	if len(args) == 0 {
		return c, identity.ErrInvalid
	}
	c.op = args[0]
	f := flag.NewFlagSet(c.op, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	switch c.op {
	case "reviewer-inspect", "reviewer-set", "publisher-inspect", "publisher-set":
		f.StringVar(&c.id, "subject", "", "account subject ID")
	case "assignment-inspect", "assign", "withdraw":
		f.StringVar(&c.id, "review", "", "review submission ID")
	default:
		return command{}, identity.ErrInvalid
	}
	var rev *int64
	var operator, reason *string
	if c.op == "reviewer-set" || c.op == "publisher-set" || c.op == "assign" || c.op == "withdraw" {
		rev = f.Int64("expected-revision", -1, "current revision; zero for initial grant")
		operator = f.String("operator", "", "maintenance attribution")
		reason = f.String("reason", "", "reason for change")
	}
	var enabled *string
	if c.op == "reviewer-set" || c.op == "publisher-set" {
		enabled = f.String("enabled", "", "true or false")
	}
	if c.op == "assign" {
		f.StringVar(&c.assignment.ReviewerSubjectID, "reviewer", "", "explicit non-author reviewer subject")
		f.Int64Var(&c.assignment.ExpectedReviewerRevision, "reviewer-revision", 0, "current grant revision")
	}
	if f.Parse(args[1:]) != nil || f.NArg() != 0 || !identity.ValidID(c.id) {
		return command{}, identity.ErrInvalid
	}
	switch c.op {
	case "reviewer-set", "publisher-set":
		c.role = identity.ProblemRoleChange{SubjectID: c.id, ExpectedRevision: *rev,
			Enabled: *enabled == "true", Operator: *operator, Reason: *reason}
		if (*enabled != "true" && *enabled != "false") || !c.role.Valid() {
			return command{}, identity.ErrInvalid
		}
	case "assign", "withdraw":
		c.assignment.ReviewID, c.assignment.ExpectedRevision = c.id, *rev
		c.assignment.Operator, c.assignment.Reason = *operator, *reason
		if (c.op == "assign" && c.assignment.ReviewerSubjectID == "") || !c.assignment.Valid() {
			return command{}, identity.ErrInvalid
		}
	}
	return c, nil
}

func run(parent context.Context, args []string, out io.Writer) error {
	c, err := parse(args)
	if err != nil || os.Getenv("OJ_DATABASE_URL") == "" {
		return identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	pool, err := submissionpg.Open(ctx, os.Getenv("OJ_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.CheckSchema(ctx, pool); err != nil {
		return err
	}
	access, err := identitypg.NewProblemAccess(pool)
	if err != nil {
		return err
	}
	reviews, err := referencepg.NewReviewStore(pool, access)
	if err != nil {
		return err
	}
	var result any
	switch c.op {
	case "reviewer-inspect":
		result, err = access.ReviewerRole(ctx, c.id)
	case "reviewer-set":
		result, err = access.SetReviewerRole(ctx, c.role)
	case "publisher-inspect":
		result, err = access.PublisherRole(ctx, c.id)
	case "publisher-set":
		result, err = access.SetPublisherRole(ctx, c.role)
	case "assignment-inspect":
		result, err = reviews.Assignment(ctx, c.id)
	case "assign", "withdraw":
		result, err = reviews.SetAssignment(ctx, c.assignment)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalid), errors.Is(err, problemreference.ErrInvalid):
			_, _ = fmt.Fprintln(os.Stderr, "invalid review maintenance arguments")
		case errors.Is(err, identity.ErrReviewerRequired):
			_, _ = fmt.Fprintln(os.Stderr, "explicit active non-author reviewer required")
		case errors.Is(err, identity.ErrPublisherRequired):
			_, _ = fmt.Fprintln(os.Stderr, "active account required for a publisher grant")
		default:
			_, _ = fmt.Fprintln(os.Stderr, "review operation not confirmed; inspect the stored revision before any further change")
		}
		os.Exit(1)
	}
}
