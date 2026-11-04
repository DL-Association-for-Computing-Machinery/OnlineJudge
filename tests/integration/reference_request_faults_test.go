//go:build integration

package integration_test

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func lostReferenceCommitStore(t *testing.T, f referenceRequestFixture) (*referencepg.Store, <-chan bool) {
	t.Helper()
	var drop atomic.Bool
	drop.Store(true)
	observed := make(chan bool, 1)
	cfg := f.b.Config()
	cfg.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &loseDraftCommitConn{Conn: conn, drop: &drop, observed: observed}, nil
	}
	pool, err := pgxpool.NewWithConfig(f.ctx, cfg)
	must(t, err)
	t.Cleanup(pool.Close)
	access, err := problempg.NewReferenceAccess(f.access)
	must(t, err)
	store, err := referencepg.New(pool, access)
	must(t, err)
	return store, observed
}

func TestReferenceRequestsUnknownCommit(t *testing.T) {
	for _, mode := range []string{"enqueue", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := newReferenceRequestFixture(t)
			f.limits(t, 1, 1)
			key := "reference-unknown-original-key"
			var first problemreference.Request
			var err error
			if mode == "cancel" {
				first, err = f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, f.catalog)
				must(t, err)
			}
			faulty, observed := lostReferenceCommitStore(t, f)
			var result problemreference.Request
			if mode == "enqueue" {
				result, err = faulty.Enqueue(f.ctx, f.actor, key, f.draft.ID, f.catalog)
			} else {
				result, err = faulty.Cancel(f.ctx, f.actor, key)
			}
			noReferenceResult(t, result, err, problemreference.ErrCommitUnknown)
			select {
			case committed := <-observed:
				if !committed {
					t.Fatal("did not observe the actual COMMIT reply")
				}
			case <-f.ctx.Done():
				t.Fatal("COMMIT loss fixture was not reached")
			}
			recovered, err := f.store.Receipt(f.ctx, f.actor, key)
			must(t, err)
			retired, err := problemreference.NewCatalog(nil)
			must(t, err)
			again, err := f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, retired)
			must(t, err)
			if !reflect.DeepEqual(again, recovered) {
				t.Fatal("same key did not recover the committed intent")
			}
			if mode == "enqueue" {
				f.counts(t, 1, 1)
			} else {
				if recovered.State != problemreference.Cancelled || recovered.Authorization != first.Authorization || recovered.ID != first.ID {
					t.Fatal("lost cancel receipt changed original authority")
				}
				f.counts(t, 1, 0)
			}
		})
	}
}

type heldReferenceAccess struct {
	referencepg.DraftAccess
	locked    chan uint32
	release   chan struct{}
	terminate func(context.Context, pgx.Tx) error
}

func (a heldReferenceAccess) LockAuthorSnapshot(ctx context.Context, tx pgx.Tx, actor identity.ProblemActor) (identity.ProblemAuthorSnapshot, error) {
	snapshot, err := a.DraftAccess.LockAuthorSnapshot(ctx, tx, actor)
	if err != nil {
		return identity.ProblemAuthorSnapshot{}, err
	}
	if a.terminate != nil {
		if err = a.terminate(ctx, tx); err != nil {
			return identity.ProblemAuthorSnapshot{}, err
		}
	} else {
		a.locked <- tx.Conn().PgConn().PID()
		select {
		case <-a.release:
		case <-ctx.Done():
			return identity.ProblemAuthorSnapshot{}, ctx.Err()
		}
	}
	return snapshot, nil
}

func TestReferenceRequestsAuthorityTransactions(t *testing.T) {
	for _, operation := range []string{"enqueue", "receipt", "cancel"} {
		for _, change := range []string{"role", "account", "recovery", "dead transaction"} {
			t.Run(operation+"/"+change, func(t *testing.T) {
				f := newReferenceRequestFixture(t)
				f.limits(t, 2, 2)
				key := "reference-authority-original"
				_, err := f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, f.catalog)
				must(t, err)
				base, err := problempg.NewReferenceAccess(f.access)
				must(t, err)
				held := heldReferenceAccess{DraftAccess: base, locked: make(chan uint32, 1), release: make(chan struct{})}
				if change == "dead transaction" {
					held.terminate = func(ctx context.Context, tx pgx.Tx) error {
						var terminated bool
						e := f.b.QueryRow(ctx, `SELECT pg_terminate_backend($1)`, tx.Conn().PgConn().PID()).Scan(&terminated)
						if e == nil && !terminated {
							return errors.New("owned test backend was not terminated")
						}
						return e
					}
				}
				store, err := referencepg.New(f.a, held)
				must(t, err)
				call := func() (problemreference.Request, error) {
					switch operation {
					case "enqueue":
						return store.Enqueue(f.ctx, f.actor, "reference-authority-second", f.draft.ID, f.catalog)
					case "receipt":
						return store.Receipt(f.ctx, f.actor, key)
					default:
						return store.Cancel(f.ctx, f.actor, key)
					}
				}
				if change == "dead transaction" {
					r, e := call()
					noReferenceResult(t, r, e, problemreference.ErrStore)
					f.counts(t, 1, 1)
					return
				}
				done := make(chan error, 1)
				go func() { _, e := call(); done <- e }()
				var pid uint32
				select {
				case pid = <-held.locked:
				case e := <-done:
					t.Fatalf("request ended before lock: %v", e)
				case <-f.ctx.Done():
					t.Fatal("authority fixture timed out")
				}
				revoked := make(chan error, 1)
				go func() {
					var e error
					switch change {
					case "role":
						_, e = f.access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: f.actor.SubjectID, ExpectedRevision: 1, Enabled: false, Operator: "reference-fixture", Reason: "transactional revocation"})
					case "account":
						_, e = f.identities.DisableCAS(f.ctx, f.actor.SubjectID, identity.Revision{CredentialVersion: 1, AuthGeneration: 1})
					case "recovery":
						_, e = f.identities.BeginRecovery(f.ctx)
					}
					revoked <- e
				}()
				waitBacklogBlocked(t, f.backlogFixture, pid, f.appA)
				close(held.release)
				must(t, <-done)
				must(t, <-revoked)
				r, e := f.store.Receipt(f.ctx, f.actor, key)
				want := identity.ErrForbidden
				if change == "account" {
					want = identity.ErrConflict
				} else if change == "recovery" {
					want = identity.ErrRecovering
				}
				noReferenceResult(t, r, e, want)
				total, queued := 1, 1
				if operation == "enqueue" {
					total, queued = 2, 2
				} else if operation == "cancel" {
					queued = 0
				}
				f.counts(t, total, queued)
			})
		}
	}
}

func TestReferenceRequestCancelledPolicyWait(t *testing.T) {
	f := newReferenceRequestFixture(t)
	f.limits(t, 1, 1)
	holder, err := f.b.Begin(f.ctx)
	must(t, err)
	defer holder.Rollback(f.ctx)
	var pid uint32
	must(t, holder.QueryRow(f.ctx, `SELECT pg_backend_pid() FROM problem_reference_request_policy WHERE singleton FOR UPDATE`).Scan(&pid))
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	type outcome struct {
		r   problemreference.Request
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		r, e := f.store.Enqueue(ctx, f.actor, "reference-cancelled-policy", f.draft.ID, f.catalog)
		done <- outcome{r, e}
	}()
	waitBacklogBlocked(t, f.backlogFixture, pid, f.appA)
	cancel()
	select {
	case r := <-done:
		noReferenceResult(t, r.r, r.err, context.Canceled)
	case <-f.ctx.Done():
		t.Fatal("cancelled admission never finished")
	}
	must(t, holder.Rollback(f.ctx))
	f.counts(t, 0, 0)
}
