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
	enabled := workerapi.WithReferenceExecution(http.NotFoundHandler(), claims, claims, ca.Roots, materials)
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
	expectStatus(s, workerCert, "differential/assignment", input, 409)
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
}
