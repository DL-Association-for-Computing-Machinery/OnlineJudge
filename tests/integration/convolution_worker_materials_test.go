//go:build integration && linux

package integration_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func testConvolutionTransport(t *testing.T, f referenceRequestFixture, local *materialstore.Local,
	request problemreference.Request, plan problemreference.ExpectedPlan, objects []materialstore.Object,
) {
	t.Helper()
	const node = "reference-node"
	_, err := f.a.Exec(f.ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,$2)`, node, 1)
	must(t, err)
	claims, err := referencepg.NewReferenceClaimer(f.store, submissionpg.LockReferenceNode, node, problemvalidation.SourceLockSHA256)
	must(t, err)
	var strip, revoke, hookFailed atomic.Bool
	materials, err := problemreference.NewReferenceMaterials(claims, referenceObjectHook{reader: local, hook: func() {
		switch {
		case strip.Swap(false):
			_, err := f.b.Exec(f.ctx, `UPDATE problem_reference_expected_plans SET differential_encoding=NULL,differential_sha256=NULL WHERE request_id=$1`, request.ID)
			hookFailed.Store(err != nil)
		case revoke.Swap(false):
			_, err := f.b.Exec(f.ctx, `UPDATE worker_nodes SET credential_version=2 WHERE id=$1`, node)
			hookFailed.Store(err != nil)
		}
	}})
	must(t, err)
	ca := testpki.New(t)
	expiry := time.Now().Add(time.Minute)
	serverCert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, expiry)
	workerCert, _, _ := ca.Issue(t, "spiffe://onlinejudge/reference-worker/"+node+"/1", x509.ExtKeyUsageClientAuth, expiry)
	judgeCert, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/"+node+"/1", x509.ExtKeyUsageClientAuth, expiry)
	newServer := func(handler http.Handler) *httptest.Server {
		s := httptest.NewUnstartedServer(handler)
		s.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs: ca.Roots, Certificates: []tls.Certificate{serverCert}}
		s.StartTLS()
		t.Cleanup(s.Close)
		return s
	}
	enabled := workerapi.WithReferenceExecution(http.NotFoundHandler(), claims, claims, ca.Roots, materials, claims)
	s := newServer(enabled)
	disabled := newServer(workerapi.WithReferenceClaims(http.NotFoundHandler(), claims, ca.Roots, materials))
	acquirer, err := workerclient.NewAcquirer(s.URL, ca.Roots, workerCert)
	must(t, err)
	defer acquirer.Close()
	claim, err := acquirer.ClaimReference(f.ctx, problemreference.ClaimInput{RequestID: request.ID, SourceID: request.Specification.Identity.SourceID, PlanSHA256: plan.SHA256})
	must(t, err)
	input := problemreference.MaterialInput{ClaimInput: problemreference.ClaimInput{
		RequestID: claim.RequestID, SourceID: claim.SourceID, PlanSHA256: claim.PlanSHA256,
	}, Token: claim.Token}
	frozen, err := acquirer.FetchDifferential(f.ctx, claim)
	must(t, err)
	recovered, err := frozen.DraftPlan(claim.PlanSHA256)
	must(t, err)
	var expected problemvalidation.ConvolutionPlan
	must(t, json.Unmarshal(plan.Differential.Encoding, &expected))
	if !reflect.DeepEqual(recovered, expected) {
		t.Fatal("Worker recovered a different plan from the two verified objects")
	}
	t.Log("real mTLS returned exactly the reviewed solver pair and its persisted plan")

	for _, invalidDescriptor := range []bool{true, false} {
		var reads atomic.Int32
		broken := newServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case invalidDescriptor && strings.HasSuffix(r.URL.Path, "/differential/assignment"):
				raw := append(bytes.Clone(plan.Differential.Encoding), '\n')
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(problemreference.DifferentialDescriptor{
					DifferentialExpectedPlan: problemreference.DifferentialExpectedPlan{Encoding: raw, SHA256: problemfixture.Hash(raw)},
					RemainingMS:              5000,
				})
			case strings.HasSuffix(r.URL.Path, "/differential/material") && reads.Add(1) == 2:
				w.WriteHeader(http.StatusServiceUnavailable)
			default:
				enabled.ServeHTTP(w, r)
			}
		}))
		client, err := workerclient.NewAcquirer(broken.URL, ca.Roots, workerCert)
		must(t, err)
		zero, err := client.FetchDifferential(f.ctx, claim)
		client.Close()
		if zero != (problemvalidation.ConvolutionSources{}) || !errors.Is(err, workerclient.ErrMaterials) ||
			(invalidDescriptor && reads.Load() != 0) || (!invalidDescriptor && reads.Load() != 2) {
			t.Fatal("invalid descriptor or second response yielded a partial pair", err, reads.Load())
		}
	}

	expectStatus := func(server *httptest.Server, cert tls.Certificate, path string, body any, want int) {
		t.Helper()
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: ca.Roots, Certificates: []tls.Certificate{cert}}}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		raw, err := json.Marshal(body)
		must(t, err)
		response, err := client.Post(server.URL+"/api/v1/worker/reference/"+path, "application/json", bytes.NewReader(raw))
		must(t, err)
		must(t, response.Body.Close())
		if response.StatusCode != want {
			t.Fatalf("%s returned %d; expected %d", path, response.StatusCode, want)
		}
	}
	var formal problem.ReferencePlan
	must(t, json.Unmarshal(plan.Encoding, &formal))
	foreign := formal.Cases[0].Input
	download := func(object materialstore.Object, digest string) any {
		return struct {
			problemreference.MaterialInput
			Object materialstore.Object `json:"object"`
			Digest string               `json:"differential_sha256"`
		}{input, object, digest}
	}
	expectStatus(disabled, workerCert, "differential/assignment", input, 404)
	expectStatus(s, judgeCert, "differential/assignment", input, 401)
	wrongToken := input
	wrongToken.Token = strings.Repeat("f", 64)
	expectStatus(s, workerCert, "differential/assignment", wrongToken, 409)
	expectStatus(s, workerCert, "differential/material", download(foreign, plan.Differential.SHA256), 409)
	expectStatus(s, workerCert, "differential/material", download(objects[0], strings.Repeat("a", 64)), 409)
	expectStatus(s, workerCert, "material", struct {
		problemreference.MaterialInput
		Object materialstore.Object `json:"object"`
	}{input, objects[0]}, 409)
	var until time.Time
	must(t, f.a.QueryRow(f.ctx, `SELECT lease_until FROM problem_reference_worker_claim WHERE singleton`).Scan(&until))
	if !until.Equal(claim.Until) {
		t.Fatal("differential material reads renewed the reservation")
	}
	strip.Store(true)
	expectStatus(s, workerCert, "differential/material", download(objects[0], plan.Differential.SHA256), 409)
	if strip.Load() || hookFailed.Load() {
		t.Fatal("comparison plan removal after storage I/O was not exercised")
	}
	expectStatus(s, workerCert, "differential/assignment", input, 204)
	expectStatus(s, workerCert, "differential/assignment", wrongToken, 409)
	expectStatus(s, workerCert, "assignment", input, 200)
	_, err = f.b.Exec(f.ctx, `UPDATE problem_reference_expected_plans SET differential_encoding=$1,differential_sha256=$2 WHERE request_id=$3`,
		plan.Differential.Encoding, plan.Differential.SHA256, request.ID)
	must(t, err)
	revoke.Store(true)
	expectStatus(s, workerCert, "differential/material", download(objects[0], plan.Differential.SHA256), 401)
	if revoke.Load() || hookFailed.Load() {
		t.Fatal("credential rotation after storage I/O was not exercised")
	}
	file, err := local.Open(f.ctx, objects[0])
	must(t, err)
	must(t, file.Close())
	zero, err := acquirer.FetchDifferential(f.ctx, claim)
	if !errors.Is(err, submission.ErrNode) {
		t.Fatal("revoked Worker was not denied as an invalid node", err)
	}
	if zero != (problemvalidation.ConvolutionSources{}) {
		t.Fatal("revoked Worker obtained a partial comparison snapshot")
	}

	_, err = f.a.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()-interval '1 second'`)
	must(t, err)
	newCredential, _, _ := ca.Issue(t, "spiffe://onlinejudge/reference-worker/"+node+"/2", x509.ExtKeyUsageClientAuth, expiry)
	reportClient, err := workerclient.NewAcquirer(s.URL, ca.Roots, newCredential)
	must(t, err)
	defer reportClient.Close()
	reportClaim, err := reportClient.ClaimReference(f.ctx, input.ClaimInput)
	must(t, err)
	testConvolutionReportAdmission(t, f, reportClient, reportClaim, request, plan, formal, expected, frozen)
}

// The isolated host test exercises the real execution. This separate database
// fixture exercises first admission and replay using controlled node evidence.
func testConvolutionReportAdmission(t *testing.T, f referenceRequestFixture, c *workerclient.Acquirer, claim problemreference.WorkerClaim,
	request problemreference.Request, plan problemreference.ExpectedPlan,
	formal problem.ReferencePlan, expected problemvalidation.ConvolutionPlan, sources problemvalidation.ConvolutionSources,
) {
	t.Helper()
	profile, _, err := problemvalidation.ResolveSource(claim.SourceID)
	must(t, err)
	slot, err := workerhost.ReferenceSlotForTier(experiment.ReferenceConvolution)
	must(t, err)
	r := problemvalidation.Report{Schema: "oj.reference-validation.v1", Publication: "blocked", Verdict: "AC",
		Profile: profile, ProfileSHA256: profile.Digest(), Plan: formal, PlanSHA256: claim.PlanSHA256,
		CompileUsage: &execution.Usage{}, Slot: slot.ContainerName(), ContainerID: strings.Repeat("a", 64),
		StartedAt: "2026-11-11T00:00:00Z"}
	for _, p := range formal.Cases {
		r.Points = append(r.Points, problemvalidation.Point{ID: p.ID, Verdict: "AC", Usage: &execution.Usage{}})
	}
	must(t, r.Finalize(nil, nil, nil))
	old, err := problemreference.FreezeWorkerReport(claim, r)
	must(t, err)
	if err := c.ReportReference(f.ctx, old); !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatal("formal AC alone consumed a prepared comparison", err)
	}

	comparison := problemvalidation.NewConvolutionReport(sources)
	comparison.ProfileSHA256, comparison.CorpusSHA256 = expected.ProfileSHA256, expected.CorpusSHA256
	comparison.Status, comparison.OracleParsed, comparison.CandidateEqual = "mismatch", 24, 9
	comparison.ContainerID, comparison.StartedAt, comparison.StopConfirmed = r.ContainerID, r.StartedAt, true
	const input = "2 2\n1 2\n3 4\n"
	comparison.Counterexample = &problemvalidation.ConvolutionCounterexample{Case: 10, Input: input, InputSHA256: problemfixture.Hash([]byte(input)),
		Expected: []uint32{3, 10, 8}, Actual: []uint32{4, 10, 8}}
	frozen, err := problemreference.FreezeWorkerReportWithComparison(claim, r, expected, comparison)
	must(t, err)
	evidence, err := frozen.DecodeEvidence()
	must(t, err)
	bad := frozen
	evidence.Comparison.Counterexample.Actual[0] = 998244353
	bad.Evidence, err = json.Marshal(evidence)
	must(t, err)
	if err := c.ReportReference(f.ctx, bad); !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatal("out-of-field counterexample was admitted", err)
	}
	must(t, c.ReportReference(f.ctx, frozen))
	view, err := f.store.AuthorReport(f.ctx, f.actor, request.RequestKey)
	must(t, err)
	if view.RequestID != claim.RequestID || view.Version != formal.Version ||
		view.Formal != (problemreference.AuthorFormalResult{Verdict: "AC", Passed: len(formal.Cases), Total: len(formal.Cases)}) ||
		view.Comparison == nil || view.Comparison.Status != "mismatch" || view.Comparison.Matched != 9 || view.Comparison.Total != 24 ||
		!reflect.DeepEqual(view.Comparison.Counterexample, &problemreference.AuthorCounterexample{
			Case: 10, Input: input, Expected: []uint32{3, 10, 8}, Actual: []uint32{4, 10, 8},
		}) {
		t.Fatal("author projection did not preserve the formal summary and first generated difference")
	}
	request.State = problemreference.Reported
	legacy, err := problemreference.ProjectAuthorReport(request, plan, plan.SHA256, problemfixture.Hash(old.Evidence), old.Evidence)
	must(t, err)
	if legacy.Formal.Verdict != "AC" || legacy.Comparison != nil {
		t.Fatal("formal-only history predating comparison admission did not project")
	}
	for _, test := range []struct {
		formalSHA, evidenceSHA string
		raw                    []byte
	}{
		{strings.Repeat("f", 64), problemfixture.Hash(frozen.Evidence), frozen.Evidence},
		{plan.SHA256, strings.Repeat("f", 64), frozen.Evidence},
		{plan.SHA256, problemfixture.Hash(bad.Evidence), bad.Evidence},
	} {
		if _, err := problemreference.ProjectAuthorReport(request, plan, test.formalSHA, test.evidenceSHA, test.raw); !errors.Is(err, problemreference.ErrInvalid) {
			t.Fatal("invalid historical binding or counterexample was exposed", err)
		}
	}
	var state string
	var stored []byte
	must(t, f.a.QueryRow(f.ctx, `SELECT r.state, p.evidence FROM problem_reference_requests r
		JOIN problem_reference_worker_reports p ON p.request_id=r.id WHERE r.id=$1`, claim.RequestID).Scan(&state, &stored))
	if state != problemreference.Reported || !bytes.Equal(stored, frozen.Evidence) {
		t.Fatal("did not atomically store the determinate mismatch evidence")
	}
	must(t, c.ReportReference(f.ctx, frozen))
	if err := c.ReportReference(f.ctx, old); !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatal("different bytes confirmed as an exact replay", err)
	}
	assertConvolutionReview(t, f, request, plan, frozen.Evidence, view.Comparison)
	t.Log("mTLS/PG: formal-only AC and invalid counterexample refused; mismatch persisted and replayed exactly")
}

func assertConvolutionReview(t *testing.T, f referenceRequestFixture, request problemreference.Request,
	plan problemreference.ExpectedPlan, report []byte, want *problemreference.AuthorComparisonResult,
) {
	t.Helper()
	bound := request.Specification.Identity
	item, err := f.store.SubmitReview(f.ctx, f.actor, problemreference.ReviewIntent{RequestKey: "review-convolution-mismatch",
		DraftID: request.DraftID, ImportRevision: bound.ImportRevision, ReferenceRequestID: request.ID})
	must(t, err)
	reviews, err := referencepg.NewReviewStore(f.b, f.access)
	must(t, err)
	reader := f.actor
	reader.SubjectID = identityID(1101)
	grant := func(subject string, rev int64, enabled bool) {
		t.Helper()
		_, err := f.access.SetReviewerRole(f.ctx, identity.ProblemRoleChange{SubjectID: subject, ExpectedRevision: rev, Enabled: enabled,
			Operator: "comparison-fixture", Reason: "verify private mismatch access"})
		must(t, err)
	}
	denied := func(actor identity.ProblemActor, id string, want error) {
		t.Helper()
		view, err := reviews.ReviewComparison(f.ctx, actor, id)
		requireReviewError(t, err, want)
		if view != (problemreference.ReviewComparison{}) {
			t.Fatal("failed comparison read returned a partial result")
		}
	}
	grant(reader.SubjectID, 0, true)
	grant(f.actor.SubjectID, 0, true)
	denied(reader, item.ID, problemreference.ErrNotFound)
	_, err = reviews.SetAssignment(f.ctx, problemreference.ReviewAssignmentChange{ReviewID: item.ID, ReviewerSubjectID: reader.SubjectID,
		ExpectedReviewerRevision: 1, Operator: "comparison-fixture", Reason: "assign synthetic non-author"})
	must(t, err)
	got, err := reviews.ReviewComparison(f.ctx, reader, item.ID)
	must(t, err)
	summary, err := reviews.ReviewEvidence(f.ctx, reader, item.ID)
	must(t, err)
	if !got.Valid() || got.Evidence != summary || summary.ReviewID != item.ID ||
		summary.ImportRevision != bound.ImportRevision || summary.ContentVersion != bound.Version ||
		summary.ComparisonPlan != plan.Differential.SHA256 || summary.EvidenceSHA256 != problemfixture.Hash(report) ||
		!reflect.DeepEqual(got.Comparison, want) {
		t.Fatal("assigned comparison did not preserve the fixed revision, report identity and generated counterexample")
	}
	denied(f.actor, item.ID, problemreference.ErrNotFound)
	denied(reader, request.ID, problemreference.ErrNotFound)
	// Even a separately valid summary cannot authorize a different report digest.
	_, err = f.a.Exec(f.ctx, `UPDATE problem_review_submissions SET evidence_sha256=$1 WHERE id=$2`, strings.Repeat("f", 64), item.ID)
	must(t, err)
	denied(reader, item.ID, problemreference.ErrStore)
	grant(reader.SubjectID, 1, false)
	denied(reader, item.ID, identity.ErrReviewerRequired)
}
