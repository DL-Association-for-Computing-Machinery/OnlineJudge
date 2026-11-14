// Command migrate changes the database schema independently from the API.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

var errRestoredDatabaseURL = errors.New("OJ_RESTORED_DATABASE_URL is required")

func main() {
	if len(os.Args) > 1 && os.Args[1] == "schema-status" {
		if err := runSchemaStatus(os.Args[2:], os.Getenv, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "judge-snapshot" {
		if err := runJudgeSnapshot(os.Args[2:], os.Getenv, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "rejudge" {
		if err := runRejudge(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "mail-budget-init" || os.Args[1] == "mail-budget-provision") {
		if err := runMailBudget(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "mail-payload-sweep" {
		if err := runMailPayloadSweep(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "mail-send-rate" {
		if err := runMailSendRate(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "mail-queue-up" {
		if err := runMailQueueUp(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "verify-backup" {
		if err := runVerifyBackup(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "capture-backup" {
		if err := runCaptureBackup(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "session-recovery" {
		if err := runSessionRecovery(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "restored-reference" {
		if err := runRestoredReference(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "verify-restored-materials" {
		if err := runVerifyRestoredMaterials(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		// Driver errors may contain connection credentials. Keep CLI output safe.
		switch {
		case errors.Is(err, postgres.ErrRestoreDatabase):
			fmt.Fprintln(os.Stderr, "restore fencing refused: database name did not match confirmation")
		case errors.Is(err, errRestoredDatabaseURL):
			fmt.Fprintln(os.Stderr, "restore fencing refused: OJ_RESTORED_DATABASE_URL is required")
		case errors.Is(err, postgres.ErrRestoreState):
			fmt.Fprintln(os.Stderr, "restore fencing refused: a running task has no matching lease")
		default:
			fmt.Fprintln(os.Stderr, "migration failed; check database access, lock contention and schema compatibility")
		}
		os.Exit(1)
	}
}

func run() error {
	var limits postgres.BacklogLimits
	var referenceLimits problemreference.Limits
	fence := len(os.Args) == 3 && os.Args[1] == "fence-restored-leases"
	if len(os.Args) == 4 && (os.Args[1] == "backlog" || os.Args[1] == "reference-backlog") {
		global, globalErr := strconv.ParseInt(os.Args[2], 10, 64)
		subject, subjectErr := strconv.ParseInt(os.Args[3], 10, 64)
		limits = postgres.BacklogLimits{Global: global, Subject: subject}
		referenceLimits = problemreference.Limits{Global: global, Owner: subject}
		if globalErr != nil || subjectErr != nil ||
			(os.Args[1] == "backlog" && !limits.Valid()) ||
			(os.Args[1] == "reference-backlog" && !referenceLimits.Valid()) {
			return fmt.Errorf("explicit positive global and subject limits required")
		}
	} else if !fence && (len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down" && os.Args[1] != "fixture")) {
		return fmt.Errorf("usage: migrate up|down|fixture, migrate backlog|reference-backlog <global> <subject>, or migrate fence-restored-leases <restored-database-name>")
	}
	var dsn string
	if fence {
		dsn = os.Getenv("OJ_RESTORED_DATABASE_URL")
		if dsn == "" {
			return errRestoredDatabaseURL
		}
	} else if dsn = os.Getenv("OJ_DATABASE_URL"); dsn == "" {
		return fmt.Errorf("OJ_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if fence || os.Args[1] == "fixture" || os.Args[1] == "backlog" || os.Args[1] == "reference-backlog" {
		pool, err := postgres.Open(ctx, dsn)
		if err != nil {
			return err
		}
		defer pool.Close()
		if err := db.CheckSchema(ctx, pool); err != nil {
			return err
		}
		if fence {
			changes, err := postgres.FenceRestoredLeases(ctx, pool, os.Args[2])
			if err != nil {
				return err
			}
			fmt.Printf("restored leases expired=%d tasks requeued=%d\n", changes.Expired, changes.Requeued)
			return nil
		}
		if os.Args[1] == "backlog" {
			if err := postgres.ConfigureBacklog(ctx, pool, limits); err != nil {
				return err
			}
			fmt.Println("submission backlog policy configured")
			return nil
		}
		if os.Args[1] == "reference-backlog" {
			if err := referencepg.Configure(ctx, pool, referenceLimits); err != nil {
				return err
			}
			fmt.Println("reference request backlog policy configured")
			return nil
		}
		version, manifest, err := experiment.SelectFixture(os.Getenv("OJ_EXPERIMENT_MATERIAL_VERSION"))
		if err != nil {
			return err
		}
		aplusb, aplusbFixture, err := experiment.LoadAPlusBFixture(os.Getenv)
		if err != nil {
			return err
		}
		if aplusbFixture.IsAPlusB() {
			reader, err := materials.OpenLocal(os.Getenv("OJ_MATERIAL_ROOT"))
			if err != nil {
				return err
			}
			defer func() { _ = reader.Close() }()
			for _, point := range aplusb.Tests {
				for _, hash := range []string{point.Input, point.Answer} {
					if err := reader.Verify(ctx, hash); err != nil {
						return err
					}
				}
			}
			store, err := postgres.New(pool, aplusb.Version, aplusbFixture.Manifest())
			if err != nil {
				return err
			}
			if err := store.RegisterFixture(ctx); err != nil {
				return err
			}
			fmt.Println("A+B isolated materials registered:", aplusb.Version)
			return nil
		}
		store, err := postgres.New(pool, version, manifest)
		if err != nil {
			return err
		}
		if path := os.Getenv("OJ_MATERIAL_ROOT"); path != "" {
			if err := experiment.WriteMaterials(path); err != nil {
				return err
			}
		}
		if err := store.RegisterFixture(ctx); err != nil {
			return err
		}
		fmt.Println("M0 synthetic fixture registered:", version, "; full Worker execution remains unvalidated")
		return nil
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	// Cleanup does not change the completed operation or its primary error.
	defer func() { _ = conn.Close() }()
	conn.SetMaxOpenConns(2)
	provider, err := db.Provider(conn)
	if err != nil {
		return err
	}
	if os.Args[1] == "up" {
		_, err = provider.Up(ctx)
	} else {
		_, err = provider.Down(ctx)
	}
	if err != nil {
		return err
	}
	fmt.Println("migration completed")
	return nil
}
