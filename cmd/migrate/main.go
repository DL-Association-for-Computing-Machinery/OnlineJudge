// Command migrate changes the database schema independently from the API.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func main() {
	if err := run(); err != nil {
		// Driver errors may contain connection credentials. Keep CLI output safe.
		fmt.Fprintln(os.Stderr, "migration failed; check database access, lock contention and schema compatibility")
		os.Exit(1)
	}
}

func run() error {
	var limits postgres.BacklogLimits
	var referenceLimits problemreference.Limits
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
	} else if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down" && os.Args[1] != "fixture") {
		return fmt.Errorf("usage: migrate up|down|fixture or migrate backlog|reference-backlog <global> <subject>")
	}
	dsn := os.Getenv("OJ_DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("OJ_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if os.Args[1] == "fixture" || os.Args[1] == "backlog" || os.Args[1] == "reference-backlog" {
		pool, err := postgres.Open(ctx, dsn)
		if err != nil {
			return err
		}
		defer pool.Close()
		if err := db.CheckSchema(ctx, pool); err != nil {
			return err
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
