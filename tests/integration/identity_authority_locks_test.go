//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func testAuthorityLocks(t *testing.T, ctx context.Context, a, b *pgxpool.Pool, first, second *identitypg.Store, h *password.Hasher) {
	t.Run("two authority operations share the row", func(t *testing.T) {
		row, err := first.Initialize(ctx, identityID(608), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		gate, entered, release := verificationGate(t)
		op := func(call context.Context, state identity.State) error {
			if state != row.State {
				t.Error("shared operation received wrong state")
			}
			return gate(call)
		}
		one, two := make(chan error, 1), make(chan error, 1)
		go func() { one <- first.WithCredentials(ctx, row.ID, row.Revision, op) }()
		go func() { two <- second.WithAccount(ctx, row.ID, row.AuthGeneration, op) }()
		awaitVerificationGate(t, entered)
		awaitVerificationGate(t, entered)
		release()
		must(t, awaitAuthority(t, one))
		must(t, awaitAuthority(t, two))
		sameIdentity(t, ctx, first, row)
	})
	for i, change := range []string{"rehash", "replace", "disable"} {
		t.Run("guard blocks "+change, func(t *testing.T) {
			row, err := first.Initialize(ctx, identityID(610+i), true, identityRecord(t, h, "synthetic original"))
			must(t, err)
			gate, entered, release := verificationGate(t)
			guardDone := make(chan error, 1)
			go func() {
				guardDone <- first.WithCredentials(ctx, row.ID, row.Revision, func(call context.Context, state identity.State) error {
					if state != row.State {
						t.Error("guard received wrong state")
					}
					return gate(call)
				})
			}()
			awaitVerificationGate(t, entered)
			backend := authorityBackend(t, ctx, b)
			next := identityRecord(t, h, "synthetic original")
			if change == "replace" {
				next = identityRecord(t, h, "synthetic replacement")
			}
			changed := make(chan error, 1)
			go func() {
				var err error
				switch change {
				case "rehash":
					_, err = second.RehashCAS(ctx, row.ID, row.Revision, next)
				case "replace":
					_, err = second.ReplacePasswordCAS(ctx, row.ID, row.Revision, next)
				case "disable":
					_, err = second.DisableCAS(ctx, row.ID, row.Revision)
				}
				changed <- err
			}()
			waitIdentityBlocked(t, ctx, b, backend.pid)
			sameIdentity(t, ctx, second, row)
			release()
			must(t, awaitAuthority(t, guardDone))
			must(t, awaitAuthority(t, changed))
			want := row
			switch change {
			case "rehash":
				want.Record, want.CredentialVersion = next, 2
			case "replace":
				want.Record, want.CredentialVersion, want.AuthGeneration = next, 2, 2
			case "disable":
				want.Enabled, want.AuthGeneration = false, 2
			}
			sameIdentity(t, ctx, first, want)
		})
	}
	// Override the server default on this fixture pool. A correct guard explicitly
	// selects READ COMMITTED, rather than inheriting REPEATABLE READ and failing
	// with serialization errors when an updated row is encountered after waiting.
	config := b.Config()
	config.MaxConns, config.MinConns, config.MinIdleConns = 2, 0, 0
	config.ConnConfig.RuntimeParams["default_transaction_isolation"] = "repeatable read"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	must(t, err)
	defer pool.Close()
	waiter, err := identitypg.New(pool, h)
	must(t, err)
	for i, scenario := range []string{"credentials after rehash", "account after rehash", "account after replace", "credentials after disable"} {
		t.Run(scenario, func(t *testing.T) {
			row, err := first.Initialize(ctx, identityID(620+i), true, identityRecord(t, h, "synthetic original"))
			must(t, err)
			want := row
			switch scenario {
			case "credentials after rehash", "account after rehash":
				want.Record, want.CredentialVersion = identityRecord(t, h, "synthetic original"), 2
			case "account after replace":
				want.Record, want.CredentialVersion, want.AuthGeneration = identityRecord(t, h, "synthetic replacement"), 2, 2
			case "credentials after disable":
				want.Enabled, want.AuthGeneration = false, 2
			}
			holder, err := a.Begin(ctx)
			must(t, err)
			defer holder.Rollback(ctx)
			var pid int32
			must(t, holder.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
			writeIdentityFixture(t, ctx, holder, want)
			calls := 0
			done := make(chan error, 1)
			op := func(_ context.Context, state identity.State) error {
				calls++
				if state != want.State {
					return errors.New("authority waiter received stale state")
				}
				return nil
			}
			go func() {
				if scenario == "account after rehash" || scenario == "account after replace" {
					done <- waiter.WithAccount(ctx, row.ID, row.AuthGeneration, op)
				} else {
					done <- waiter.WithCredentials(ctx, row.ID, row.Revision, op)
				}
			}()
			waitIdentityBlocked(t, ctx, b, pid)
			must(t, holder.Commit(ctx))
			err = awaitAuthority(t, done)
			if scenario == "account after rehash" {
				must(t, err)
				if calls != 1 {
					t.Fatal("same-password rehash revoked existing account authority")
				}
			} else if !errors.Is(err, identity.ErrConflict) || errors.Is(err, identity.ErrGuardedOperation) || calls != 0 {
				t.Fatal("authority waiter accepted stale revisions or used the wrong isolation")
			}
			sameIdentity(t, ctx, first, want)
		})
	}
}
