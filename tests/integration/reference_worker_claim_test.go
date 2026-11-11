//go:build integration && linux

package integration_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres/referencedb"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func claimFixture(t *testing.T) (expectedPlanFixture, *referencepg.ReferenceClaimer, submission.NodeIdentity, problemreference.ClaimInput) {
	t.Helper()
	f := newExpectedPlanFixture(t)
	claims, node, input := claimFixtureFor(t, f)
	return f, claims, node, input
}

func claimFixtureFor(t *testing.T, f expectedPlanFixture) (*referencepg.ReferenceClaimer, submission.NodeIdentity, problemreference.ClaimInput) {
	t.Helper()
	node := submission.NodeIdentity{ID: "reference-node", CredentialVersion: 1}
	_, err := f.a.Exec(f.ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,$2)`, node.ID, node.CredentialVersion)
	must(t, err)
	claims, err := referencepg.NewReferenceClaimer(f.store, submissionpg.LockReferenceNode, node.ID, f.request.Specification.Identity.SourceLockSHA256)
	must(t, err)
	input := problemreference.ClaimInput{RequestID: f.request.ID, SourceID: f.request.Specification.Identity.SourceID, PlanSHA256: f.want.SHA256}
	return claims, node, input
}

func prepareClaim(t *testing.T, f expectedPlanFixture, key string) {
	t.Helper()
	_, err := f.service(t, f.session, f.store).Prepare(f.ctx, f.credential, key)
	must(t, err)
}

// legacyReservation exercises the old schema's real SQL for historical Down
// tests. A current binary cannot start or claim against schema 11 or 12.
func legacyReservation(f expectedPlanFixture, node submission.NodeIdentity, input problemreference.ClaimInput) (problemreference.WorkerClaim, error) {
	tx, err := f.a.Begin(f.ctx)
	if err != nil {
		return problemreference.WorkerClaim{}, err
	}
	defer func() { _ = tx.Rollback(f.ctx) }()
	if err := submissionpg.LockReferenceNode(f.ctx, tx, node); err != nil {
		return problemreference.WorkerClaim{}, err
	}
	q := referencedb.New(tx)
	if _, err := q.ReferenceExpectedPlanForShare(f.ctx, input.RequestID); err != nil {
		return problemreference.WorkerClaim{}, err
	}
	token := strings.Repeat("a", 64)
	until, err := q.ReserveReferenceWorkerClaim(f.ctx, referencedb.ReserveReferenceWorkerClaimParams{
		RequestID: input.RequestID, NodeID: node.ID, CredentialVersion: node.CredentialVersion, LeaseToken: token, PlanSha256: input.PlanSHA256,
	})
	if err != nil {
		return problemreference.WorkerClaim{}, err
	}
	if err := tx.Commit(f.ctx); err != nil {
		return problemreference.WorkerClaim{}, err
	}
	return problemreference.WorkerClaim{RequestID: input.RequestID, SourceID: input.SourceID, PlanSHA256: input.PlanSHA256, Token: token, Until: until.Time}, nil
}

func noReferenceClaim(t *testing.T, got problemreference.WorkerClaim, err, want error) {
	t.Helper()
	if got != (problemreference.WorkerClaim{}) || !errors.Is(err, want) {
		t.Fatalf("expected zero claim and %v, got %v", want, err)
	}
}

func TestReferenceWorkerClaimRechecksAuthorityAndPlan(t *testing.T) {
	for _, tc := range []string{"unprepared", "cancelled", "regranted", "disabled author", "recovery", "profile", "source lock", "wrong digest", "plan identity", "wrong node", "old certificate", "disabled node"} {
		t.Run(tc, func(t *testing.T) {
			f, claims, node, input := claimFixture(t)
			if tc != "unprepared" {
				prepareClaim(t, f, expectedPlanKey)
			}
			want := problemreference.ErrClaimUnavailable
			switch tc {
			case "cancelled":
				_, err := f.store.Cancel(f.ctx, f.actor, expectedPlanKey)
				must(t, err)
			case "regranted":
				f.role(t, f.actor.SubjectID, 1, false)
				f.role(t, f.actor.SubjectID, 2, true)
			case "disabled author":
				_, err := f.identities.DisableCAS(f.ctx, f.actor.SubjectID, identity.Revision{CredentialVersion: 1, AuthGeneration: 1})
				must(t, err)
			case "recovery":
				epoch, err := f.identities.BeginRecovery(f.ctx)
				must(t, err)
				must(t, f.identities.FinishRecovery(f.ctx, epoch))
			case "profile":
				raw := []byte(`{"id":"retired"}`)
				_, err := f.a.Exec(f.ctx, `UPDATE problem_reference_requests SET profile_encoding=$1,profile_sha256=$2 WHERE id=$3`, raw, problemfixture.Hash(raw), input.RequestID)
				must(t, err)
			case "source lock":
				var err error
				claims, err = referencepg.NewReferenceClaimer(f.store, submissionpg.LockReferenceNode, node.ID, problemvalidation.SourceLockSHA256)
				must(t, err)
			case "wrong digest":
				input.PlanSHA256 = strings.Repeat("a", 64)
			case "plan identity":
				bad := bytes.Replace(f.want.Encoding, []byte(f.request.Specification.Identity.SourceLockSHA256), bytes.Repeat([]byte{'a'}, 64), 1)
				input.PlanSHA256 = problemfixture.Hash(bad)
				_, err := f.a.Exec(f.ctx, `UPDATE problem_reference_expected_plans SET plan_encoding=$1,plan_sha256=$2 WHERE request_id=$3`, bad, input.PlanSHA256, input.RequestID)
				must(t, err)
			case "wrong node":
				node.ID = "different-node"
				want = submission.ErrNode
			case "old certificate":
				_, err := f.a.Exec(f.ctx, `UPDATE worker_nodes SET credential_version=2 WHERE id=$1`, node.ID)
				must(t, err)
				want = submission.ErrNode
			case "disabled node":
				_, err := f.a.Exec(f.ctx, `UPDATE worker_nodes SET enabled=false WHERE id=$1`, node.ID)
				must(t, err)
				want = submission.ErrNode
			}
			got, err := claims.ClaimReference(f.ctx, node, input)
			noReferenceClaim(t, got, err, want)
			var count int
			must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_reference_worker_claim`).Scan(&count))
			if count != 0 {
				t.Fatal("denied claim mutated reservation")
			}
		})
	}
}

func TestReferenceWorkerClaimSharesOneReservation(t *testing.T) {
	f, first, node, input := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	f.limits(t, 2, 2)
	const secondKey = "reference-second-claim-key"
	secondRequest, err := f.store.Enqueue(f.ctx, f.actor, secondKey, f.draft.ID, f.catalog)
	must(t, err)
	prepareClaim(t, f, secondKey)
	peer := submission.NodeIdentity{ID: "reference-peer", CredentialVersion: 1}
	_, err = f.a.Exec(f.ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,$2)`, peer.ID, peer.CredentialVersion)
	must(t, err)
	second, err := referencepg.NewReferenceClaimer(f.store, submissionpg.LockReferenceNode, peer.ID, f.request.Specification.Identity.SourceLockSHA256)
	must(t, err)
	input2 := input
	input2.RequestID = secondRequest.ID
	type outcome struct {
		claim problemreference.WorkerClaim
		err   error
	}
	done := make(chan outcome, 2)
	start := make(chan struct{})
	for i, claimer := range []*referencepg.ReferenceClaimer{first, second} {
		go func() {
			<-start
			got, err := claimer.ClaimReference(f.ctx, []submission.NodeIdentity{node, peer}[i], []problemreference.ClaimInput{input, input2}[i])
			done <- outcome{got, err}
		}()
	}
	close(start)
	a, b := <-done, <-done
	if a.err != nil {
		a, b = b, a
	}
	must(t, a.err)
	noReferenceClaim(t, b.claim, b.err, problemreference.ErrClaimUnavailable)
	winner, winnerNode, winnerInput, winnerKey := first, node, input, expectedPlanKey
	if a.claim.RequestID == input2.RequestID {
		winner, winnerNode, winnerInput, winnerKey = second, peer, input2, secondKey
	}
	if !a.claim.ValidFor(winnerInput) {
		t.Fatal("successful claim did not bind chosen plan")
	}
	duplicate, err := winner.ClaimReference(f.ctx, winnerNode, winnerInput)
	noReferenceClaim(t, duplicate, err, problemreference.ErrClaimUnavailable)
	_, err = f.store.Cancel(f.ctx, f.actor, winnerKey)
	must(t, err)
	var until time.Time
	var token string
	must(t, f.a.QueryRow(f.ctx, `SELECT lease_until,lease_token FROM problem_reference_worker_claim WHERE singleton`).Scan(&until, &token))
	if !until.Equal(a.claim.Until) || token != a.claim.Token {
		t.Fatal("retry or cancellation changed the original lease")
	}
	_, err = f.a.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()-interval '1 second'`)
	must(t, err)
	if winnerInput == input {
		winner, winnerNode, winnerInput = second, peer, input2
	} else {
		winner, winnerNode, winnerInput = first, node, input
	}
	fresh, err := winner.ClaimReference(f.ctx, winnerNode, winnerInput)
	must(t, err)
	if !fresh.ValidFor(winnerInput) || fresh.Token == a.claim.Token {
		t.Fatal("expired reservation did not get a fresh token")
	}
	f.counts(t, 2, 1)
}

func TestReferenceWorkerClaimViaRealMTLS(t *testing.T) {
	f, claims, node, input := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	ca := testpki.New(t)
	serverCert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Minute))
	server := httptest.NewUnstartedServer(workerapi.WithReferenceClaims(workerapi.NewHandler(f.first, ca.Roots), claims, ca.Roots))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs: ca.Roots, Certificates: []tls.Certificate{serverCert}, SessionTicketsDisabled: true}
	server.StartTLS()
	defer server.Close()
	client := func(namespace string) (*workerclient.Acquirer, tls.Certificate) {
		cert, _, _ := ca.Issue(t, "spiffe://onlinejudge/"+namespace+"/"+node.ID+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
		c, err := workerclient.NewAcquirer(server.URL, ca.Roots, cert)
		must(t, err)
		t.Cleanup(c.Close)
		return c, cert
	}
	regular, ordinaryCert := client("worker")
	ref, refCert := client("reference-worker")
	got, err := regular.ClaimReference(f.ctx, input)
	noReferenceClaim(t, got, err, submission.ErrNode)
	if _, err := ref.Claim(f.ctx); !errors.Is(err, submission.ErrNode) {
		t.Fatal("reference certificate reached regular judgment protocol")
	}
	status := func(endpoint, path string, cert tls.Certificate) int {
		t.Helper()
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: ca.Roots, Certificates: []tls.Certificate{cert}}}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		resp, err := client.Post(endpoint+path, "application/json", strings.NewReader("{}"))
		must(t, err)
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	for _, route := range []string{"claim", "renew", "report", "assignment", "material"} {
		if status(server.URL, "/api/v1/worker/"+route, refCert) != http.StatusUnauthorized {
			t.Fatal("reference certificate could reach a regular Worker route", route)
		}
	}
	disabled := httptest.NewUnstartedServer(workerapi.WithReferenceClaims(workerapi.NewHandler(f.first, ca.Roots), nil, ca.Roots))
	disabled.TLS = server.TLS.Clone()
	disabled.StartTLS()
	defer disabled.Close()
	if status(disabled.URL, "/api/v1/worker/reference/claim", ordinaryCert) != http.StatusNotFound {
		t.Fatal("unconfigured controller exposed the reference claim route")
	}
	got, err = ref.ClaimReference(f.ctx, input)
	must(t, err)
	if !got.ValidFor(input) {
		t.Fatal("mTLS claim did not return reserved plan")
	}
	var requestID, nodeID, digest string
	must(t, f.a.QueryRow(f.ctx, `SELECT request_id,node_id,plan_sha256 FROM problem_reference_worker_claim WHERE singleton`).Scan(&requestID, &nodeID, &digest))
	if requestID != input.RequestID || nodeID != node.ID || digest != input.PlanSHA256 {
		t.Fatal("mTLS claim did not commit the expected binding")
	}
}

func TestReferenceClaimDown11ProtectsHistory(t *testing.T) {
	f, _, node, input := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	_, err := f.provider.DownTo(f.ctx, 10)
	must(t, err)
	if db.CheckSchema(f.ctx, f.a) == nil {
		t.Fatal("startup accepted schema 10")
	}
	_, err = f.provider.UpTo(f.ctx, 11)
	must(t, err)
	_, err = legacyReservation(f, node, input)
	must(t, err)
	_, err = f.a.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()-interval '1 second'`)
	must(t, err)
	if _, err := f.provider.DownTo(f.ctx, 10); err == nil || !strings.Contains(err.Error(), "cannot discard reference claim state") {
		t.Fatalf("rollback removed claim state: %v", err)
	}
	_, err = f.provider.Up(f.ctx)
	must(t, err)
	must(t, db.CheckSchema(f.ctx, f.a))
}
