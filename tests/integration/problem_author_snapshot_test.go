//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func readAuthorSnapshot(t *testing.T, f draftFixture, actor identity.ProblemActor) (identity.ProblemAuthorSnapshot, error) {
	t.Helper()
	tx, err := f.a.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	must(t, err)
	defer tx.Rollback(f.ctx)
	snapshot, err := f.access.LockAuthorSnapshot(f.ctx, tx, actor)
	if err == nil {
		must(t, tx.Commit(f.ctx))
	}
	return snapshot, err
}

func requireNoAuthorSnapshot(t *testing.T, snapshot identity.ProblemAuthorSnapshot, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || snapshot != (identity.ProblemAuthorSnapshot{}) {
		t.Fatalf("failed author check returned authority: snapshot=%+v err=%v want=%v", snapshot, err, want)
	}
}

func TestProblemAuthorSnapshotRevision(t *testing.T) {
	f := newDraftFixture(t)
	snapshot, err := readAuthorSnapshot(t, f, f.actor)
	requireNoAuthorSnapshot(t, snapshot, err, identity.ErrForbidden)
	f.role(t, f.actor.SubjectID, 0, true)
	first, err := readAuthorSnapshot(t, f, f.actor)
	must(t, err)
	if first != (identity.ProblemAuthorSnapshot{Actor: f.actor, RoleRevision: 1}) {
		t.Fatal("snapshot did not bind the current actor and role revision")
	}
	f.role(t, f.actor.SubjectID, 1, false)
	snapshot, err = readAuthorSnapshot(t, f, f.actor)
	requireNoAuthorSnapshot(t, snapshot, err, identity.ErrForbidden)
	f.role(t, f.actor.SubjectID, 2, true)
	regranted, err := readAuthorSnapshot(t, f, f.actor)
	must(t, err)
	if regranted.Actor != first.Actor || regranted.RoleRevision != 3 || regranted == first {
		t.Fatal("regrant revived the old authorization snapshot")
	}
	// Legacy callers retain the same authorization path without needing a snapshot.
	must(t, f.repository.CheckAuthor(f.ctx, f.actor))
	f.count(t, 0, 0)
}

func TestProblemAuthorSnapshotIdentityGenerations(t *testing.T) {
	t.Run("credential-only rehash then replacement", func(t *testing.T) {
		f := newDraftFixture(t)
		f.role(t, f.actor.SubjectID, 0, true)
		first, err := readAuthorSnapshot(t, f, f.actor)
		must(t, err)
		row, err := f.identities.Get(f.ctx, f.actor.SubjectID)
		must(t, err)
		h := identityHasher(t, false)
		rehashed, err := f.identities.RehashCAS(f.ctx, row.ID, row.Revision, identityRecord(t, h, "synthetic author password"))
		must(t, err)
		if rehashed.CredentialVersion != 2 || rehashed.AuthGeneration != 1 {
			t.Fatal("fixture did not perform a credential-only rehash")
		}
		afterRehash, err := readAuthorSnapshot(t, f, f.actor)
		must(t, err)
		if afterRehash != first {
			t.Fatal("credential-only rehash changed existing-session authority")
		}
		replaced, err := f.identities.ReplacePasswordCAS(f.ctx, row.ID, rehashed.Revision, identityRecord(t, h, "synthetic replacement password"))
		must(t, err)
		snapshot, err := readAuthorSnapshot(t, f, f.actor)
		requireNoAuthorSnapshot(t, snapshot, err, identity.ErrConflict)
		current := f.actor
		current.AuthGeneration = replaced.AuthGeneration
		snapshot, err = readAuthorSnapshot(t, f, current)
		must(t, err)
		if snapshot.Actor != current || snapshot.RoleRevision != first.RoleRevision || snapshot == first {
			t.Fatal("password replacement did not change the complete snapshot")
		}
	})
	t.Run("recovery closed then reopened", func(t *testing.T) {
		f := newDraftFixture(t)
		f.role(t, f.actor.SubjectID, 0, true)
		first, err := readAuthorSnapshot(t, f, f.actor)
		must(t, err)
		epoch, err := f.identities.BeginRecovery(f.ctx)
		must(t, err)
		snapshot, err := readAuthorSnapshot(t, f, f.actor)
		requireNoAuthorSnapshot(t, snapshot, err, identity.ErrRecovering)
		must(t, f.identities.FinishRecovery(f.ctx, epoch))
		snapshot, err = readAuthorSnapshot(t, f, f.actor)
		requireNoAuthorSnapshot(t, snapshot, err, identity.ErrConflict)
		current := f.actor
		current.RecoveryID = epoch
		snapshot, err = readAuthorSnapshot(t, f, current)
		must(t, err)
		if snapshot.Actor != current || snapshot.RoleRevision != first.RoleRevision || snapshot == first {
			t.Fatal("recovery did not change the complete snapshot")
		}
	})
}

func TestProblemAuthorSnapshotFailureIsZero(t *testing.T) {
	for _, scenario := range []string{"nil receiver", "nil context", "nil transaction", "invalid actor", "cancelled", "closed transaction", "failed transaction", "unknown account", "disabled account", "wrong generation", "wrong recovery"} {
		t.Run(scenario, func(t *testing.T) {
			f := newDraftFixture(t)
			f.role(t, f.actor.SubjectID, 0, true)
			tx, err := f.a.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
			must(t, err)
			defer tx.Rollback(f.ctx)
			access, call, actor := f.access, f.ctx, f.actor
			want := identity.ErrInvalid
			switch scenario {
			case "nil receiver":
				access = (*identitypg.ProblemAccess)(nil)
			case "nil context":
				call = nil
			case "nil transaction":
				tx = nil
			case "invalid actor":
				actor = identity.ProblemActor{}
			case "cancelled":
				var cancel context.CancelFunc
				call, cancel = context.WithCancel(f.ctx)
				cancel()
				want = context.Canceled
			case "closed transaction":
				must(t, tx.Rollback(f.ctx))
				want = identity.ErrStore
			case "failed transaction":
				_, err = tx.Exec(f.ctx, `SELECT 1/0`)
				if err == nil {
					t.Fatal("transaction failure fixture did not fail")
				}
				want = identity.ErrStore
			case "unknown account":
				actor.SubjectID = identityID(9999)
				want = identity.ErrConflict
			case "disabled account":
				_, err = f.identities.DisableCAS(f.ctx, actor.SubjectID, identity.Revision{CredentialVersion: 1, AuthGeneration: 1})
				must(t, err)
				want = identity.ErrConflict
			case "wrong generation":
				actor.AuthGeneration++
				want = identity.ErrConflict
			case "wrong recovery":
				actor.RecoveryID = identityID(9999)
				want = identity.ErrConflict
			}
			snapshot, err := access.LockAuthorSnapshot(call, tx, actor)
			requireNoAuthorSnapshot(t, snapshot, err, want)
		})
	}
}

func TestProblemAuthorSnapshotLocksUntilTransactionEnd(t *testing.T) {
	for _, end := range []string{"commit", "rollback"} {
		t.Run(end, func(t *testing.T) {
			f := newDraftFixture(t)
			f.role(t, f.actor.SubjectID, 0, true)
			tx, err := f.a.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
			must(t, err)
			defer tx.Rollback(f.ctx)
			_, err = f.access.LockAuthorSnapshot(f.ctx, tx, f.actor)
			must(t, err)
			// Probe each row independently: SetAuthorRole alone would stop at the
			// account lock and could not prove that the role row is also locked.
			queries := []string{
				`SELECT generation FROM identity_session_recovery WHERE singleton FOR UPDATE NOWAIT`,
				`SELECT id FROM identity_credentials WHERE id=$1 FOR UPDATE NOWAIT`,
				`SELECT subject_id FROM identity_problem_authors WHERE subject_id=$1 FOR UPDATE NOWAIT`,
			}
			probe := func(locked bool) {
				for i, query := range queries {
					other, e := f.b.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
					must(t, e)
					var value string
					if i == 0 {
						e = other.QueryRow(f.ctx, query).Scan(&value)
					} else {
						e = other.QueryRow(f.ctx, query, f.actor.SubjectID).Scan(&value)
					}
					must(t, other.Rollback(f.ctx))
					if !locked {
						must(t, e)
						continue
					}
					var pgerr *pgconn.PgError
					if !errors.As(e, &pgerr) || pgerr.Code != "55P03" {
						t.Fatalf("snapshot lock %d not held by caller: %v", i, e)
					}
				}
			}
			probe(true)
			if end == "commit" {
				must(t, tx.Commit(f.ctx))
			} else {
				must(t, tx.Rollback(f.ctx))
			}
			probe(false)
		})
	}
}

func TestProblemAuthorSnapshotWaiterSeesRevocation(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	holder, err := f.b.Begin(f.ctx)
	must(t, err)
	defer holder.Rollback(f.ctx)
	var pid uint32
	must(t, holder.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	// Controlled row mutation isolates the role wait from SetAuthorRole's earlier
	// account lock. The production role-change path is covered by draft tests.
	_, err = holder.Exec(f.ctx, `UPDATE identity_problem_authors SET enabled=false,revision=2 WHERE subject_id=$1`, f.actor.SubjectID)
	must(t, err)
	waiter, err := f.a.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	must(t, err)
	defer waiter.Rollback(f.ctx)
	type outcome struct {
		snapshot identity.ProblemAuthorSnapshot
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		snapshot, e := f.access.LockAuthorSnapshot(f.ctx, waiter, f.actor)
		done <- outcome{snapshot, e}
	}()
	waitBacklogBlocked(t, f.backlogFixture, pid, f.appA)
	must(t, holder.Commit(f.ctx))
	select {
	case result := <-done:
		requireNoAuthorSnapshot(t, result.snapshot, result.err, identity.ErrForbidden)
	case <-f.ctx.Done():
		t.Fatal("snapshot waiter did not finish after revocation committed")
	}
}
