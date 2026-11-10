//go:build integration && linux

package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
	"github.com/jackc/pgx/v5"
)

const expectedPlanKey = "expected-reference-original"

type expectedPlanFixture struct {
	referenceRequestFixture
	materials  *materialstore.Local
	credential redisstore.Credential
	session    fixedDraftSession
	request    problemreference.Request
	want       problemreference.ExpectedPlan
}

func newExpectedPlanFixture(t *testing.T) expectedPlanFixture {
	t.Helper()
	f := expectedPlanFixture{referenceRequestFixture: referenceRequestFixture{draftFixture: newDraftFixture(t)}}
	f.role(t, f.actor.SubjectID, 0, true)
	b := f.bundle
	b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("synthetic expected plan source\n"))
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	must(t, err)
	t.Cleanup(func() { must(t, p.Close()) })
	plan, err := p.PrepareReference(f.ctx, problem.SmallReferenceLimits())
	must(t, err)
	raw, err := json.Marshal(plan)
	must(t, err)
	f.want = problemreference.ExpectedPlan{Encoding: raw, SHA256: plan.Digest()}
	root := filepath.Join(t.TempDir(), "objects")
	must(t, os.Mkdir(root, 0700))
	f.materials, err = materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	must(t, err)
	t.Cleanup(func() { must(t, f.materials.Close()) })
	f.credential, err = redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	must(t, err)
	f.session = fixedDraftSession{view: redisstore.View{SubjectID: f.actor.SubjectID, AuthGeneration: f.actor.AuthGeneration, RecoveryID: f.actor.RecoveryID}}
	drafts, err := problem.New(f.session, f.repository, f.materials)
	must(t, err)
	f.draft, err = drafts.Create(f.ctx, f.credential, "expected-plan-original-draft", p)
	must(t, err)
	access, err := problempg.NewReferenceAccess(f.access)
	must(t, err)
	f.store, err = referencepg.New(f.a, access)
	must(t, err)
	f.limits(t, 1, 1)
	f.catalog, err = problemreference.NewCatalog(map[string]*problem.Prepared{f.draft.SourceID: p})
	must(t, err)
	f.request, err = f.store.Enqueue(f.ctx, f.actor, expectedPlanKey, f.draft.ID, f.catalog)
	must(t, err)
	must(t, p.Close())
	must(t, f.prepared.Close())
	must(t, os.RemoveAll(b.Directory))
	return f
}

func (f expectedPlanFixture) service(t *testing.T, sessions problemreference.SessionReader, repo problemreference.PlanRepository) *problemreference.PlanService {
	t.Helper()
	s, err := problemreference.NewPlanService(sessions, repo, f.repository, f.materials)
	must(t, err)
	return s
}

func (f expectedPlanFixture) planCount(t *testing.T, want int) {
	t.Helper()
	var count int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_reference_expected_plans`).Scan(&count))
	if count != want {
		t.Fatalf("expected %d plan rows, got %d", want, count)
	}
}

func noExpectedPlan(t *testing.T, p problemreference.ExpectedPlan, err, want error) {
	t.Helper()
	if p.Encoding != nil || p.SHA256 != "" || !errors.Is(err, want) {
		t.Fatalf("expected no plan and %v, got %v", want, err)
	}
}

func TestExpectedReferencePlanLifecycleAndSchema(t *testing.T) {
	f := newExpectedPlanFixture(t)
	// Schema 9 already has configured admission and an intent. No plan yet:
	// Down10 and re-upgrade must preserve that entire ledger.
	_, err := f.provider.DownTo(f.ctx, 9)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("startup accepted schema 9")
	}
	r, err := f.store.Receipt(f.ctx, f.actor, expectedPlanKey)
	must(t, err)
	if !reflect.DeepEqual(r, f.request) {
		t.Fatal("Down10 changed the request")
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	s := f.service(t, f.session, f.store)
	missing, err := s.Receipt(f.ctx, f.credential, expectedPlanKey)
	noExpectedPlan(t, missing, err, problemreference.ErrNotFound)
	plan, err := s.Prepare(f.ctx, f.credential, expectedPlanKey)
	must(t, err)
	if !reflect.DeepEqual(plan, f.want) || bytes.Contains(plan.Encoding, []byte("synthetic expected plan source")) {
		t.Fatal("plan did not preserve the original identity-only bytes")
	}
	plan.Encoding[0] = '!'
	again, err := s.Prepare(f.ctx, f.credential, expectedPlanKey)
	must(t, err)
	if !reflect.DeepEqual(again, f.want) {
		t.Fatal("same-key prepare changed stored bytes")
	}
	f.planCount(t, 1)
	f.counts(t, 1, 1)
	draft, err := f.repository.GetOwn(f.ctx, f.actor, f.draft.ID)
	must(t, err)
	if !reflect.DeepEqual(draft, f.draft) {
		t.Fatal("preparation changed the draft")
	}
	other := f.actor
	other.SubjectID = identityID(1101)
	f.role(t, other.SubjectID, 0, true)
	missing, err = f.store.ExpectedPlan(f.ctx, other, expectedPlanKey)
	noExpectedPlan(t, missing, err, problemreference.ErrNotFound)
	_, err = f.store.Cancel(f.ctx, f.actor, expectedPlanKey)
	must(t, err)
	f.role(t, f.actor.SubjectID, 1, false)
	missing, err = s.Receipt(f.ctx, f.credential, expectedPlanKey)
	noExpectedPlan(t, missing, err, identity.ErrForbidden)
	f.role(t, f.actor.SubjectID, 2, true)
	history, err := s.Receipt(f.ctx, f.credential, expectedPlanKey)
	must(t, err)
	if !reflect.DeepEqual(history, f.want) {
		t.Fatal("cancel or regrant changed history")
	}
	missing, err = s.Prepare(f.ctx, f.credential, expectedPlanKey)
	noExpectedPlan(t, missing, err, problemreference.ErrNotQueued)
	if _, err := f.provider.DownTo(f.ctx, 9); err == nil || !strings.Contains(err.Error(), "cannot discard expected reference plans") {
		t.Fatalf("Down10 did not protect plan history: %v", err)
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
	f.planCount(t, 1)
	f.counts(t, 1, 0)
}

type expectedSessionFunc func(context.Context, redisstore.Credential) (redisstore.View, error)

func (fn expectedSessionFunc) Read(ctx context.Context, c redisstore.Credential) (redisstore.View, error) {
	return fn(ctx, c)
}

func TestExpectedPlanRechecksAfterFreeze(t *testing.T) {
	for _, tc := range []struct {
		name string
		want error
	}{
		{"cancelled", problemreference.ErrNotQueued},
		{"revoked", identity.ErrForbidden},
		{"regranted", problemreference.ErrAuthorityChanged},
		{"generation", problemreference.ErrAuthorityChanged},
		{"recovery", problemreference.ErrAuthorityChanged},
		{"database account", identity.ErrConflict},
		{"database recovery", identity.ErrRecovering},
		{"logout", redisstore.ErrStale},
		{"changed request profile", problemreference.ErrSourceChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newExpectedPlanFixture(t)
			var reads int
			session := expectedSessionFunc(func(_ context.Context, c redisstore.Credential) (redisstore.View, error) {
				if !reflect.DeepEqual(c, f.credential) {
					t.Fatal("preparation changed credential")
				}
				reads++
				view := f.session.view
				if reads == 2 {
					switch tc.name {
					case "cancelled":
						_, err := f.store.Cancel(f.ctx, f.actor, expectedPlanKey)
						must(t, err)
					case "revoked", "regranted":
						f.role(t, f.actor.SubjectID, 1, false)
						if tc.name == "regranted" {
							f.role(t, f.actor.SubjectID, 2, true)
						}
					case "generation":
						view.AuthGeneration++
					case "recovery":
						view.RecoveryID = identityID(123)
					case "database account":
						_, err := f.identities.DisableCAS(f.ctx, f.actor.SubjectID, identity.Revision{CredentialVersion: 1, AuthGeneration: 1})
						must(t, err)
					case "database recovery":
						_, err := f.identities.BeginRecovery(f.ctx)
						must(t, err)
					case "logout":
						return redisstore.View{}, redisstore.ErrStale
					case "changed request profile":
						raw := []byte(`{"id":"changed"}`)
						_, err := f.a.Exec(f.ctx, `UPDATE problem_reference_requests SET profile_encoding=$1,profile_sha256=$2 WHERE id=$3`,
							raw, problemfixture.Hash(raw), f.request.ID)
						must(t, err)
					}
				}
				return view, nil
			})
			plan, err := f.service(t, session, f.store).Prepare(f.ctx, f.credential, expectedPlanKey)
			noExpectedPlan(t, plan, err, tc.want)
			if reads != 2 {
				t.Fatal("did not reach the post-freeze observation")
			}
			f.planCount(t, 0)
		})
	}
}

type expectedRepoHook struct {
	problemreference.PlanRepository
	persist func(context.Context, identity.ProblemActor, problemreference.PreparedPlan) (problemreference.ExpectedPlan, error)
}

func (r expectedRepoHook) PersistExpectedPlan(ctx context.Context, actor identity.ProblemActor, p problemreference.PreparedPlan) (problemreference.ExpectedPlan, error) {
	return r.persist(ctx, actor, p)
}

func TestExpectedPlanUnknownCommitRecoversOriginalKey(t *testing.T) {
	f := newExpectedPlanFixture(t)
	faulty, observed := lostReferenceCommitStore(t, f.referenceRequestFixture)
	s := f.service(t, f.session, expectedRepoHook{PlanRepository: f.store, persist: faulty.PersistExpectedPlan})
	plan, err := s.Prepare(f.ctx, f.credential, expectedPlanKey)
	noExpectedPlan(t, plan, err, problemreference.ErrCommitUnknown)
	select {
	case committed := <-observed:
		if !committed {
			t.Fatal("did not lose an actual COMMIT reply")
		}
	case <-f.ctx.Done():
		t.Fatal("commit-loss fixture was not reached")
	}
	recovered, err := s.Receipt(f.ctx, f.credential, expectedPlanKey)
	must(t, err)
	if !reflect.DeepEqual(recovered, f.want) {
		t.Fatal("original key did not recover the committed plan")
	}
	f.planCount(t, 1)
}

func TestConcurrentExpectedPlanWritersShareOriginalRow(t *testing.T) {
	f := newExpectedPlanFixture(t)
	// Hold the same request row used by cancellation so a real final-write
	// transaction must wait after its material slot has already been released.
	holder, err := f.b.Begin(f.ctx)
	must(t, err)
	defer func() { _ = holder.Rollback(f.ctx) }()
	_, err = holder.Exec(f.ctx, `SELECT id FROM problem_reference_requests WHERE id=$1 FOR UPDATE`, f.request.ID)
	must(t, err)
	s := f.service(t, f.session, f.store)
	type outcome struct {
		plan problemreference.ExpectedPlan
		err  error
	}
	done := make(chan outcome, 2)
	start := func() {
		go func() {
			p, err := s.Prepare(f.ctx, f.credential, expectedPlanKey)
			done <- outcome{p, err}
		}()
	}
	start()
	waitBacklogBlocked(t, f.backlogFixture, holder.Conn().PgConn().PID(), f.appA)
	start()
	must(t, holder.Commit(f.ctx))
	for range 2 {
		select {
		case r := <-done:
			must(t, r.err)
			if !reflect.DeepEqual(r.plan, f.want) {
				t.Fatal("concurrent writers returned different bytes")
			}
		case <-f.ctx.Done():
			t.Fatal("concurrent preparation did not finish")
		}
	}
	f.planCount(t, 1)
}

func TestExpectedPlanConflictingBytesDoNotOverwrite(t *testing.T) {
	f := newExpectedPlanFixture(t)
	var other problem.ReferencePlan
	must(t, json.Unmarshal(f.want.Encoding, &other))
	other.Cases[0].Answer.SHA256 = strings.Repeat("a", 64)
	raw, err := json.Marshal(other)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_reference_expected_plans VALUES ($1,$2,$3)`, f.request.ID, raw, other.Digest())
	must(t, err)
	s := f.service(t, f.session, f.store)
	plan, err := s.Prepare(f.ctx, f.credential, expectedPlanKey)
	noExpectedPlan(t, plan, err, problemreference.ErrPlanConflict)
	plan, err = s.Receipt(f.ctx, f.credential, expectedPlanKey)
	must(t, err)
	if !bytes.Equal(plan.Encoding, raw) || plan.SHA256 != other.Digest() {
		t.Fatal("different plan overwrote the recorded bytes")
	}
	f.planCount(t, 1)
}

func TestExpectedPlanQueryUsesRequestPrimaryKey(t *testing.T) {
	f := newExpectedPlanFixture(t)
	tx, err := f.a.BeginTx(f.ctx, pgx.TxOptions{})
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_reference_requests
		(id,owner_subject_id,request_key,draft_id,source_id,content_version,import_revision,source_lock_sha256,
		 identity_protocol,profile_encoding,profile_sha256,auth_generation,recovery_id,author_revision)
		SELECT md5('expected-plan-'||n::text),r.owner_subject_id,'expected-plan-key-'||n::text,
		r.draft_id,r.source_id,r.content_version,r.import_revision,r.source_lock_sha256,
		r.identity_protocol,r.profile_encoding,r.profile_sha256,r.auth_generation,r.recovery_id,r.author_revision
		FROM problem_reference_requests r CROSS JOIN generate_series(1,2000) n WHERE r.id=$1`, f.request.ID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_reference_expected_plans SELECT id,$1,$2 FROM problem_reference_requests`,
		f.want.Encoding, f.want.SHA256)
	must(t, err)
	_, err = tx.Exec(f.ctx, `ANALYZE problem_reference_expected_plans`)
	must(t, err)
	sql, err := os.ReadFile("../../db/queries/problemreference/expected_plans.sql")
	must(t, err)
	query := strings.SplitN(string(sql), "\n\n", 2)[0]
	query = strings.SplitN(query, "\n", 2)[1]
	var evidence string
	must(t, tx.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+strings.ReplaceAll(query, "@request_id", "$1"), f.request.ID).Scan(&evidence))
	if !strings.Contains(evidence, "problem_reference_expected_plans_pkey") || !strings.Contains(evidence, "Index Cond") {
		t.Fatal("expected bounded primary key lookup", evidence)
	}
	t.Log(evidence)

	claimSQL, err := os.ReadFile("../../db/queries/problemreference/worker_claim.sql")
	must(t, err)
	mustAnalyzeClaimLookup := func(name, param, index string) {
		t.Helper()
		chunks := strings.Split(string(claimSQL), "-- name: "+name+" :one\n")
		if len(chunks) != 2 {
			t.Fatal("missing authoritative claim query", name)
		}
		query := strings.Split(chunks[1], "\n-- name:")[0]
		must(t, tx.QueryRow(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+strings.ReplaceAll(query, param, "$1"), f.request.ID).Scan(&evidence))
		if !strings.Contains(evidence, index) || !strings.Contains(evidence, "Index Cond") {
			t.Fatal("claim lookup lost its primary-key bound", name, evidence)
		}
		t.Log(evidence)
	}
	_, err = tx.Exec(f.ctx, `ANALYZE problem_reference_requests`)
	must(t, err)
	mustAnalyzeClaimLookup("ReferenceRequestByIDForUpdate", "@id", "problem_reference_requests_pkey")
	mustAnalyzeClaimLookup("ReferenceExpectedPlanForShare", "@request_id", "problem_reference_expected_plans_pkey")
}
