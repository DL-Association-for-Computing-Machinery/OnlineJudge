//go:build integration

package integration_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailbudget"
)

func TestMailSealBudgetAcrossIndependentPools(t *testing.T) {
	f := newBacklogFixture(t)
	// Separate database, not just a second pool pointed at the issuance schema.
	var dbname string
	must(t, f.a.QueryRow(f.ctx, "SELECT current_schema()").Scan(&dbname))
	name := pgx.Identifier{dbname}.Sanitize()
	_, err := f.a.Exec(f.ctx, "CREATE DATABASE "+name)
	must(t, err)
	t.Cleanup(func() {
		_, err := f.a.Exec(f.ctx, "DROP DATABASE "+name)
		must(t, err)
	})
	open := func() *pgxpool.Pool {
		t.Helper()
		cfg := f.a.Config()
		cfg.ConnConfig.Database = dbname
		delete(cfg.ConnConfig.RuntimeParams, "search_path")
		cfg.ConnConfig.RuntimeParams["synchronous_commit"] = "off"
		pool, err := pgxpool.NewWithConfig(f.ctx, cfg)
		must(t, err)
		t.Cleanup(pool.Close)
		return pool
	}
	first, second := open(), open()
	key := func(id string, firstByte byte) *emailtoken.Protector {
		t.Helper()
		p, err := emailtoken.NewProtector(id, map[string][32]byte{id: {firstByte}})
		must(t, err)
		return p
	}
	original, alias, fresh := key("original", 1), key("alias", 1), key("original", 2)
	a, err := mailbudget.New(first, original)
	must(t, err)
	b, err := mailbudget.New(second, alias)
	must(t, err)
	if a.Reserve(f.ctx, original) != mailbudget.ErrUnavailable {
		t.Fatal("missing ledger allowed a seal")
	}
	_, err = first.Exec(f.ctx, mailbudget.LedgerSQL)
	must(t, err)
	_, err = first.Exec(f.ctx, `CREATE FUNCTION public.check_budget_commit_mode() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
			IF current_setting('synchronous_commit') <> 'on' THEN
				RAISE EXCEPTION 'seal budget write is asynchronous';
			END IF;
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER check_budget_commit_mode BEFORE INSERT OR UPDATE ON public.oj_mail_seal_budget
			FOR EACH ROW EXECUTE FUNCTION public.check_budget_commit_mode();`)
	must(t, err)
	if a.Reserve(f.ctx, original) != mailbudget.ErrUnavailable {
		t.Fatal("missing budget allowed a seal")
	}
	must(t, mailbudget.Provision(f.ctx, first, original, 3))
	if mailbudget.Provision(f.ctx, second, alias, 10) != mailbudget.ErrUnavailable {
		t.Fatal("same key material was provisioned a second time under a new name")
	}

	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	epoch, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	must(t, issuer.FinishRecovery(f.ctx, epoch))
	mail, err := identity.ParseEmail("seal-budget@example.invalid")
	must(t, err)
	subject := identityID(1)
	_, err = issuer.InitializePersonal(f.ctx, subject, mail, true, identityRecord(t, h, "synthetic budget fixture password"))
	must(t, err)
	version := identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}
	for _, wrong := range []*emailtoken.Protector{fresh, alias} {
		id, err := issuer.IssueEmailToken(f.ctx, subject, version, emailtoken.Activation, wrong, a.Reserve,
			func(context.Context, pgx.Tx, string) error {
				t.Error("mismatched key reached enqueue")
				return nil
			})
		if id != "" || !errors.Is(err, mailbudget.ErrUnavailable) {
			t.Fatalf("mismatched key must reject issuance: %v", err)
		}
	}
	abort := errors.New("synthetic issuance abort")
	_, err = issuer.IssueEmailToken(f.ctx, subject, version,
		emailtoken.Activation, original, a.Reserve, func(context.Context, pgx.Tx, string) error { return abort })
	if !errors.Is(err, abort) {
		t.Fatalf("expected issuance rollback, got %v", err)
	}
	var tokens int
	must(t, f.a.QueryRow(f.ctx, "SELECT count(*) FROM identity_email_tokens").Scan(&tokens))
	if tokens != 0 {
		t.Fatal("failed issuance persisted a token")
	}
	var admitted atomic.Int64
	var workers sync.WaitGroup
	for i := range 10 {
		workers.Go(func() {
			budget, key := a, original
			if i%2 == 0 {
				budget, key = b, alias
			}
			if err := budget.Reserve(f.ctx, key); err == nil {
				admitted.Add(1)
			} else if err != mailbudget.ErrUnavailable {
				t.Errorf("unexpected reservation error: %v", err)
			}
		})
	}
	workers.Wait()
	if admitted.Load() != 2 || b.Reserve(f.ctx, alias) != mailbudget.ErrUnavailable {
		t.Fatal("shared ceiling or non-refund invariant failed")
	}
	must(t, mailbudget.Provision(f.ctx, second, fresh, 1))
	c, err := mailbudget.New(first, fresh)
	must(t, err)
	canceled, cancel := context.WithCancel(f.ctx)
	cancel()
	if c.Reserve(canceled, fresh) != mailbudget.ErrUnavailable {
		t.Fatal("canceled reservation allowed a seal")
	}
	must(t, c.Reserve(f.ctx, fresh))
	if c.Reserve(f.ctx, fresh) != mailbudget.ErrUnavailable {
		t.Fatal("fresh key did not enforce its own ceiling")
	}
}
