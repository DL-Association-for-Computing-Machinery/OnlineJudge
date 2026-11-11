//go:build integration && linux

package integration_test

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestReferenceRenewalExtendsOnlyMatchingLiveClaim(t *testing.T) {
	f, claims, node, selection := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	claim, err := claims.ClaimReference(f.ctx, node, selection)
	must(t, err)
	input := problemreference.MaterialInput{ClaimInput: selection, Token: claim.Token}
	var before time.Time
	must(t, f.a.QueryRow(f.ctx, `UPDATE problem_reference_worker_claim
		SET lease_until=clock_timestamp()+interval '4 seconds' RETURNING lease_until`).Scan(&before))
	fresh, err := claims.RenewReference(f.ctx, node, input)
	must(t, err)
	if !fresh.ValidFor(selection) || fresh.Token != claim.Token || fresh.Until.Sub(before) < 20*time.Second {
		t.Fatal("renewal changed the claim identity or did not extend it")
	}
	var stored time.Time
	must(t, f.a.QueryRow(f.ctx, `SELECT lease_until FROM problem_reference_worker_claim WHERE singleton`).Scan(&stored))
	if !stored.Equal(fresh.Until) {
		t.Fatal("renewal acknowledgement does not match committed authority")
	}
	_, err = f.store.Cancel(f.ctx, f.actor, expectedPlanKey)
	must(t, err)
	got, err := claims.RenewReference(f.ctx, node, input)
	noReferenceClaim(t, got, err, problemreference.ErrClaimUnavailable)
	must(t, f.a.QueryRow(f.ctx, `SELECT lease_until FROM problem_reference_worker_claim WHERE singleton`).Scan(&stored))
	if !stored.Equal(fresh.Until) {
		t.Fatal("denied renewal extended cancelled authority")
	}
}

func TestReferenceRenewalQueryPlanUsesSingleton(t *testing.T) {
	f, claims, node, selection := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	claim, err := claims.ClaimReference(f.ctx, node, selection)
	must(t, err)
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	rows, err := tx.Query(f.ctx, `EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
		UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()+interval '30 seconds'
		WHERE singleton AND request_id=$1 AND node_id=$2 AND credential_version=$3
		AND lease_token=$4 AND plan_sha256=$5 AND lease_until>clock_timestamp() RETURNING lease_until`,
		selection.RequestID, node.ID, node.CredentialVersion, claim.Token, selection.PlanSHA256)
	must(t, err)
	sawClaimScan := false
	for rows.Next() {
		var line string
		must(t, rows.Scan(&line))
		// PostgreSQL may print bound lease tokens in Filter; retain only the scan
		// shape and buffer evidence, never the filter expression.
		if strings.Contains(line, "Scan") && strings.Contains(line, "problem_reference_worker_claim") {
			sawClaimScan = true
			t.Log(line)
		}
		if strings.Contains(line, "Buffers:") {
			t.Log(line)
		}
	}
	must(t, rows.Err())
	rows.Close()
	if !sawClaimScan {
		t.Fatal("renewal plan did not scan the singleton relation")
	}
}

func TestReferenceRenewalRejectsNodeAndDraftDrift(t *testing.T) {
	for _, kind := range []string{"node disabled", "credential rotated", "draft changed"} {
		t.Run(kind, func(t *testing.T) {
			f, claims, node, selection := claimFixture(t)
			prepareClaim(t, f, expectedPlanKey)
			claim, err := claims.ClaimReference(f.ctx, node, selection)
			must(t, err)
			want := submission.ErrNode
			switch kind {
			case "node disabled":
				_, err = f.b.Exec(f.ctx, `UPDATE worker_nodes SET enabled=false WHERE id=$1`, node.ID)
			case "credential rotated":
				_, err = f.b.Exec(f.ctx, `UPDATE worker_nodes SET credential_version=2 WHERE id=$1`, node.ID)
			case "draft changed":
				_, err = f.b.Exec(f.ctx, `UPDATE problem_drafts SET request_sha256=$1 WHERE id=$2`, strings.Repeat("f", 64), f.draft.ID)
				want = problemreference.ErrClaimUnavailable
			}
			must(t, err)
			got, err := claims.RenewReference(f.ctx, node, problemreference.MaterialInput{ClaimInput: selection, Token: claim.Token})
			noReferenceClaim(t, got, err, want)
		})
	}
}

func TestReferenceRenewalLostCommitReturnsNoGrant(t *testing.T) {
	f, clean, node, selection := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	claim, err := clean.ClaimReference(f.ctx, node, selection)
	must(t, err)
	input := problemreference.MaterialInput{ClaimInput: selection, Token: claim.Token}
	store, observed := lostReferenceCommitStore(t, f.referenceRequestFixture)
	faulty, err := referencepg.NewReferenceClaimer(store, submissionpg.LockReferenceNode, node.ID, f.request.Specification.Identity.SourceLockSHA256)
	must(t, err)
	got, err := faulty.RenewReference(f.ctx, node, input)
	noReferenceClaim(t, got, err, problemreference.ErrCommitUnknown)
	select {
	case committed := <-observed:
		if !committed {
			t.Fatal("fixture did not lose an actual COMMIT reply")
		}
	case <-f.ctx.Done():
		t.Fatal("commit-loss fixture timed out")
	}
	var until time.Time
	must(t, f.a.QueryRow(f.ctx, `SELECT lease_until FROM problem_reference_worker_claim WHERE singleton`).Scan(&until))
	if !until.After(claim.Until) {
		t.Fatal("renewal did not commit before acknowledgement loss")
	}
}

func TestReferenceRenewalViaDedicatedMTLS(t *testing.T) {
	f, claims, node, selection := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	claim, err := claims.ClaimReference(f.ctx, node, selection)
	must(t, err)
	materials, err := problemreference.NewReferenceMaterials(claims, f.materials)
	must(t, err)
	ca := testpki.New(t)
	serverCert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Minute))
	refCert, _, _ := ca.Issue(t, "spiffe://onlinejudge/reference-worker/reference-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
	judgeCert, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/reference-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
	for _, enabled := range []bool{false, true} {
		var handler http.Handler = workerapi.WithReferenceClaims(http.NotFoundHandler(), claims, ca.Roots, materials)
		if enabled {
			handler = workerapi.WithReferenceExecution(http.NotFoundHandler(), claims, claims, ca.Roots, materials)
		}
		s := httptest.NewUnstartedServer(handler)
		s.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots, Certificates: []tls.Certificate{serverCert}}
		s.StartTLS()
		t.Cleanup(s.Close)
		judge, err := workerclient.NewAcquirer(s.URL, ca.Roots, judgeCert)
		must(t, err)
		t.Cleanup(judge.Close)
		if err := judge.RenewReference(f.ctx, claim); !errors.Is(err, submission.ErrNode) {
			t.Fatal("ordinary judge certificate renewed reference", err)
		}
		ref, err := workerclient.NewAcquirer(s.URL, ca.Roots, refCert)
		must(t, err)
		t.Cleanup(ref.Close)
		if err := ref.RenewReference(f.ctx, claim); (err == nil) != enabled {
			t.Fatal("unexpected opt-in outcome", enabled, err)
		}
		if enabled {
			_, err := f.store.Cancel(f.ctx, f.actor, expectedPlanKey)
			must(t, err)
			if err := ref.RenewReference(f.ctx, claim); !errors.Is(err, problemreference.ErrClaimUnavailable) {
				t.Fatal("cancelled request renewed via mTLS", err)
			}
		}
	}
}

func TestReferenceRenewalCannotReviveClaimAfterLockWait(t *testing.T) {
	f, claims, node, selection := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	claim, err := claims.ClaimReference(f.ctx, node, selection)
	must(t, err)
	input := problemreference.MaterialInput{ClaimInput: selection, Token: claim.Token}
	tx, err := f.b.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `UPDATE problem_reference_worker_claim
		SET lease_until=clock_timestamp()+interval '150 milliseconds'`)
	must(t, err)
	type renewal struct {
		claim problemreference.WorkerClaim
		err   error
	}
	done := make(chan renewal, 1)
	go func() {
		got, err := claims.RenewReference(f.ctx, node, input)
		done <- renewal{got, err}
	}()
	waitBacklogBlocked(t, f.backlogFixture, tx.Conn().PgConn().PID(), f.appA)
	time.Sleep(200 * time.Millisecond)
	must(t, tx.Commit(f.ctx))
	out := <-done
	noReferenceClaim(t, out.claim, out.err, problemreference.ErrClaimUnavailable)
	replacement, err := claims.ClaimReference(f.ctx, node, selection)
	must(t, err)
	if replacement.Token == claim.Token {
		t.Fatal("reclaimed reservation reused an expired token")
	}
	got, err := claims.RenewReference(f.ctx, node, input)
	noReferenceClaim(t, got, err, problemreference.ErrClaimUnavailable)
	var token string
	must(t, f.a.QueryRow(f.ctx, `SELECT lease_token FROM problem_reference_worker_claim WHERE singleton`).Scan(&token))
	if token != replacement.Token {
		t.Fatal("expired renewal altered the replacement token")
	}
	input.Token = replacement.Token
	f.role(t, f.actor.SubjectID, 1, false)
	f.role(t, f.actor.SubjectID, 2, true)
	_, err = claims.RenewReference(f.ctx, node, input)
	if !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatal("regrant revived old author authority", err)
	}
}
