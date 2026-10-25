// Command migrate changes the database schema independently from the API.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
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
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down" && os.Args[1] != "fixture") {
		return fmt.Errorf("usage: migrate up|down|fixture")
	}
	dsn := os.Getenv("OJ_DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("OJ_DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if os.Args[1] == "fixture" {
		pool, err := postgres.Open(ctx, dsn)
		if err != nil {
			return err
		}
		defer pool.Close()
		if err := db.CheckSchema(ctx, pool); err != nil {
			return err
		}
		store, err := postgres.New(pool, experiment.Version, experiment.Manifest())
		if err != nil {
			return err
		}
		if err := store.RegisterFixture(ctx); err != nil {
			return err
		}
		fmt.Println("M0 synthetic fixture registered; execution remains unvalidated")
		return nil
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer conn.Close()
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
