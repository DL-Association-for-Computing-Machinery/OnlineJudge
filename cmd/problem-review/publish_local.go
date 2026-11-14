package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/jackc/pgx/v5/pgxpool"
)

func publishAPlusBLocal(parent context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("publish-aplusb-local", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	review := f.String("review", "", "review ID")
	reviewer := f.String("reviewer", "", "assigned account ID")
	publisher := f.String("publisher", "", "separate publisher account ID")
	publicID := f.String("problem", "", "reserved public problem ID")
	key := f.String("request-key", "", "original publication request key")
	material := f.String("material-root", "", "private material directory")
	admissionFile := f.String("admission-file", "", "operator-pinned local acceptance config")
	socket := f.String("database-socket", "", "local PostgreSQL Unix socket directory")
	username := f.String("database-user", "", "local maintenance database identity")
	assignmentRev := f.Int64("assignment-revision", 0, "expected assignment revision")
	publisherRev := f.Int64("publisher-revision", 0, "expected publisher grant revision")
	if f.Parse(args) != nil || f.NArg() != 0 || !identity.ValidID(*review) || !identity.ValidID(*reviewer) ||
		!identity.ValidID(*publisher) || *publisher == *reviewer || !identity.ValidID(*publicID) ||
		!problem.ValidKey(*key) || *assignmentRev < 1 || *publisherRev < 1 ||
		!filepath.IsAbs(*socket) || filepath.Clean(*socket) != *socket || *username == "" ||
		os.Getenv("OJ_DATABASE_URL") != "" {
		return identity.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 40*time.Second)
	defer cancel()
	raw, err := checkedfile.ReadSecret(*admissionFile, 16<<10)
	if err != nil {
		return problemreference.ErrInvalid
	}
	var in problemvalidation.APlusBLocalAdmissionInput
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(new(any)) != io.EOF {
		return problemreference.ErrInvalid
	}
	admission, err := problemvalidation.ReadAPlusBLocalAdmission(ctx, in)
	if err != nil {
		return err
	}
	// Select only the named local socket; do not inherit a remote DSN or
	// pgx failover hosts from the environment.
	c, err := pgxpool.ParseConfig("host=/tmp port=5432 dbname=postgres user=postgres sslmode=disable")
	if err != nil {
		return err
	}
	c.ConnConfig.Host, c.ConnConfig.User = *socket, *username
	c.ConnConfig.Fallbacks = nil
	c.ConnConfig.Password = ""
	pool, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.CheckSchema(ctx, pool); err != nil {
		return fmt.Errorf("local schema: %w", err)
	}
	var authGeneration int64
	var recoveryID string
	if err := pool.QueryRow(ctx, `SELECT a.auth_generation,r.generation FROM identity_credentials a
		CROSS JOIN identity_session_recovery r WHERE a.id=$1 AND a.enabled AND r.singleton AND r.enabled`, *publisher).Scan(&authGeneration, &recoveryID); err != nil {
		return err
	}
	actor := identity.ProblemActor{SubjectID: *publisher, SessionVersion: identity.SessionVersion{
		AuthGeneration: authGeneration, RecoveryID: recoveryID}}
	access, err := identitypg.NewProblemAccess(pool)
	if err != nil {
		return err
	}
	reviews, err := referencepg.NewReviewStore(pool, access)
	if err != nil {
		return err
	}
	reviews, err = reviews.WithLocalExecution(admission)
	if err != nil {
		return err
	}
	prepared, plan, err := reviews.PrepareLocalPublication(ctx, problempg.ReadBoundReviewDraft, *review, *publicID)
	if err != nil {
		return err
	}
	objects, err := materialstore.OpenLocal(materialstore.Config{Root: *material, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	if err != nil {
		return err
	}
	defer func() { _ = objects.Close() }()
	sealed, err := prepared.Seal(ctx, objects)
	if err != nil {
		return err
	}
	_, version, execution := admission.Binding()
	published, err := reviews.PublishProblem(ctx, access, actor, objects, plan, referencepg.PublicationIntent{
		Materials: sealed, ReviewID: *review, ReviewerID: *reviewer, ExecutionVersion: version, ExecutionSHA256: execution,
		RequestKey: *key, PublisherRevision: *publisherRev, AssignmentRevision: *assignmentRev,
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(published)
}
