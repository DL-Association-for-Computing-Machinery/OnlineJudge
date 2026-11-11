//go:build integration && linux

package integration_test

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
)

func TestReferenceDispatchPagesAndClaimsAtMostOnce(t *testing.T) {
	f, claims, node, initial := claimFixture(t)
	f.limits(t, 12, 12)
	requests := []problemreference.Request{f.request}
	for i := range 8 {
		r, err := f.store.Enqueue(f.ctx, f.actor, fmt.Sprintf("dispatch-page-intent-%02d", i), f.draft.ID, f.catalog)
		must(t, err)
		requests = append(requests, r)
	}
	sort.Slice(requests, func(i, j int) bool { return requests[i].ID < requests[j].ID })
	wanted := requests[8]
	prepareClaim(t, f, wanted.RequestKey)
	input := problemreference.DispatchInput{SourceID: initial.SourceID}
	page, err := claims.DispatchReference(f.ctx, node, input)
	must(t, err)
	if page.Claim != nil || page.After != requests[7].ID || !page.ValidFor(input) {
		t.Fatal("first page did not stop at eight unprepared requests")
	}
	input.After = page.After
	type outcome struct {
		result problemreference.DispatchResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	for range 2 {
		go func() {
			result, err := claims.DispatchReference(f.ctx, node, input)
			outcomes <- outcome{result, err}
		}()
	}
	winners := 0
	for range 2 {
		out := <-outcomes
		must(t, out.err)
		if out.result.Claim != nil {
			winners++
			if out.result.Claim.RequestID != wanted.ID || out.result.Claim.PlanSHA256 != f.want.SHA256 || !out.result.ValidFor(input) {
				t.Fatal("reservation selected the wrong request or digest")
			}
		}
	}
	if winners != 1 {
		t.Fatal("expected exactly one concurrent reservation", winners)
	}
	_, err = f.a.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()-interval '1 second'`)
	must(t, err)
	prepareClaim(t, f, requests[0].RequestKey)
	replacement, err := claims.DispatchReference(f.ctx, node, problemreference.DispatchInput{SourceID: input.SourceID})
	must(t, err)
	if replacement.Claim == nil || replacement.Claim.RequestID != requests[0].ID {
		t.Fatal("second intent did not replace the expired singleton")
	}
	again, err := claims.DispatchReference(f.ctx, node, input)
	must(t, err)
	if again.Claim != nil {
		t.Fatal("displaced request was automatically run again")
	}
}

func TestReferenceDispatchUnknownCommitAndLegacyHistory(t *testing.T) {
	t.Run("lost first reservation", func(t *testing.T) {
		f, clean, node, binding := claimFixture(t)
		prepareClaim(t, f, expectedPlanKey)
		store, observed := lostReferenceCommitStore(t, f.referenceRequestFixture)
		faulty, err := referencepg.NewReferenceClaimer(store, submissionpg.LockReferenceNode, node.ID, f.request.Specification.Identity.SourceLockSHA256)
		must(t, err)
		input := problemreference.DispatchInput{SourceID: binding.SourceID}
		got, err := faulty.DispatchReference(f.ctx, node, input)
		if got.Claim != nil || got.After != "" || !errors.Is(err, problemreference.ErrCommitUnknown) {
			t.Fatal("lost write was acknowledged or skipped", err)
		}
		select {
		case committed := <-observed:
			if !committed {
				t.Fatal("did not lose the real COMMIT reply")
			}
		case <-f.ctx.Done():
			t.Fatal("COMMIT loss not observed")
		}
		_, err = f.a.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()-interval '1 second'`)
		must(t, err)
		got, err = clean.DispatchReference(f.ctx, node, input)
		must(t, err)
		if got.Claim != nil {
			t.Fatal("unknown committed reservation could be selected again")
		}
	})
	t.Run("legacy plan", func(t *testing.T) {
		f, claims, node, binding := claimFixture(t)
		_, err := f.provider.DownTo(f.ctx, 12)
		must(t, err)
		prepareClaim(t, f, expectedPlanKey)
		_, err = f.provider.Up(f.ctx)
		must(t, err)
		input := problemreference.DispatchInput{SourceID: binding.SourceID}
		got, err := claims.DispatchReference(f.ctx, node, input)
		must(t, err)
		if got.Claim != nil || got.After != binding.RequestID {
			t.Fatal("legacy plan could have run automatically")
		}
		_, err = claims.ClaimReference(f.ctx, node, binding)
		must(t, err)
		var quarantined bool
		must(t, f.a.QueryRow(f.ctx, `SELECT first_claim_at IS NULL FROM problem_reference_dispatch_history WHERE request_id=$1`, binding.RequestID).Scan(&quarantined))
		if !quarantined {
			t.Fatal("manual claim erased original legacy uncertainty")
		}
	})
}

func TestReferenceDispatchViaDedicatedMTLS(t *testing.T) {
	f, claims, node, binding := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	material, err := problemreference.NewReferenceMaterials(claims, f.materials)
	must(t, err)
	ca := testpki.New(t)
	cert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Minute))
	server := httptest.NewUnstartedServer(workerapi.WithReferenceDispatch(http.NotFoundHandler(), claims, claims, claims, claims, ca.Roots, material))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots, Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()
	for _, namespace := range []string{"worker", "reference-worker"} {
		credential, _, _ := ca.Issue(t, "spiffe://onlinejudge/"+namespace+"/"+node.ID+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
		client, err := workerclient.NewAcquirer(server.URL, ca.Roots, credential)
		must(t, err)
		t.Cleanup(client.Close)
		got, err := client.DispatchReference(f.ctx, problemreference.DispatchInput{SourceID: binding.SourceID})
		if namespace == "worker" {
			if got.Claim != nil || !errors.Is(err, submission.ErrNode) {
				t.Fatal("ordinary node reached dispatch", err)
			}
			continue
		}
		must(t, err)
		if got.Claim == nil || !got.Claim.ValidFor(binding) {
			t.Fatal("specialized node did not receive the frozen binding")
		}
		must(t, client.RenewReference(f.ctx, *got.Claim))
		must(t, client.ReportReference(f.ctx, workerReportForClaim(t, f, *got.Claim, "AC")))
		status, err := f.store.Receipt(f.ctx, f.actor, expectedPlanKey)
		must(t, err)
		if status.State != problemreference.Reported {
			t.Fatal("dispatched plan did not accept its bound report")
		}
	}
}
