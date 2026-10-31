//go:build integration

package integration_test

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func waitIdentityBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pid int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for {
		var blocked bool
		must(t, pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1::integer=ANY(pg_blocking_pids(pid)))`, pid).Scan(&blocked))
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("identity writer never blocked")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func writeIdentityFixture(t *testing.T, ctx context.Context, tx pgx.Tx, row identity.Snapshot) {
	t.Helper()
	r, p := row.Record, row.Record.Parameters
	_, err := tx.Exec(ctx, `UPDATE identity_credentials SET enabled=$2,credential_version=$3,auth_generation=$4,
        format_version=$5,algorithm=$6,argon_version=$7,memory_kib=$8,iterations=$9,parallelism=$10,
        salt_bytes=$11,digest_bytes=$12,salt=$13,digest=$14 WHERE id=$1`, row.ID, row.Enabled,
		row.CredentialVersion, row.AuthGeneration, int32(r.FormatVersion), r.Algorithm, int32(r.ArgonVersion),
		int64(p.MemoryKiB), int64(p.Iterations), int16(p.Parallelism), int32(p.SaltBytes), int32(p.DigestBytes), r.Salt, r.Digest)
	must(t, err)
}

func testIdentityRaces(t *testing.T, ctx context.Context, a, b *pgxpool.Pool, first, second *identitypg.Store, h *password.Hasher) {
	for index, rehash := range []bool{true, false} {
		name := "two replacements"
		if rehash {
			name = "two rehashes"
		}
		t.Run(name, func(t *testing.T) {
			old, err := first.Initialize(ctx, identityID(200+index), true, identityRecord(t, h, "synthetic original"))
			must(t, err)
			records := []password.Record{identityRecord(t, h, "synthetic original"), identityRecord(t, h, "synthetic original")}
			type result struct {
				row   identity.Snapshot
				err   error
				index int
			}
			results, start := make(chan result, 2), make(chan struct{})
			for i, store := range []*identitypg.Store{first, second} {
				go func() {
					<-start
					var row identity.Snapshot
					var err error
					if rehash {
						row, err = store.RehashCAS(ctx, old.ID, old.Revision, records[i])
					} else {
						row, err = store.ReplacePasswordCAS(ctx, old.ID, old.Revision, records[i])
					}
					results <- result{row, err, i}
				}()
			}
			close(start)
			wins := 0
			for range 2 {
				r := <-results
				if r.err == identity.ErrConflict {
					continue
				}
				must(t, r.err)
				wins++
				want := old
				want.Record, want.CredentialVersion = records[r.index], 2
				if !rehash {
					want.AuthGeneration = 2
				}
				if !reflect.DeepEqual(r.row, want) {
					t.Fatal("winner returned inconsistent complete row")
				}
				sameIdentity(t, ctx, second, want)
			}
			if wins != 1 {
				t.Fatal("CAS did not have exactly one winner")
			}
		})
	}
	// The holder writes authoritative facts after the competing Store call has
	// actually blocked. Its commit forces PostgreSQL to recheck both predicates.
	for index, winner := range []string{"replace", "disable", "rehash-before-replace", "rehash-before-disable"} {
		t.Run("wait then "+winner, func(t *testing.T) {
			old, err := first.Initialize(ctx, identityID(210+index), true, identityRecord(t, h, "synthetic original"))
			must(t, err)
			next := identityRecord(t, h, "synthetic original")
			tx, err := a.Begin(ctx)
			must(t, err)
			defer tx.Rollback(ctx)
			var pid int32
			must(t, tx.QueryRow(ctx, `SELECT pg_backend_pid() FROM identity_credentials WHERE id=$1 FOR UPDATE`, old.ID).Scan(&pid))
			result := make(chan error, 1)
			go func() {
				var err error
				switch winner {
				case "rehash-before-replace":
					_, err = second.ReplacePasswordCAS(ctx, old.ID, old.Revision, next)
				case "rehash-before-disable":
					_, err = second.DisableCAS(ctx, old.ID, old.Revision)
				default:
					_, err = second.RehashCAS(ctx, old.ID, old.Revision, next)
				}
				result <- err
			}()
			waitIdentityBlocked(t, ctx, a, pid)
			want := old
			switch winner {
			case "disable":
				want.Enabled, want.AuthGeneration = false, 2
			case "replace":
				want.Record, want.CredentialVersion, want.AuthGeneration = next, 2, 2
			default:
				want.Record, want.CredentialVersion = next, 2
			}
			writeIdentityFixture(t, ctx, tx, want)
			must(t, tx.Commit(ctx))
			if err := <-result; err != identity.ErrConflict {
				t.Fatal("blocked stale writer changed new authority")
			}
			sameIdentity(t, ctx, first, want)
		})
	}
	t.Run("caller cancellation while row locked", func(t *testing.T) {
		old, err := first.Initialize(ctx, identityID(220), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		tx, err := a.Begin(ctx)
		must(t, err)
		defer tx.Rollback(ctx)
		var pid int32
		must(t, tx.QueryRow(ctx, `SELECT pg_backend_pid() FROM identity_credentials WHERE id=$1 FOR UPDATE`, old.ID).Scan(&pid))
		short, cancel := context.WithCancel(ctx)
		defer cancel()
		result := make(chan error, 1)
		go func() { _, err := second.DisableCAS(short, old.ID, old.Revision); result <- err }()
		waitIdentityBlocked(t, ctx, a, pid)
		cancel()
		err = <-result
		if !errors.Is(err, identity.ErrStore) || !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation lost driver cause or storage uncertainty")
		}
		must(t, tx.Rollback(ctx))
		sameIdentity(t, ctx, first, old)
	})
	t.Run("statement failure rolls back every field", func(t *testing.T) {
		old, err := first.Initialize(ctx, identityID(221), true, identityRecord(t, h, "synthetic original"))
		must(t, err)
		next := identityRecord(t, h, "synthetic replacement")
		_, err = a.Exec(ctx, `CREATE FUNCTION reject_identity_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private injected detail' USING ERRCODE='P0001'; END $$;
            CREATE TRIGGER reject_identity_update AFTER UPDATE ON identity_credentials FOR EACH ROW EXECUTE FUNCTION reject_identity_update()`)
		must(t, err)
		defer func() {
			_, err := a.Exec(ctx, `DROP TRIGGER reject_identity_update ON identity_credentials; DROP FUNCTION reject_identity_update()`)
			must(t, err)
		}()
		for _, change := range []func() error{
			func() error { _, err := first.RehashCAS(ctx, old.ID, old.Revision, next); return err },
			func() error { _, err := second.ReplacePasswordCAS(ctx, old.ID, old.Revision, next); return err },
			func() error { _, err := second.DisableCAS(ctx, old.ID, old.Revision); return err },
		} {
			err := change()
			if !errors.Is(err, identity.ErrStore) || err.Error() != identity.ErrStore.Error() {
				t.Fatal("storage failure was hidden or leaked details")
			}
			sameIdentity(t, ctx, second, old)
		}
	})
	t.Run("positive bigint counters never wrap", func(t *testing.T) {
		for i, maxAuth := range []bool{false, true} {
			old, err := first.Initialize(ctx, identityID(230+i), true, identityRecord(t, h, "synthetic original"))
			must(t, err)
			if maxAuth {
				old.AuthGeneration = math.MaxInt64
			} else {
				old.CredentialVersion = math.MaxInt64
			}
			tx, err := a.Begin(ctx)
			must(t, err)
			writeIdentityFixture(t, ctx, tx, old)
			must(t, tx.Commit(ctx))
			next := identityRecord(t, h, "synthetic next")
			if _, err := second.ReplacePasswordCAS(ctx, old.ID, old.Revision, next); err != identity.ErrConflict {
				t.Fatal("replacement overflowed a counter")
			}
			if maxAuth {
				if _, err := second.DisableCAS(ctx, old.ID, old.Revision); err != identity.ErrConflict {
					t.Fatal("disable overflowed auth generation")
				}
			} else {
				if _, err := second.RehashCAS(ctx, old.ID, old.Revision, next); err != identity.ErrConflict {
					t.Fatal("rehash overflowed credential version")
				}
			}
			sameIdentity(t, ctx, first, old)
		}
	})
	t.Run("retired record cannot prevent disable", func(t *testing.T) {
		oldHasher := identityHasher(t, true)
		oldStore, err := identitypg.New(a, oldHasher)
		must(t, err)
		old, err := oldStore.Initialize(ctx, identityID(240), true, identityRecord(t, oldHasher, "synthetic old"))
		must(t, err)
		current := identityRecord(t, h, "synthetic current").Parameters
		retired, err := password.New(password.Policy{Current: current}, password.Limits{MaxMemoryKiB: 256, MaxIterations: 4, MaxParallelism: 4, MaxPasswordBytes: 128, MaxSaltBytes: 64, MaxDigestBytes: 64})
		must(t, err)
		store, err := identitypg.New(b, retired)
		must(t, err)
		if _, err := store.Get(ctx, old.ID); err != password.ErrRecord {
			t.Fatal("retired tuple still decoded")
		}
		state, err := store.State(ctx, old.ID)
		must(t, err)
		if state != old.State {
			t.Fatal("fresh revoker could not read authority without password material")
		}
		rev, err := store.DisableCAS(ctx, old.ID, state.Revision)
		must(t, err)
		old.Enabled, old.Revision = false, rev
		sameIdentity(t, ctx, first, old)
	})
}
