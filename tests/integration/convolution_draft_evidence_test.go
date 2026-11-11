//go:build integration && linux

package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
	"github.com/jackc/pgx/v5/pgconn"
)

type observedDraftSolvers struct {
	*materialstore.Local
	opened []materialstore.Object
}

func (s *observedDraftSolvers) Open(ctx context.Context, object materialstore.Object) (*materialstore.Snapshot, error) {
	s.opened = append(s.opened, object)
	return s.Local.Open(ctx, object)
}

func TestPrivateDraftConfiguredConvolutionEvidence(t *testing.T) {
	directory := os.Getenv("OJ_PRIVATE_DRAFT_BUNDLE_DIR")
	if directory == "" {
		t.Skip("requires the preverified private Library Checker bundle")
	}
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	f.ctx = ctx
	lock, err := os.ReadFile("../../config/problem-sources/library-checker.json")
	must(t, err)
	p, err := problem.LoadBundle(directory, problemvalidation.ConvolutionSourceID, lock)
	must(t, err)
	t.Cleanup(func() { must(t, p.Close()) })
	expected, err := p.ImportIdentity()
	must(t, err)
	catalog, err := problemreference.NewCatalog(map[string]*problem.Prepared{expected.SourceID: p})
	must(t, err)

	root := filepath.Join(t.TempDir(), "objects")
	must(t, os.Mkdir(root, 0700))
	config := materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1}
	written, err := materialstore.OpenLocal(config)
	must(t, err)
	t.Cleanup(func() { must(t, written.Close()) })
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: f.actor.SubjectID, AuthGeneration: f.actor.AuthGeneration, RecoveryID: f.actor.RecoveryID}}
	service, err := problem.New(sessions, f.repository, written)
	must(t, err)
	credential, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	must(t, err)
	created, err := service.Create(ctx, credential, "convolution-evidence-import", p)
	must(t, err)
	must(t, p.Close())
	must(t, written.Close())

	reader, err := problempg.New(f.b, f.access)
	must(t, err)
	persisted, err := reader.GetOwn(ctx, f.actor, created.ID)
	must(t, err)
	reopened, err := materialstore.OpenLocal(config)
	must(t, err)
	t.Cleanup(func() { must(t, reopened.Close()) })
	objects := &observedDraftSolvers{Local: reopened}
	untrusted := expected
	untrusted.SourceLockSHA256 = strings.Repeat("0", 64)
	if _, err := problemvalidation.FreezeDraftConvolution(ctx, persisted, created.ID, f.actor.SubjectID, untrusted, objects, lock); err == nil || len(objects.opened) != 0 {
		t.Fatal("unreviewed source lock reached object storage")
	}
	if _, err := problemvalidation.FreezeDraftConvolution(ctx, persisted, created.ID, identityID(1101), expected, objects, lock); !errors.Is(err, problem.ErrInvalid) || len(objects.opened) != 0 {
		t.Fatal("wrong owner reached object storage", err)
	}
	sources, err := problemvalidation.FreezeDraftConvolution(ctx, persisted, created.ID, f.actor.SubjectID, expected, objects, lock)
	must(t, err)
	report := problemvalidation.NewConvolutionReport(sources)
	wantBinding := problemvalidation.ConvolutionDraftBinding{ID: created.ID, Version: expected.Version, ImportRevision: expected.ImportRevision}
	if report.Draft == nil || *report.Draft != wantBinding || report.SourceLock != expected.SourceLockSHA256 ||
		report.Status != "incomplete" || report.StopConfirmed || report.Publication != "blocked" {
		t.Fatal("evidence lost its draft provenance or implied validation")
	}
	wantObjects := []materialstore.Object{
		{SHA256: report.Oracle.SHA256, Size: report.Oracle.Size},
		{SHA256: report.Candidate.SHA256, Size: report.Candidate.Size},
	}
	if !slices.Equal(objects.opened, wantObjects) {
		t.Fatal("persisted preparation did not read exactly the two reviewed roles")
	}
	raw, err := json.Marshal(report)
	must(t, err)
	if strings.Contains(string(raw), f.actor.SubjectID) {
		t.Fatal("report contained private owner identity")
	}
	again, err := reader.GetOwn(ctx, f.actor, created.ID)
	must(t, err)
	if again.State != "private-draft" || again.ValidationState != "not-run" || again.ImportRevision != expected.ImportRevision {
		t.Fatal("preparation altered the persisted draft")
	}
	t.Run("expected plans", func(t *testing.T) {
		access, err := problempg.NewReferenceAccess(f.access)
		must(t, err)
		repo, err := referencepg.New(f.a, access)
		must(t, err)
		requests := referenceRequestFixture{draftFixture: f, store: repo, catalog: catalog, draft: persisted}
		requests.limits(t, 3, 3)
		testConvolutionPlanPersistence(t, requests, reopened, sessions, credential, lock, wantObjects)
	})
}

type failConvolutionCandidate struct {
	*materialstore.Local
	oracle, candidate   materialstore.Object
	sawOracle, rejected bool
}

func (s *failConvolutionCandidate) Open(ctx context.Context, object materialstore.Object) (*materialstore.Snapshot, error) {
	if object == s.candidate && s.sawOracle {
		s.rejected = true
		return nil, materialstore.ErrIntegrity
	}
	if object == s.oracle {
		s.sawOracle = true
	}
	return s.Local.Open(ctx, object)
}

func testConvolutionPlanPersistence(t *testing.T, f referenceRequestFixture, materials *materialstore.Local, sessions fixedDraftSession,
	credential redisstore.Credential, lock []byte, objects []materialstore.Object,
) {
	t.Helper()
	const historicalKey = "convolution-pre-migration-plan"
	historical, err := f.store.Enqueue(f.ctx, f.actor, historicalKey, f.draft.ID, f.catalog)
	must(t, err)
	snapshot, err := historical.FreezeDraft(f.ctx, f.draft, materials)
	must(t, err)
	formal := snapshot.Plan()
	must(t, snapshot.Close())
	raw, err := json.Marshal(formal)
	must(t, err)
	_, err = f.provider.DownTo(f.ctx, 13)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("startup accepted pre-comparison schema")
	}
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_reference_expected_plans (request_id,plan_encoding,plan_sha256) VALUES ($1,$2,$3)`,
		historical.ID, raw, formal.Digest())
	must(t, err)
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	// Downgrade must preserve actual schema 13 plans, not just empty tables.
	_, err = f.provider.DownTo(f.ctx, 13)
	must(t, err)
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))

	plans, err := problemreference.NewPlanService(sessions, f.store, f.repository, materials, lock)
	must(t, err)
	old, err := plans.Prepare(f.ctx, credential, historicalKey)
	must(t, err)
	if old.Differential != nil || old.SHA256 != formal.Digest() || !slices.Equal(old.Encoding, raw) {
		t.Fatal("pre-migration plan changed on replay")
	}

	const key = "convolution-expected-plan-original"
	original, err := f.store.Enqueue(f.ctx, f.actor, key, f.draft.ID, f.catalog)
	must(t, err)
	failing := &failConvolutionCandidate{Local: materials, oracle: objects[0], candidate: objects[1]}
	fails, err := problemreference.NewPlanService(sessions, f.store, f.repository, failing, lock)
	must(t, err)
	rejected, err := fails.Prepare(f.ctx, credential, key)
	noExpectedPlan(t, rejected, err, materialstore.ErrIntegrity)
	if !failing.rejected {
		t.Fatal("did not reach the candidate after the independent oracle")
	}
	missing, err := plans.Receipt(f.ctx, credential, key)
	noExpectedPlan(t, missing, err, problemreference.ErrNotFound)

	faulty, observed := lostReferenceCommitStore(t, f)
	unknown, err := problemreference.NewPlanService(sessions, expectedRepoHook{PlanRepository: f.store, persist: faulty.PersistExpectedPlan}, f.repository, materials, lock)
	must(t, err)
	missing, err = unknown.Prepare(f.ctx, credential, key)
	noExpectedPlan(t, missing, err, problemreference.ErrCommitUnknown)
	select {
	case committed := <-observed:
		if !committed {
			t.Fatal("did not lose an actual COMMIT reply")
		}
	case <-f.ctx.Done():
		t.Fatal("commit-loss fixture was not reached")
	}
	recovered, err := plans.Receipt(f.ctx, credential, key)
	must(t, err)
	if recovered.Differential == nil || !recovered.ValidForRequest(original) || recovered.SHA256 != formal.Digest() || !slices.Equal(recovered.Encoding, raw) {
		t.Fatal("original key did not recover both plans with the original formal digest")
	}
	var sidecar problemvalidation.ConvolutionPlan
	must(t, json.Unmarshal(recovered.Differential.Encoding, &sidecar))
	if sidecar.Draft.ID != f.draft.ID || sidecar.Oracle.SHA256 != objects[0].SHA256 || sidecar.Candidate.SHA256 != objects[1].SHA256 ||
		sidecar.Cases != 24 || strings.Contains(string(recovered.Differential.Encoding), f.actor.SubjectID) {
		t.Fatal("persisted comparison metadata does not identify the reviewed trial")
	}
	replay, err := plans.Prepare(f.ctx, credential, key)
	must(t, err)
	if !reflect.DeepEqual(replay, recovered) {
		t.Fatal("replay did not return the original two plans")
	}
	checkDifferentialPlanConstraints(t, f, original.ID, recovered.Differential)
	sidecar.CorpusSHA256 = strings.Repeat("0", 64)
	different, err := json.Marshal(sidecar)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `UPDATE problem_reference_expected_plans SET differential_encoding=$1,differential_sha256=$2 WHERE request_id=$3`,
		different, problemfixture.Hash(different), original.ID)
	must(t, err)
	conflict, err := plans.Prepare(f.ctx, credential, key)
	noExpectedPlan(t, conflict, err, problemreference.ErrPlanConflict)
	prior, err := plans.Receipt(f.ctx, credential, key)
	must(t, err)
	if prior.Differential == nil || !slices.Equal(prior.Differential.Encoding, different) {
		t.Fatal("different comparison overwrote the stored bytes")
	}
	if _, err := f.provider.DownTo(f.ctx, 13); err == nil || !strings.Contains(err.Error(), "cannot discard differential reference plans") {
		t.Fatalf("Down14 did not preserve comparison history: %v", err)
	}
	must(t, db.CheckSchema(f.ctx, f.a))
}

func checkDifferentialPlanConstraints(t *testing.T, f referenceRequestFixture, id string, plan *problemreference.DifferentialExpectedPlan) {
	t.Helper()
	for _, tc := range []struct {
		raw    []byte
		digest any
	}{
		{nil, plan.SHA256},
		{plan.Encoding, nil},
		{plan.Encoding, strings.Repeat("0", 64)},
	} {
		_, err := f.a.Exec(f.ctx, `UPDATE problem_reference_expected_plans SET differential_encoding=$1, differential_sha256=$2 WHERE request_id=$3`,
			tc.raw, tc.digest, id)
		var constraint *pgconn.PgError
		if !errors.As(err, &constraint) || constraint.Code != "23514" {
			t.Fatalf("incomplete or corrupt differential plan passed the database constraint: %v", err)
		}
	}
}
