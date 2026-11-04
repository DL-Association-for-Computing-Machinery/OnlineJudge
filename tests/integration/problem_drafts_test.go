//go:build integration

package integration_test

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
	"github.com/jackc/pgx/v5"
)

type draftFixture struct {
	backlogFixture
	identities *identitypg.Store
	access     *identitypg.ProblemAccess
	repository *problempg.Store
	actor      identity.ProblemActor
	prepared   *problem.Prepared
	bundle     *problemfixture.Bundle
}

func newDraftFixture(t *testing.T) draftFixture {
	t.Helper()
	f := draftFixture{backlogFixture: newBacklogFixture(t)}
	h := identityHasher(t, false)
	var err error
	f.identities, err = identitypg.New(f.a, h)
	must(t, err)
	_, err = f.identities.Initialize(f.ctx, identityID(1100), true, identityRecord(t, h, "synthetic author password"))
	must(t, err)
	_, err = f.identities.Initialize(f.ctx, identityID(1101), true, identityRecord(t, h, "synthetic other password"))
	must(t, err)
	epoch, err := f.identities.BeginRecovery(f.ctx)
	must(t, err)
	must(t, f.identities.FinishRecovery(f.ctx, epoch))
	f.actor = identity.ProblemActor{SubjectID: identityID(1100), SessionVersion: identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}}
	f.access, err = identitypg.NewProblemAccess(f.a)
	must(t, err)
	f.repository, err = problempg.New(f.a, f.access)
	must(t, err)
	f.bundle = problemfixture.New(t)
	f.prepared, err = problem.LoadBundle(f.bundle.Directory, "library-checker:aplusb", f.bundle.Lock)
	must(t, err)
	t.Cleanup(func() { must(t, f.prepared.Close()) })
	return f
}
func (f draftFixture) role(t *testing.T, id string, revision int64, enabled bool) identity.ProblemAuthorRole {
	t.Helper()
	r, err := f.access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: id, ExpectedRevision: revision, Enabled: enabled, Operator: "synthetic-maintenance", Reason: "controlled private draft integration"})
	must(t, err)
	return r
}
func (f draftFixture) count(t *testing.T, drafts, refs int) {
	t.Helper()
	var a, b int
	must(t, f.a.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM problem_drafts),(SELECT count(*) FROM problem_draft_materials)`).Scan(&a, &b))
	if a != drafts || b != refs {
		t.Fatalf("partial metadata: drafts=%d refs=%d", a, b)
	}
}

func TestPrivateDraftRolesAndOwner(t *testing.T) {
	f := newDraftFixture(t)
	if err := f.repository.CheckAuthor(f.ctx, f.actor); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("ordinary enabled account became an author")
	}
	_, err := f.repository.Create(f.ctx, f.actor, "private-request-integration-01", f.prepared)
	if !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("ungranted create accepted")
	}
	f.count(t, 0, 0)
	role := f.role(t, f.actor.SubjectID, 0, true)
	if role.Revision != 1 {
		t.Fatal("wrong initial CAS revision")
	}
	_, err = f.access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: f.actor.SubjectID, ExpectedRevision: 0, Enabled: false, Operator: "fixture", Reason: "stale CAS"})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatal("stale role CAS accepted")
	}
	draft, err := f.repository.Create(f.ctx, f.actor, "private-request-integration-01", f.prepared)
	must(t, err)
	f.count(t, 1, len(f.prepared.Content().References))
	other := f.actor
	other.SubjectID = identityID(1101)
	f.role(t, other.SubjectID, 0, true)
	for _, id := range []string{draft.ID, identityID(9999)} {
		_, err = f.repository.GetOwn(f.ctx, other, id)
		if !errors.Is(err, problem.ErrNotFound) {
			t.Fatal("foreign/unknown existence exposed")
		}
	}
	read, err := f.repository.GetOwn(f.ctx, f.actor, draft.ID)
	must(t, err)
	if !reflect.DeepEqual(read, draft) {
		t.Fatal("reopened owner view changed")
	}
	f.role(t, f.actor.SubjectID, 1, false)
	for _, operation := range []func() error{func() error { _, e := f.repository.GetOwn(f.ctx, f.actor, draft.ID); return e }, func() error {
		_, e := f.repository.Create(f.ctx, f.actor, "private-request-integration-01", f.prepared)
		return e
	}} {
		if !errors.Is(operation(), identity.ErrForbidden) {
			t.Fatal("owner/retry bypassed revoked role")
		}
	}
	var audit int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM identity_problem_author_audit WHERE subject_id=$1`, f.actor.SubjectID).Scan(&audit))
	if audit != 2 {
		t.Fatal("role CAS/audit not atomic")
	}
	f.role(t, f.actor.SubjectID, 2, true)
	read, err = f.repository.GetOwn(f.ctx, f.actor, draft.ID)
	must(t, err)
	if read.ID != draft.ID {
		t.Fatal("regrant changed immutable draft")
	}
	_, err = f.provider.DownTo(f.ctx, 7)
	must(t, err)
	if _, err = f.provider.Down(f.ctx); err == nil {
		t.Fatal("migration discarded private author/draft state")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
}

func TestPrivateDraftIdempotencyAndRollback(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	type outcome struct {
		d   problem.Draft
		err error
	}
	out := make(chan outcome, 8)
	start := make(chan struct{})
	for range 8 {
		go func() {
			<-start
			d, e := f.repository.Create(f.ctx, f.actor, "same-original-request-01", f.prepared)
			out <- outcome{d, e}
		}()
	}
	close(start)
	var first problem.Draft
	for range 8 {
		r := <-out
		must(t, r.err)
		if first.ID == "" {
			first = r.d
		} else if !reflect.DeepEqual(first, r.d) {
			t.Fatal("same-key parallel request changed receipt")
		}
	}
	f.count(t, 1, len(f.prepared.Content().References))
	// Deliberately discard the successful receipt and recover via a new adapter.
	reopened, err := problempg.New(f.b, f.access)
	must(t, err)
	again, err := reopened.Create(f.ctx, f.actor, "same-original-request-01", f.prepared)
	must(t, err)
	if !reflect.DeepEqual(first, again) {
		t.Fatal("original key failed to recover committed receipt")
	}
	changed := problemfixture.New(t)
	changed.Document["provenance"].(map[string]any)["original_title"] = "changed private metadata"
	changed.Write(t)
	p, err := problem.LoadBundle(changed.Directory, "library-checker:aplusb", changed.Lock)
	must(t, err)
	defer func() { must(t, p.Close()) }()
	_, err = f.repository.Create(f.ctx, f.actor, "same-original-request-01", p)
	if !errors.Is(err, problem.ErrConflict) {
		t.Fatal("different content under same key accepted")
	}
	_, err = f.a.Exec(f.ctx, `CREATE FUNCTION reject_draft_ref() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.logical_name='statement' THEN RAISE EXCEPTION 'synthetic reference failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_draft_ref BEFORE INSERT ON problem_draft_materials FOR EACH ROW EXECUTE FUNCTION reject_draft_ref()`)
	must(t, err)
	_, err = f.repository.Create(f.ctx, f.actor, "rollback-whole-reference-set", f.prepared)
	if !errors.Is(err, problem.ErrStore) {
		t.Fatal("reference failure was ignored")
	}
	f.count(t, 1, len(f.prepared.Content().References))
}

type fixedDraftSession struct{ view redisstore.View }

func (s fixedDraftSession) Read(context.Context, redisstore.Credential) (redisstore.View, error) {
	return s.view, nil
}
func TestPrivateDraftMaterialBoundary(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	root := filepath.Join(t.TempDir(), "private-objects")
	must(t, os.Mkdir(root, 0700))
	materials, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 2})
	must(t, err)
	defer func() { must(t, materials.Close()) }()
	service, err := problem.New(fixedDraftSession{redisstore.View{SubjectID: f.actor.SubjectID, AuthGeneration: 1, RecoveryID: f.actor.RecoveryID}}, f.repository, materials)
	must(t, err)
	c, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	must(t, err)
	target := filepath.Join(f.bundle.Directory, "statements/aplusb.md")
	raw := f.bundle.Files["statements/aplusb.md"]
	bad := append([]byte(nil), raw...)
	bad[0] = 'X'
	must(t, os.WriteFile(target, bad, 0600))
	_, err = service.Create(f.ctx, c, "corrupt-material-request-01", f.prepared)
	if !errors.Is(err, materialstore.ErrIntegrity) {
		t.Fatal("corrupt file admitted")
	}
	f.count(t, 0, 0)
	must(t, os.WriteFile(target, raw, 0600))
	created, err := service.Create(f.ctx, c, "corrupt-material-request-01", f.prepared)
	must(t, err)
	f.count(t, 1, len(f.prepared.Content().References))
	for _, r := range created.References {
		snapshot, e := materials.Open(f.ctx, r.Object)
		must(t, e)
		must(t, snapshot.Close())
	}
	reopened, err := problempg.New(f.b, f.access)
	must(t, err)
	got, err := reopened.GetOwn(f.ctx, f.actor, created.ID)
	must(t, err)
	if !reflect.DeepEqual(got, created) {
		t.Fatal("new connection lost immutable references")
	}
	t.Log("real private material Put/Open plus owner-scoped PG draft committed; session observation stubbed in this PG test")
}

type heldDraftAccess struct {
	access  *identitypg.ProblemAccess
	locked  chan uint32
	release chan struct{}
	lose    bool
	fixture draftFixture
}

func (a heldDraftAccess) LockAuthor(ctx context.Context, tx pgx.Tx, actor identity.ProblemActor) error {
	if err := a.access.LockAuthor(ctx, tx, actor); err != nil {
		return err
	}
	if a.lose {
		var terminated bool
		return a.fixture.b.QueryRow(ctx, `SELECT pg_terminate_backend($1)`, tx.Conn().PgConn().PID()).Scan(&terminated)
	}
	a.locked <- tx.Conn().PgConn().PID()
	select {
	case <-a.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func TestPrivateDraftRevocationLocks(t *testing.T) {
	for _, mode := range []string{"role", "account", "recovery", "lost-transaction"} {
		t.Run(mode, func(t *testing.T) {
			f := newDraftFixture(t)
			f.role(t, f.actor.SubjectID, 0, true)
			held := heldDraftAccess{access: f.access, locked: make(chan uint32, 1), release: make(chan struct{}), lose: mode == "lost-transaction", fixture: f}
			repository, err := problempg.New(f.a, held)
			must(t, err)
			if held.lose {
				_, err = repository.Create(f.ctx, f.actor, "lost-author-transaction-01", f.prepared)
				if err == nil {
					t.Fatal("business committed on separate live transaction")
				}
				f.count(t, 0, 0)
				return
			}
			created := make(chan error, 1)
			go func() {
				_, e := repository.Create(f.ctx, f.actor, "author-lock-ordered-request", f.prepared)
				created <- e
			}()
			var pid uint32
			select {
			case pid = <-held.locked:
			case e := <-created:
				t.Fatalf("create failed before identity locks: %v", e)
			case <-f.ctx.Done():
				t.Fatal("identity lock fixture timed out")
			}
			revoked := make(chan error, 1)
			go func() {
				var e error
				switch mode {
				case "role":
					_, e = f.access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: f.actor.SubjectID, ExpectedRevision: 1, Enabled: false, Operator: "fixture", Reason: "concurrent revocation"})
				case "account":
					_, e = f.identities.DisableCAS(f.ctx, f.actor.SubjectID, identity.Revision{CredentialVersion: 1, AuthGeneration: 1})
				case "recovery":
					_, e = f.identities.BeginRecovery(f.ctx)
				}
				revoked <- e
			}()
			waitBacklogBlocked(t, f.backlogFixture, pid, f.appA)
			close(held.release)
			must(t, <-created)
			must(t, <-revoked)
			_, err = f.repository.Create(f.ctx, f.actor, "after-current-revocation-01", f.prepared)
			if err == nil {
				t.Fatal("stale authority admitted another draft")
			}
			f.count(t, 1, len(f.prepared.Content().References))
		})
	}
}
