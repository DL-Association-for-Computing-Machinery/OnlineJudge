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
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func loadLocalText(ctx context.Context, in problemvalidation.LocalTextAdmissionInput, bundle, lock string) (problemvalidation.LocalExecutionAdmission, error) {
	var zero problemvalidation.LocalExecutionAdmission
	raw, err := checkedfile.ReadPublic(lock, 1<<20)
	if err != nil {
		return zero, err
	}
	prepared, err := problem.LoadBundle(bundle, in.SourceID, raw)
	if err != nil {
		return zero, err
	}
	defer func() { _ = prepared.Close() }()
	return problemvalidation.ReadLocalTextAdmission(ctx, in, prepared)
}

func registerLocalText(parent context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("register-text-local", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	bundle := f.String("bundle", "", "private local text bundle")
	lock := f.String("source-lock", "", "trusted source lock")
	file := f.String("admission-file", "", "private pinned local admission configuration")
	socket := f.String("database-socket", "", "local PostgreSQL Unix socket directory")
	user := f.String("database-user", "", "local maintenance database identity")
	if f.Parse(args) != nil || f.NArg() != 0 || !filepath.IsAbs(*bundle) || !filepath.IsAbs(*lock) ||
		!filepath.IsAbs(*socket) || filepath.Clean(*socket) != *socket || *user == "" || os.Getenv("OJ_DATABASE_URL") != "" {
		return problemreference.ErrInvalid
	}
	raw, err := checkedfile.ReadSecret(*file, 16<<10)
	if err != nil {
		return err
	}
	var in problemvalidation.LocalTextAdmissionInput
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || dec.Decode(new(any)) != io.EOF || !in.Valid() {
		return problemreference.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, 40*time.Second)
	defer cancel()
	admission, err := loadLocalText(ctx, in, *bundle, *lock)
	if err != nil {
		return err
	}
	pool, err := openPublicationSocket(ctx, *socket, *user)
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
	reviews, err = reviews.WithLocalExecution(admission)
	if err != nil {
		return err
	}
	if err := reviews.RegisterLocalExecution(ctx); err != nil {
		return err
	}
	_, version, _ := admission.Binding()
	_, err = fmt.Fprintln(out, version)
	return err
}
