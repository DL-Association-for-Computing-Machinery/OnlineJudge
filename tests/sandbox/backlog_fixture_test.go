//go:build sandbox

package sandbox_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func configureSandboxBacklog(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	// Explicit synthetic fixture limits, never defaults or production capacity.
	if err := postgres.ConfigureBacklog(ctx, pool, postgres.BacklogLimits{Global: 10000, Subject: 10000}); err != nil {
		t.Fatal("cannot configure synthetic backlog policy")
	}
}
