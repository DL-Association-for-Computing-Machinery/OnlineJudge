package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

var errRejudgeUsage = errors.New("usage: migrate rejudge status <database> <request-key> | enqueue <database> <submission-id> <previous-generation> <material-version> <manifest-sha256> <request-key> <reason> --confirm-rejudge")

func runRejudge(args []string) error {
	var intent postgres.RejudgeIntent
	switch {
	case len(args) == 3 && args[0] == "status" && args[1] != "" && submission.ValidKey(args[2]):
	case len(args) == 9 && args[0] == "enqueue" && args[1] != "" && args[8] == "--confirm-rejudge":
		generation, err := strconv.ParseInt(args[3], 10, 64)
		intent = postgres.RejudgeIntent{ID: args[2], PreviousGeneration: generation, Version: args[4], Digest: args[5], Key: args[6], Reason: args[7]}
		if err != nil || !intent.Valid() {
			return errRejudgeUsage
		}
	default:
		return errRejudgeUsage
	}
	dsn := os.Getenv("OJ_REJUDGE_DATABASE_URL")
	if dsn == "" {
		return errors.New("OJ_REJUDGE_DATABASE_URL is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		return errors.New("rejudge database unavailable")
	}
	defer pool.Close()
	if err := db.CheckSchema(ctx, pool); err != nil {
		return err
	}
	var receipt postgres.RejudgeReceipt
	if args[0] == "status" {
		receipt, err = postgres.ReadRejudge(ctx, pool, args[1], args[2])
	} else {
		receipt, err = postgres.Rejudge(ctx, pool, args[1], intent, verifyRejudgeMaterial)
	}
	if err != nil {
		for _, known := range []error{postgres.ErrRejudgeOperator, postgres.ErrRestoreDatabase, postgres.ErrRejudgeState,
			postgres.ErrRejudgeUnknown, submission.ErrNotFound, submission.ErrInvalid, submission.ErrConflict,
			submission.ErrMaterial, submission.ErrBacklog, submission.ErrAdmissionPolicy} {
			if errors.Is(err, known) {
				return known
			}
		}
		return errors.New("rejudge unavailable; check database access and read the original request key")
	}
	if err := json.NewEncoder(os.Stdout).Encode(receipt); err != nil {
		return postgres.ErrRejudgeUnknown
	}
	return nil
}

func verifyRejudgeMaterial(ctx context.Context, version, digest string, manifest []byte) error {
	return verifyRejudgeMaterialWithEnv(ctx, version, digest, manifest, os.Getenv)
}

func verifyRejudgeMaterialWithEnv(ctx context.Context, version, digest string, manifest []byte, getenv func(string) string) error {
	fixture, err := rejudgeFixture(version, digest, manifest, getenv)
	if err != nil {
		return submission.ErrMaterial
	}
	path := getenv("OJ_MATERIAL_ROOT")
	if !filepath.IsAbs(path) {
		return submission.ErrMaterial
	}
	return fixture.VerifyMaterials(ctx, path)
}

func rejudgeFixture(version, digest string, manifest []byte, getenv func(string) string) (experiment.WorkerFixture, error) {
	_, f, err := experiment.LoadCachedFixture(getenv)
	if err != nil {
		return experiment.WorkerFixture{}, submission.ErrMaterial
	}
	if !f.UsesCachedText() {
		switch configured := getenv("OJ_EXPERIMENT_MATERIAL_VERSION"); {
		case configured != "":
			f, err = experiment.ParseWorkerFixture(configured)
		case version == experiment.PinnedVersion || version == experiment.UbuntuVersion:
			f, err = experiment.ParseWorkerFixture(version)
		default:
			// Preserve the original Desktop maintenance contract. Ubuntu A+B
			// and local text always require independent operator configuration.
			f, err = experiment.APlusBFixture(manifest, digest)
		}
	}
	if err != nil || !f.MatchesMaterial(version, digest, manifest) {
		return experiment.WorkerFixture{}, submission.ErrMaterial
	}
	return f, nil
}
