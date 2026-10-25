package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open creates a bounded M0 pool; production budgets require load validation.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "5000"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "3000"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("cannot create database pool")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("database unavailable")
	}
	return pool, nil
}
