//go:build integration

package integration_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func awaitAuthority(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("authority operation did not finish")
		return nil
	}
}

func authorityBackend(t *testing.T, ctx context.Context, pool *pgxpool.Pool) identityBackend {
	t.Helper()
	var backend identityBackend
	rows, err := pool.Query(ctx, `SELECT pid,backend_start FROM pg_stat_activity
        WHERE datname=current_database() AND state='idle in transaction'
        AND query LIKE '%name: GetStateForShare%'`)
	must(t, err)
	defer rows.Close()
	count := 0
	for rows.Next() {
		must(t, rows.Scan(&backend.pid, &backend.started))
		count++
	}
	must(t, rows.Err())
	if count != 1 {
		t.Fatalf("expected exactly one owned guard backend, got %d", count)
	}
	return backend
}

func testIdentityAuthority(t *testing.T, ctx context.Context, a, b *pgxpool.Pool) {
	h := identityHasher(t, false)
	first, err := identitypg.New(a, h)
	must(t, err)
	second, err := identitypg.New(b, h)
	must(t, err)
	t.Run("invalid or stale authority never invokes operation", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(601), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		calls := 0
		op := func(context.Context, identity.State) error { calls++; return nil }
		var absent context.Context
		for _, call := range []func() error{
			func() error { return first.WithCredentials(ctx, "invalid", row.Revision, op) },
			func() error { return first.WithCredentials(ctx, row.ID, identity.Revision{}, op) },
			func() error { return first.WithAccount(ctx, row.ID, 0, op) },
			func() error { return first.WithCredentials(ctx, row.ID, row.Revision, nil) },
			func() error { return first.WithAccount(absent, row.ID, row.AuthGeneration, op) },
			func() error { return new(identitypg.Store).WithAccount(ctx, row.ID, row.AuthGeneration, op) },
		} {
			mustCredentialError(t, call(), identity.ErrInvalid)
		}
		for _, call := range []func() error{
			func() error { return first.WithAccount(ctx, identityID(999), row.AuthGeneration, op) },
			func() error {
				return first.WithCredentials(ctx, row.ID, identity.Revision{CredentialVersion: 2, AuthGeneration: 1}, op)
			},
			func() error {
				return first.WithCredentials(ctx, row.ID, identity.Revision{CredentialVersion: 1, AuthGeneration: 2}, op)
			},
			func() error { return first.WithAccount(ctx, row.ID, 2, op) },
		} {
			mustCredentialError(t, call(), identity.ErrConflict)
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		mustCredentialError(t, first.WithAccount(canceled, row.ID, row.AuthGeneration, op), context.Canceled)
		rev, err := second.DisableCAS(ctx, row.ID, row.Revision)
		must(t, err)
		mustCredentialError(t, first.WithAccount(ctx, row.ID, rev.AuthGeneration, op), identity.ErrConflict)
		mustCredentialError(t, first.WithCredentials(ctx, row.ID, rev, op), identity.ErrConflict)
		if calls != 0 {
			t.Fatal("invalid, canceled, or stale authority invoked operation")
		}
	})
	t.Run("rehash preserves existing account authority only", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(602), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		fresh, err := second.RehashCAS(ctx, row.ID, row.Revision, identityRecord(t, h, "synthetic original"))
		must(t, err)
		calls := 0
		op := func(call context.Context, state identity.State) error {
			calls++
			deadline, ok := call.Deadline()
			if state != fresh.State || !ok || time.Until(deadline) > 5*time.Second {
				t.Fatal("operation received wrong state or no bounded context")
			}
			return nil
		}
		mustCredentialError(t, first.WithCredentials(ctx, row.ID, row.Revision, op), identity.ErrConflict)
		must(t, first.WithAccount(ctx, row.ID, row.AuthGeneration, op))
		must(t, second.WithCredentials(ctx, row.ID, fresh.Revision, op))
		if calls != 2 {
			t.Fatal("wrong rehash authority distinction")
		}
		sameIdentity(t, ctx, first, fresh)
	})
	t.Run("retired password record does not prevent account state guard", func(t *testing.T) {
		old := identityHasher(t, true)
		writer, err := identitypg.New(a, old)
		must(t, err)
		row, err := writer.Initialize(ctx, identityID(603), true, identityRecord(t, old, "synthetic original"))
		must(t, err)
		current := identityRecord(t, h, "synthetic current").Parameters
		retired, err := password.New(password.Policy{Current: current}, password.Limits{MaxMemoryKiB: 256, MaxIterations: 4, MaxParallelism: 4,
			MaxPasswordBytes: 128, MaxSaltBytes: 64, MaxDigestBytes: 64})
		must(t, err)
		store, err := identitypg.New(b, retired)
		must(t, err)
		_, err = store.Get(ctx, row.ID)
		mustCredentialError(t, err, password.ErrRecord)
		calls := 0
		must(t, store.WithAccount(ctx, row.ID, row.AuthGeneration, func(_ context.Context, state identity.State) error {
			calls++
			if state != row.State {
				t.Fatal("authority guard decoded or changed password material")
			}
			return nil
		}))
		if calls != 1 {
			t.Fatal("state-only authority operation was not invoked once")
		}
	})
	t.Run("callback error and panic release locks without masking effects", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(604), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		private := errors.New("synthetic private callback detail")
		calls := 0
		err = first.WithAccount(ctx, row.ID, row.AuthGeneration, func(context.Context, identity.State) error {
			calls++
			return errors.Join(private, identity.ErrConflict)
		})
		if !errors.Is(err, identity.ErrGuardedOperation) || !errors.Is(err, private) || !errors.Is(err, identity.ErrConflict) ||
			errors.Is(err, identity.ErrStore) || err.Error() != identity.ErrGuardedOperation.Error() || calls != 1 {
			t.Fatal("callback error was exposed, retried, or misclassified")
		}
		panicked := false
		func() {
			defer func() { panicked = recover() == private }()
			_ = first.WithCredentials(ctx, row.ID, row.Revision, func(context.Context, identity.State) error { panic(private) })
		}()
		if !panicked {
			t.Fatal("callback panic was swallowed")
		}
		_, err = second.DisableCAS(ctx, row.ID, row.Revision)
		must(t, err)
	})
	t.Run("shared locks and mutation ordering", func(t *testing.T) { testAuthorityLocks(t, ctx, a, b, first, second, h) })
	t.Run("cancellation and lost backend", func(t *testing.T) { testAuthorityFailures(t, ctx, a, b, first, second, h) })
	t.Run("natural primary key locking plan", func(t *testing.T) {
		tx, err := a.Begin(ctx)
		must(t, err)
		defer tx.Rollback(ctx)
		rows, err := tx.Query(ctx, `EXPLAIN (ANALYZE,BUFFERS) SELECT id,enabled,credential_version,auth_generation
            FROM identity_credentials WHERE id=$1 FOR SHARE`, identityID(5000))
		must(t, err)
		var plan strings.Builder
		for rows.Next() {
			var line string
			must(t, rows.Scan(&line))
			plan.WriteString(line + "\n")
		}
		must(t, rows.Err())
		rows.Close()
		if !strings.Contains(plan.String(), "identity_credentials_pkey") || !strings.Contains(plan.String(), "LockRows") {
			t.Fatal("locking point lookup did not naturally use the primary key")
		}
		t.Log(plan.String())
	})
}
