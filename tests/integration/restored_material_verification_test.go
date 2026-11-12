//go:build integration && linux

package integration_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/restorecheck"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestVerifyRestoredMaterialsFromArchive(t *testing.T) {
	f := newExpectedPlanFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	requestID, comparison, formalCount, comparisonDraft := addHistoricalComparison(t, f)
	judge, private := t.TempDir(), t.TempDir()
	must(t, os.Chmod(private, 0700))
	must(t, experiment.WriteMaterials(judge))
	// Copy the exact archived material set before creating a separate target.
	// The local store is digest named, so shared draft objects are copied once.
	for _, ref := range append(append([]problem.Reference(nil), f.draft.References...), comparisonDraft.References...) {
		file, err := f.materials.Open(f.ctx, ref.Object)
		must(t, err)
		var raw bytes.Buffer
		_, err = raw.ReadFrom(file)
		must(t, err)
		must(t, file.Close())
		must(t, os.WriteFile(filepath.Join(private, ref.Object.SHA256), raw.Bytes(), 0600))
	}
	archive, schema := dumpSubmissionFixture(t, f.backlogFixture, t.TempDir())
	target := newSubmissionRestoreTarget(t, f.ctx, schema, archive)
	database := target.Config().ConnConfig.Database
	binary, err := filepath.Abs("../../.local/bin/migrate")
	must(t, err)
	call := func(dsnVariable, confirmed string) (string, error) {
		cmd := exec.CommandContext(f.ctx, binary, "verify-restored-materials", "-timeout", "5s", confirmed, judge, private)
		cmd.Env = []string{dsnVariable + "=" + target.Config().ConnString() + " search_path=" + schema + " options='-c default_transaction_read_only=on'",
			"TMPDIR=" + filepath.Join(private, "missing-scratch")}
		raw, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(raw)), err
	}
	refuse := func(want string) {
		t.Helper()
		if got, err := call("OJ_RESTORED_DATABASE_URL", database); err == nil || got != want {
			t.Fatalf("expected %q, got %q (%v)", want, got, err)
		}
	}
	if got, err := call("OJ_DATABASE_URL", database); err == nil || got != "OJ_RESTORED_DATABASE_URL is required" {
		t.Fatal("material verification fell back to the runtime DSN")
	}
	if got, err := call("OJ_RESTORED_DATABASE_URL", "wrong-db"); err == nil || got != "restored material database name did not match confirmation" {
		t.Fatal("material verification accepted a different database")
	}
	stamp := time.Unix(1_700_000_000, 0)
	for _, root := range []string{judge, private} {
		must(t, os.Chtimes(root, stamp, stamp))
	}
	pass := func() {
		t.Helper()
		got, err := call("OJ_RESTORED_DATABASE_URL", database)
		if err != nil {
			t.Fatalf("valid restored material set rejected: %s (%v)", got, err)
		}
		var count restorecheck.MaterialSummary
		must(t, json.Unmarshal([]byte(got), &count))
		var formal problem.ReferencePlan
		must(t, json.Unmarshal(f.want.Encoding, &formal))
		wantPrivate := int64(len(f.draft.References) + len(comparisonDraft.References) + len(formal.ReferencedObjects()) + formalCount + 2)
		if count.JudgeManifests != 1 || count.JudgeReferences != 4 || count.Drafts != 2 || count.ReferencePlans != 2 ||
			count.DifferentialPlans != 1 || count.PrivateReferences != wantPrivate {
			t.Fatalf("restored inventory count disagrees: %+v", count)
		}
	}
	pass()
	for _, query := range []string{
		`EXPLAIN (ANALYZE, BUFFERS) SELECT id,owner_subject_id,source_id,content_version,request_sha256,state,validation_state,document
			FROM problem_drafts WHERE id>$1 ORDER BY id LIMIT 1`,
		`EXPLAIN (ANALYZE, BUFFERS) SELECT p.request_id,r.draft_id,r.source_id,r.content_version,r.import_revision,r.source_lock_sha256,r.profile_sha256,
			p.plan_encoding,p.plan_sha256,p.differential_encoding,p.differential_sha256,COALESCE(d.owner_subject_id=r.owner_subject_id,false)
			FROM problem_reference_expected_plans p LEFT JOIN problem_reference_requests r ON r.id=p.request_id
			LEFT JOIN problem_drafts d ON d.id=r.draft_id WHERE p.request_id>$1 ORDER BY p.request_id LIMIT 1`,
	} {
		rows, err := target.Query(f.ctx, query, "")
		must(t, err)
		plan, err := pgx.CollectRows(rows, pgx.RowTo[string])
		must(t, err)
		t.Logf("restored inventory seek:\n%s", strings.Join(plan, "\n"))
	}
	for _, root := range []string{judge, private} {
		info, err := os.Stat(root)
		must(t, err)
		if !info.ModTime().Equal(stamp) {
			t.Fatal("verification created or removed an entry in a material root")
		}
	}
	entries, err := os.ReadDir(judge)
	must(t, err)
	judgeFile := filepath.Join(judge, entries[0].Name())
	judgeBytes, err := os.ReadFile(judgeFile)
	must(t, err)
	must(t, os.Remove(judgeFile))
	refuse("restored judge file " + entries[0].Name() + " failed verification")
	must(t, os.Symlink(filepath.Join(judge, entries[1].Name()), judgeFile))
	refuse("restored judge file " + entries[0].Name() + " failed verification")
	must(t, os.Remove(judgeFile))
	must(t, os.WriteFile(judgeFile, judgeBytes, 0600))
	_, err = target.Exec(f.ctx, `UPDATE submission_materials SET manifest=jsonb_set(manifest,'{version}','"wrong-version"'::jsonb) WHERE version_id=$1`, experiment.Version)
	must(t, err)
	refuse("restored judge manifest invalid")
	_, err = target.Exec(f.ctx, `UPDATE submission_materials SET manifest=$1::jsonb WHERE version_id=$2`, string(experiment.Manifest()), experiment.Version)
	must(t, err)
	ref := f.draft.References[0]
	privateFile := filepath.Join(private, ref.Object.SHA256)
	privateBytes, err := os.ReadFile(privateFile)
	must(t, err)
	broken := bytes.Clone(privateBytes)
	broken[0] ^= 1
	must(t, os.WriteFile(privateFile, broken, 0600))
	refuse("restored private object " + ref.Object.SHA256 + " failed verification")
	must(t, os.WriteFile(privateFile, privateBytes, 0600))
	_, err = target.Exec(f.ctx, `DELETE FROM problem_draft_materials WHERE draft_id=$1 AND logical_name=$2`, f.draft.ID, ref.Name)
	must(t, err)
	refuse("restored draft material inventory invalid")
	_, err = target.Exec(f.ctx, `INSERT INTO problem_draft_materials(draft_id,logical_name,sha256,byte_size) VALUES ($1,$2,$3,$4)`,
		f.draft.ID, ref.Name, ref.Object.SHA256, ref.Object.Size)
	must(t, err)
	// A well-formed plan cannot bind an existing object from another draft.
	var formal problem.ReferencePlan
	must(t, json.Unmarshal(f.want.Encoding, &formal))
	wrong := comparison
	wrong.Oracle.SHA256, wrong.Oracle.Size = formal.Source.SHA256, formal.Source.Size
	replaceComparison(t, f, target, requestID, wrong)
	refuse("restored reference plan object is outside its draft inventory")
	replaceComparison(t, f, target, requestID, comparison)

	// Even a consistently rewritten request and plan cannot claim a lock
	// different from the original restored draft's provenance.
	replaceFormal := func(p problem.ReferencePlan) {
		t.Helper()
		body, err := json.Marshal(p)
		must(t, err)
		_, err = target.Exec(f.ctx, `UPDATE problem_reference_requests SET source_lock_sha256=$1 WHERE id=$2`, p.SourceLockSHA256, f.request.ID)
		must(t, err)
		_, err = target.Exec(f.ctx, `UPDATE problem_reference_expected_plans SET plan_encoding=$1,plan_sha256=$2 WHERE request_id=$3`, body, p.Digest(), f.request.ID)
		must(t, err)
	}
	wrongLock := formal
	wrongLock.SourceLockSHA256 = strings.Repeat("a", 64)
	replaceFormal(wrongLock)
	refuse("restored reference plan does not match its draft")
	replaceFormal(formal)
	pass()
}

func addHistoricalComparison(t *testing.T, f expectedPlanFixture) (string, problemvalidation.ConvolutionPlan, int, problem.Draft) {
	t.Helper()
	const root = "convolution/convolution_mod"
	oracle, candidate := []byte("synthetic oracle, not executed\n"), []byte("synthetic candidate, not executed\n")
	b := problemfixture.NewConvolution(t)
	b.AddOriginal(t, root+"/sol/naive.cpp", oracle)
	b.AddOriginal(t, root+"/sol/correct.cpp", candidate)
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, problemvalidation.ConvolutionSourceID, b.Lock)
	must(t, err)
	t.Cleanup(func() { must(t, p.Close()) })
	svc, err := problem.New(f.session, f.repository, f.materials)
	must(t, err)
	draft, err := svc.Create(f.ctx, f.credential, "restore-comparison-draft", p)
	must(t, err)
	catalog, err := problemreference.NewCatalog(map[string]*problem.Prepared{draft.SourceID: p})
	must(t, err)
	f.limits(t, 2, 2)
	r, err := f.store.Enqueue(f.ctx, f.actor, "restore-comparison-intent", draft.ID, catalog)
	must(t, err)
	snapshot, err := r.FreezeDraft(f.ctx, draft, f.materials)
	must(t, err)
	formal := snapshot.Plan()
	must(t, snapshot.Close())
	raw, err := json.Marshal(formal)
	must(t, err)
	comparison := problemvalidation.ConvolutionPlan{
		Protocol: problemvalidation.ConvolutionPlanProtocol, SourceID: draft.SourceID,
		SourceRevision: strings.Repeat("1", 40), SourceLock: formal.SourceLockSHA256,
		Draft:     problemvalidation.ConvolutionDraftBinding{ID: draft.ID, Version: draft.Version, ImportRevision: draft.ImportRevision},
		Oracle:    problemvalidation.ConvolutionProgram{Path: root + "/sol/naive.cpp", SHA256: problemfixture.Hash(oracle), Size: int64(len(oracle))},
		Candidate: problemvalidation.ConvolutionProgram{Path: root + "/sol/correct.cpp", SHA256: problemfixture.Hash(candidate), Size: int64(len(candidate))},
		Corpus:    "convolution-mod-small-v1", Seed: 0x4f4a2d50342d3031, Cases: 24, CorpusSHA256: strings.Repeat("b", 64),
		ProfileSHA256: r.Specification.ProfileSHA256, FormalSHA256: formal.Digest(),
	}
	_, err = f.a.Exec(f.ctx, `INSERT INTO problem_reference_expected_plans(request_id,plan_encoding,plan_sha256) VALUES ($1,$2,$3)`, r.ID, raw, formal.Digest())
	must(t, err)
	replaceComparison(t, f, f.a, r.ID, comparison)
	return r.ID, comparison, len(formal.ReferencedObjects()), draft
}

func replaceComparison(t *testing.T, f expectedPlanFixture, pool *pgxpool.Pool, id string, p problemvalidation.ConvolutionPlan) {
	t.Helper()
	raw, err := json.Marshal(p)
	must(t, err)
	_, err = pool.Exec(f.ctx, `UPDATE problem_reference_expected_plans SET differential_encoding=$1,differential_sha256=$2 WHERE request_id=$3`, raw, p.Digest(), id)
	must(t, err)
}
