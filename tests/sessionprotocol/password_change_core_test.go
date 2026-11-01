//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type passwordChangeStore struct {
	*identitypg.Store
	get     func(context.Context, string) (identity.Snapshot, error)
	replace func(context.Context, string, identity.Revision, string, password.Record) (identity.Snapshot, error)
}

func (s *passwordChangeStore) Get(ctx context.Context, id string) (identity.Snapshot, error) {
	if s.get != nil {
		return s.get(ctx, id)
	}
	return s.Store.Get(ctx, id)
}

func (s *passwordChangeStore) ReplacePasswordForSession(ctx context.Context, id string, want identity.Revision, recovery string, record password.Record) (identity.Snapshot, error) {
	if s.replace != nil {
		return s.replace(ctx, id, want, recovery, record)
	}
	return s.Store.ReplacePasswordForSession(ctx, id, want, recovery, record)
}

func changeVersion(t *testing.T, f *fixture, auth int64) identity.SessionVersion {
	t.Helper()
	r, err := f.identities[0].RecoveryState(f.ctx)
	must(t, err)
	if !r.Enabled {
		t.Fatal("fixture recovery is not open")
	}
	return identity.SessionVersion{AuthGeneration: auth, RecoveryID: r.Generation}
}

func changer(t *testing.T, f *fixture, store identity.PasswordChangeStore) *identity.PasswordChanger {
	t.Helper()
	c, err := identity.NewPasswordChanger(store, f.budget)
	must(t, err)
	return c
}

func testPasswordChangeCore(t *testing.T, f *fixture) {
	t.Run("concurrent legacy replacement has exactly one winner", func(t *testing.T) {
		p := f.subject(t, true)
		version := changeVersion(t, f, p.AuthGeneration)
		ready, release := make(chan struct{}, 2), make(chan struct{})
		type result struct {
			state identity.State
			err   error
			index int
		}
		results := make(chan result, 2)
		for index := range 2 {
			owner := f.identities[index]
			store := &passwordChangeStore{Store: owner, get: func(ctx context.Context, id string) (identity.Snapshot, error) {
				row, err := owner.Get(ctx, id)
				ready <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return identity.Snapshot{}, ctx.Err()
				}
				return row, err
			}}
			c := changer(t, f, store)
			go func() {
				state, err := c.Change(t.Context(), p.ID, version, []byte("synthetic protocol password"), []byte([]string{"synthetic replacement alpha", "synthetic replacement beta"}[index]))
				results <- result{state, err, index}
			}()
		}
		for range 2 {
			select {
			case <-ready:
			case <-time.After(3 * time.Second):
				t.Fatal("concurrent changes did not capture initial revision")
			}
		}
		close(release)
		winner := -1
		for range 2 {
			r := passwordChangeResult(t, results)
			if r.err == nil {
				if winner != -1 || r.state.CredentialVersion != 2 || r.state.AuthGeneration != 2 {
					t.Fatal("concurrent replacement duplicated or first rehashed")
				}
				winner = r.index
			} else if !errors.Is(r.err, identity.ErrConflict) || r.state != (identity.State{}) {
				t.Fatal("losing replacement returned authority or unexpected error")
			}
		}
		if winner < 0 {
			t.Fatal("neither concurrent replacement succeeded")
		}
		row, err := f.identities[0].Get(f.ctx, p.ID)
		must(t, err)
		verified, err := f.hasher.Verify([]byte([]string{"synthetic replacement alpha", "synthetic replacement beta"}[winner]), row.Record)
		must(t, err)
		if !verified.Match || verified.NeedsUpgrade || row.CredentialVersion != 2 || row.AuthGeneration != 2 {
			t.Fatal("winner was not one current replacement")
		}
	})
	t.Run("original session generation is never relabeled", func(t *testing.T) {
		p := f.subject(t, false)
		ns := namespace(t)
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		_, credential := f.create(t, s, p)
		view, err := s.Read(f.ctx, credential)
		must(t, err)
		version := identity.SessionVersion{AuthGeneration: view.AuthGeneration, RecoveryID: view.RecoveryID}
		c := changer(t, f, f.identities[0])
		state, err := c.Change(t.Context(), p.ID, version, []byte("synthetic protocol password"), []byte("synthetic replacement"))
		must(t, err)
		if state.CredentialVersion != 2 || state.AuthGeneration != 2 {
			t.Fatal("replacement did not advance both revisions")
		}
		_, err = s.Read(f.ctx, credential)
		wantError(t, err, identity.ErrConflict)
		state, err = c.Change(t.Context(), p.ID, version, []byte("synthetic replacement"), []byte("synthetic second replacement"))
		if !errors.Is(err, identity.ErrConflict) || state != (identity.State{}) {
			t.Fatal("old session fence adopted new auth generation")
		}
		exists, err := f.control.Exists(f.ctx, tokenKey(ns, credential)).Result()
		must(t, err)
		if exists != 1 {
			t.Fatal("password change relied on deleting old Redis credentials")
		}
	})
	t.Run("final replacement rejects disable and recovery races", func(t *testing.T) {
		for _, kind := range []string{"disable", "recovery", "rehash"} {
			t.Run(kind, func(t *testing.T) {
				p := f.subject(t, false)
				version := changeVersion(t, f, p.AuthGeneration)
				store := &passwordChangeStore{Store: f.identities[0]}
				var writes int
				store.replace = func(ctx context.Context, id string, want identity.Revision, epoch string, record password.Record) (identity.Snapshot, error) {
					writes++
					switch kind {
					case "disable":
						_, err := f.identities[1].DisableCAS(f.ctx, p.ID, p.Revision)
						must(t, err)
					case "recovery":
						f.open(t)
					default:
						fresh, err := f.hasher.Create([]byte("synthetic protocol password"))
						must(t, err)
						_, err = f.identities[1].RehashCAS(f.ctx, p.ID, want, fresh)
						must(t, err)
					}
					return store.Store.ReplacePasswordForSession(ctx, id, want, epoch, record)
				}
				state, err := changer(t, f, store).Change(t.Context(), p.ID, version, []byte("synthetic protocol password"), []byte("synthetic replacement"))
				if !errors.Is(err, identity.ErrConflict) || state != (identity.State{}) || writes != 1 {
					t.Fatal("final race retried or authorized a replacement")
				}
				row, err := f.identities[1].Get(f.ctx, p.ID)
				must(t, err)
				wantVersion := int64(1)
				if kind == "rehash" {
					wantVersion = 2
				}
				if row.CredentialVersion != wantVersion {
					t.Fatal("rejected race replaced password")
				}
			})
		}
	})
	t.Run("disabled matching current revision cannot be replaced", func(t *testing.T) {
		p := f.subject(t, false)
		version := changeVersion(t, f, p.AuthGeneration)
		revision, err := f.identities[1].DisableCAS(f.ctx, p.ID, p.Revision)
		must(t, err)
		record, err := f.hasher.Create([]byte("synthetic replacement"))
		must(t, err)
		_, err = f.identities[0].ReplacePasswordForSession(t.Context(), p.ID, revision, version.RecoveryID, record)
		wantError(t, err, identity.ErrConflict)
		row, err := f.identities[1].Get(f.ctx, p.ID)
		must(t, err)
		if row.Enabled || row.Revision != revision {
			t.Fatal("disabled current revision was replaced or enabled")
		}
	})
	t.Run("recovery waits for final account update and canceled wait releases locks", func(t *testing.T) { testPasswordChangeLocks(t, f) })
	t.Run("unknown committed receipt is not retried", func(t *testing.T) {
		p := f.subject(t, false)
		version := changeVersion(t, f, p.AuthGeneration)
		store := &passwordChangeStore{Store: f.identities[0]}
		var writes atomic.Int32
		lost := errors.Join(identity.ErrStore, context.DeadlineExceeded)
		store.replace = func(ctx context.Context, id string, want identity.Revision, epoch string, r password.Record) (identity.Snapshot, error) {
			writes.Add(1)
			if _, err := store.Store.ReplacePasswordForSession(ctx, id, want, epoch, r); err != nil {
				return identity.Snapshot{}, err
			}
			return identity.Snapshot{}, lost
		}
		state, err := changer(t, f, store).Change(t.Context(), p.ID, version, []byte("synthetic protocol password"), []byte("synthetic replacement"))
		if state != (identity.State{}) || err != lost || writes.Load() != 1 {
			t.Fatal("lost write receipt was retried or returned authority")
		}
		row, err := f.identities[1].Get(f.ctx, p.ID)
		must(t, err)
		match, err := f.hasher.Verify([]byte("synthetic replacement"), row.Record)
		must(t, err)
		if row.CredentialVersion != 2 || row.AuthGeneration != 2 || !match.Match {
			t.Fatal("fixture did not demonstrate committed replacement with lost receipt")
		}
	})
	t.Run("both revision overflows fail closed", func(t *testing.T) {
		for _, column := range []string{"credential_version", "auth_generation"} {
			p := f.subject(t, false)
			query := `UPDATE identity_credentials SET credential_version=$2 WHERE id=$1`
			if column == "auth_generation" {
				query = `UPDATE identity_credentials SET auth_generation=$2 WHERE id=$1`
			}
			_, err := f.pools[0].Exec(f.ctx, query, p.ID, int64(math.MaxInt64))
			must(t, err)
			row, err := f.identities[0].Get(f.ctx, p.ID)
			must(t, err)
			version := changeVersion(t, f, row.AuthGeneration)
			state, err := changer(t, f, f.identities[1]).Change(t.Context(), p.ID, version, []byte("synthetic protocol password"), []byte("synthetic replacement"))
			if state != (identity.State{}) || !errors.Is(err, identity.ErrConflict) {
				t.Fatal("overflow change authorized")
			}
			record, err := f.hasher.Create([]byte("synthetic replacement"))
			must(t, err)
			_, err = f.identities[1].ReplacePasswordForSession(t.Context(), p.ID, row.Revision, version.RecoveryID, record)
			wantError(t, err, identity.ErrConflict)
		}
	})
	t.Run("conditional replacement has primary key query plan", func(t *testing.T) { testPasswordChangePlan(t, f) })
}

func testPasswordChangeLocks(t *testing.T, f *fixture) {
	for _, cancelWait := range []bool{false, true} {
		name := "recovery serializes after commit"
		if cancelWait {
			name = "cancel releases final lock waiter"
		}
		t.Run(name, func(t *testing.T) {
			p := f.subject(t, false)
			version := changeVersion(t, f, p.AuthGeneration)
			held, err := f.pools[0].Begin(f.ctx)
			must(t, err)
			defer func() {
				if err := held.Rollback(f.ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Error("fixture lock rollback failed")
				}
			}()
			var blocker int32
			must(t, held.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&blocker))
			_, err = held.Exec(f.ctx, `SELECT id FROM identity_credentials WHERE id=$1 FOR UPDATE`, p.ID)
			must(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			type result struct {
				state identity.State
				err   error
			}
			changed := make(chan result, 1)
			c := changer(t, f, f.identities[1])
			go func() {
				state, err := c.Change(ctx, p.ID, version, []byte("synthetic protocol password"), []byte("synthetic replacement"))
				changed <- result{state, err}
			}()
			waiter := oneBackend(t, f.ctx, f.pools[0], blocker)
			if cancelWait {
				cancel()
				r := passwordChangeResult(t, changed)
				if r.state != (identity.State{}) || !errors.Is(r.err, identity.ErrStore) || !errors.Is(r.err, context.Canceled) {
					t.Fatal("canceled final write lost storage classification")
				}
				gone(t, f.ctx, f.pools[0], waiter)
				must(t, held.Rollback(f.ctx))
				row, err := f.identities[0].Get(f.ctx, p.ID)
				must(t, err)
				if row.CredentialVersion != 1 || row.AuthGeneration != 1 {
					t.Fatal("canceled lock waiter changed credential")
				}
				f.open(t)
				return
			}
			type recoveryResult struct {
				epoch string
				err   error
			}
			recovered := make(chan recoveryResult, 1)
			go func() { epoch, err := f.identities[0].BeginRecovery(f.ctx); recovered <- recoveryResult{epoch, err} }()
			oneBackend(t, f.ctx, f.pools[0], waiter.pid)
			select {
			case <-recovered:
				t.Fatal("recovery crossed final password replacement lock")
			default:
			}
			must(t, held.Rollback(f.ctx))
			r := passwordChangeResult(t, changed)
			if r.err != nil || r.state.CredentialVersion != 2 || r.state.AuthGeneration != 2 {
				t.Fatal("locked password replacement did not complete before recovery")
			}
			recovery := passwordChangeResult(t, recovered)
			must(t, recovery.err)
			must(t, f.identities[1].FinishRecovery(f.ctx, recovery.epoch))
		})
	}
}

func passwordChangeResult[T any](t *testing.T, out <-chan T) T {
	t.Helper()
	select {
	case result := <-out:
		return result
	case <-time.After(6 * time.Second):
		t.Fatal("password change operation did not finish")
	}
	var zero T
	return zero
}

func testPasswordChangePlan(t *testing.T, f *fixture) {
	p := f.subject(t, false)
	tx, err := f.pools[0].Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `INSERT INTO identity_credentials
        SELECT lpad(to_hex(n),32,'0'),true,1,1,format_version,algorithm,argon_version,memory_kib,iterations,parallelism,salt_bytes,digest_bytes,salt,digest
        FROM identity_credentials CROSS JOIN generate_series(1000000,1001999) n WHERE id=$1`, p.ID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `ANALYZE identity_credentials`)
	must(t, err)
	// Plan the actual owning query without executing it, using pgx's named-arg
	// rewrite. Never log the plan: expressions can contain verification material.
	source, err := os.ReadFile("../../db/queries/identity/credentials.sql")
	must(t, err)
	_, query, found := strings.Cut(string(source), "-- name: ReplacePasswordForSession :one\n")
	if !found {
		t.Fatal("password replacement query source unavailable")
	}
	query, _, _ = strings.Cut(query, "\n-- name:")
	record, err := f.hasher.Create([]byte("synthetic replacement"))
	must(t, err)
	args := pgx.NamedArgs{"id": p.ID, "expected_credential_version": p.CredentialVersion, "expected_auth_generation": p.AuthGeneration,
		"format_version": int32(record.FormatVersion), "algorithm": record.Algorithm, "argon_version": int32(record.ArgonVersion),
		"memory_kib": int64(record.Parameters.MemoryKiB), "iterations": int64(record.Parameters.Iterations), "parallelism": int16(record.Parameters.Parallelism),
		"salt_bytes": int32(record.Parameters.SaltBytes), "digest_bytes": int32(record.Parameters.DigestBytes), "salt": record.Salt, "digest": record.Digest}
	var plan []byte
	if err := tx.QueryRow(f.ctx, "EXPLAIN (FORMAT JSON) "+query, args).Scan(&plan); err != nil {
		t.Fatal("password replacement query planning failed")
	}
	if !strings.Contains(string(plan), "identity_credentials_pkey") || !strings.Contains(string(plan), "Index") {
		t.Fatal("password replacement point update lacked primary key plan")
	}
	t.Log("password replacement EXPLAIN: primary key index retained; no write executed")
}
