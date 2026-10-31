//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func identityHasher(t *testing.T, old bool) *password.Hasher {
	t.Helper()
	legacy := password.Parameters{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltBytes: 8, DigestBytes: 24}
	policy := password.Policy{Current: password.Parameters{MemoryKiB: 128, Iterations: 2, Parallelism: 2, SaltBytes: 16, DigestBytes: 32}, Legacy: []password.Parameters{legacy}}
	if old {
		policy = password.Policy{Current: legacy}
	}
	h, err := password.New(policy, password.Limits{MaxMemoryKiB: 256, MaxIterations: 4, MaxParallelism: 4,
		MaxPasswordBytes: 128, MaxSaltBytes: 64, MaxDigestBytes: 64})
	must(t, err)
	return h
}

func identityRecord(t *testing.T, h *password.Hasher, secret string) password.Record {
	t.Helper()
	r, err := h.Create([]byte(secret))
	must(t, err)
	return r
}

func identityID(n int) string { return fmt.Sprintf("%032x", n) }

func sameIdentity(t *testing.T, ctx context.Context, s *identitypg.Store, want identity.Snapshot) {
	t.Helper()
	got, err := s.Get(ctx, want.ID)
	must(t, err)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("complete identity snapshot changed unexpectedly")
	}
}

func testIdentityCredentials(t *testing.T, ctx context.Context, a, b *pgxpool.Pool, provider *goose.Provider, submissions *submissionpg.Store) {
	h := identityHasher(t, false)
	first, err := identitypg.New(a, h)
	must(t, err)
	second, err := identitypg.New(b, h)
	must(t, err)
	t.Run("migration and retained data", func(t *testing.T) { testIdentityMigrations(t, ctx, a, provider, submissions, h) })
	t.Run("round trip and guarded lifecycle", func(t *testing.T) {
		oldHasher := identityHasher(t, true)
		oldStore, err := identitypg.New(a, oldHasher)
		must(t, err)
		old := identityRecord(t, oldHasher, "synthetic original")
		row, err := oldStore.Initialize(ctx, identityID(100), true, old)
		must(t, err)
		if row.Revision != (identity.Revision{CredentialVersion: 1, AuthGeneration: 1}) || !row.Enabled {
			t.Fatal("wrong initial authority")
		}
		sameIdentity(t, ctx, second, row)
		reopened, err := identitypg.New(b, h)
		must(t, err)
		sameIdentity(t, ctx, reopened, row)
		fresh := identityRecord(t, h, "synthetic original")
		if _, err := first.Initialize(ctx, row.ID, false, fresh); err != identity.ErrConflict {
			t.Fatal("duplicate initialization overwrote credentials")
		}
		match, err := h.Verify([]byte("synthetic original"), row.Record)
		must(t, err)
		if !match.Match || !match.NeedsUpgrade {
			t.Fatal("stored legacy record did not verify")
		}
		row, err = first.RehashCAS(ctx, row.ID, row.Revision, fresh)
		must(t, err)
		if row.Revision != (identity.Revision{CredentialVersion: 2, AuthGeneration: 1}) {
			t.Fatal("rehash changed account revocation facts")
		}
		sameIdentity(t, ctx, second, row)
		beforeDisable := row
		rev, err := second.DisableCAS(ctx, row.ID, row.Revision)
		must(t, err)
		row.Enabled, row.Revision = false, rev
		if rev != (identity.Revision{CredentialVersion: 2, AuthGeneration: 2}) {
			t.Fatal("disable changed credential version or failed to revoke")
		}
		sameIdentity(t, ctx, first, row)
		if _, err := first.ReplacePasswordCAS(ctx, row.ID, beforeDisable.Revision, fresh); err != identity.ErrConflict {
			t.Fatal("replacement ignored a stale auth generation with unchanged credential version")
		}
		sameIdentity(t, ctx, second, row)
		if _, err := first.RehashCAS(ctx, row.ID, beforeDisable.Revision, fresh); err != identity.ErrConflict {
			t.Fatal("stale rehash survived disable")
		}
		if _, err := first.RehashCAS(ctx, row.ID, row.Revision, fresh); err != identity.ErrConflict {
			t.Fatal("disabled rehash allowed")
		}
		replacement := identityRecord(t, h, "synthetic replacement")
		row, err = first.ReplacePasswordCAS(ctx, row.ID, row.Revision, replacement)
		must(t, err)
		if row.Enabled || row.Revision != (identity.Revision{CredentialVersion: 3, AuthGeneration: 3}) {
			t.Fatal("password replacement enabled the account or failed to revoke")
		}
		sameIdentity(t, ctx, second, row)
	})
	t.Run("validation and storage constraints", func(t *testing.T) {
		r := identityRecord(t, h, "synthetic bounds")
		row, err := first.Initialize(ctx, identityID(101), true, r)
		must(t, err)
		for _, badID := range []string{"", "user@example.invalid", "../account", identityID(1) + "0"} {
			if _, err := first.Initialize(ctx, badID, true, r); err != identity.ErrInvalid {
				t.Fatal("bad ID initialized")
			}
			if _, err := first.Get(ctx, badID); err != identity.ErrInvalid {
				t.Fatal("bad ID read")
			}
			if _, err := first.State(ctx, badID); err != identity.ErrInvalid {
				t.Fatal("bad ID state read")
			}
		}
		if _, err := first.Get(ctx, identityID(999)); err != identity.ErrNotFound {
			t.Fatal("missing identity was not explicit")
		}
		if _, err := first.State(ctx, identityID(999)); err != identity.ErrNotFound {
			t.Fatal("missing state was not explicit")
		}
		for _, rev := range []identity.Revision{{}, {CredentialVersion: -1, AuthGeneration: 1}} {
			if _, err := first.ReplacePasswordCAS(ctx, row.ID, rev, r); err != identity.ErrInvalid {
				t.Fatal("invalid revision updated")
			}
			if _, err := first.RehashCAS(ctx, row.ID, rev, r); err != identity.ErrInvalid {
				t.Fatal("invalid revision rehashed")
			}
			if _, err := first.DisableCAS(ctx, row.ID, rev); err != identity.ErrInvalid {
				t.Fatal("invalid revision disabled")
			}
		}
		for _, mutate := range []func(*password.Record){
			func(r *password.Record) { r.FormatVersion++ }, func(r *password.Record) { r.ArgonVersion++ },
			func(r *password.Record) { r.Parameters.MemoryKiB = ^uint32(0) },
			func(r *password.Record) { r.Salt = nil }, func(r *password.Record) { r.Digest = nil },
		} {
			bad := r
			mutate(&bad)
			if _, err := first.ReplacePasswordCAS(ctx, row.ID, row.Revision, bad); !errors.Is(err, password.ErrRecord) {
				t.Fatal("invalid record reached persistence")
			}
		}
		legacy := identityRecord(t, identityHasher(t, true), "synthetic legacy")
		if _, err := first.Initialize(ctx, identityID(102), true, legacy); err != password.ErrRecord {
			t.Fatal("current store wrote a legacy record")
		}
		// Direct writes exercise database constraints independently of Go validation.
		for _, query := range []string{
			`UPDATE identity_credentials SET format_version=2 WHERE id=$1`,
			`UPDATE identity_credentials SET algorithm='argon2i' WHERE id=$1`,
			`UPDATE identity_credentials SET argon_version=18 WHERE id=$1`,
			`UPDATE identity_credentials SET memory_kib=7 WHERE id=$1`,
			`UPDATE identity_credentials SET memory_kib=129 WHERE id=$1`,
			`UPDATE identity_credentials SET iterations=4294967296 WHERE id=$1`,
			`UPDATE identity_credentials SET parallelism=256 WHERE id=$1`,
			`UPDATE identity_credentials SET salt_bytes=65,salt=decode(repeat('00',65),'hex') WHERE id=$1`,
			`UPDATE identity_credentials SET digest_bytes=65,digest=decode(repeat('00',65),'hex') WHERE id=$1`,
			`UPDATE identity_credentials SET salt=''::bytea WHERE id=$1`,
			`UPDATE identity_credentials SET credential_version=0 WHERE id=$1`,
			`UPDATE identity_credentials SET auth_generation=0 WHERE id=$1`,
		} {
			_, err := a.Exec(ctx, query, row.ID)
			if err == nil {
				t.Fatal("database constraint accepted invalid synthetic fixture")
			}
		}
		sameIdentity(t, ctx, second, row)
		tx, err := a.Begin(ctx)
		must(t, err)
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `UPDATE identity_credentials SET salt_bytes=64,digest_bytes=64,
            salt=decode(repeat('00',64),'hex'),digest=decode(repeat('01',64),'hex') WHERE id=$1`, row.ID)
		must(t, err)
		must(t, tx.Rollback(ctx))
		sameIdentity(t, ctx, second, row)
		tooLarge, err := password.New(password.Policy{Current: password.Parameters{MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltBytes: 128, DigestBytes: 32}},
			password.Limits{MaxMemoryKiB: 64, MaxIterations: 1, MaxParallelism: 1, MaxPasswordBytes: 64, MaxSaltBytes: 128, MaxDigestBytes: 32})
		must(t, err)
		if _, err := identitypg.New(a, tooLarge); err != identity.ErrInvalid {
			t.Fatal("incompatible storage envelope accepted")
		}
	})
	t.Run("concurrent updates and failure boundaries", func(t *testing.T) { testIdentityRaces(t, ctx, a, b, first, second, h) })
	t.Run("indexed read and CAS plans", func(t *testing.T) { testIdentityPlans(t, ctx, a) })
}
