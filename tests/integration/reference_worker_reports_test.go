//go:build integration && linux

package integration_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
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
	"github.com/jackc/pgx/v5/pgconn"
)

func acceptedReportFixture(t *testing.T, verdict string) (expectedPlanFixture, *referencepg.ReferenceClaimer, submission.NodeIdentity, problemreference.WorkerReportInput) {
	t.Helper()
	f, claims, node, selection := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	claim, err := claims.ClaimReference(f.ctx, node, selection)
	must(t, err)
	var plan problem.ReferencePlan
	must(t, json.Unmarshal(f.want.Encoding, &plan))
	profile := problemvalidation.PinnedProfile()
	r := problemvalidation.Report{Schema: "oj.reference-validation.v1", Status: "reference-validation-failed", Publication: "blocked", Verdict: verdict,
		Plan: plan, PlanSHA256: f.want.SHA256, Profile: profile, ProfileSHA256: profile.Digest(), CompileUsage: &execution.Usage{CPU: 1, Memory: 2},
		Slot: (workerhost.M0Slot{}).ContainerName(), ContainerID: strings.Repeat("a", 64), StartedAt: "2026-11-11T00:00:00Z"}
	for n, c := range plan.Cases {
		p := problemvalidation.Point{ID: c.ID, Verdict: "NOT_RUN"}
		if verdict == "AC" || (verdict != "CE" && n == 0) {
			p.Verdict, p.Usage = verdict, &execution.Usage{CPU: 3, Memory: 4}
		}
		r.Points = append(r.Points, p)
	}
	must(t, r.Finalize(nil, nil, nil))
	input, err := problemreference.FreezeWorkerReport(claim, r)
	must(t, err)
	return f, claims, node, input
}

func TestReferenceReportCannotCrossExpiryWhileWaitingForClaim(t *testing.T) {
	f, claims, node, input := acceptedReportFixture(t, "AC")
	tx, err := f.b.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()+interval '150 milliseconds'`)
	must(t, err)
	done := make(chan error, 1)
	go func() { done <- claims.ReportReference(f.ctx, node, input) }()
	waitBacklogBlocked(t, f.backlogFixture, tx.Conn().PgConn().PID(), f.appA)
	time.Sleep(200 * time.Millisecond)
	must(t, tx.Commit(f.ctx))
	if err := <-done; !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatal("expired first report accepted", err)
	}
	got, err := f.store.Receipt(f.ctx, f.actor, expectedPlanKey)
	must(t, err)
	if got.State != problemreference.Queued {
		t.Fatal("expiry changed the queued intent")
	}
}

func TestReferenceReportDown12WaitsAndPreservesConcurrentReport(t *testing.T) {
	f, claims, node, input := acceptedReportFixture(t, "AC")
	tx, err := f.b.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `LOCK TABLE problem_reference_worker_reports IN ACCESS EXCLUSIVE MODE`)
	must(t, err)
	report := make(chan error, 1)
	go func() { report <- claims.ReportReference(f.ctx, node, input) }()
	pid := waitBacklogBlocked(t, f.backlogFixture, tx.Conn().PgConn().PID(), f.appA)
	down := make(chan error, 1)
	go func() { _, err := f.provider.DownTo(f.ctx, 11); down <- err }()
	watch, cancel := context.WithTimeout(f.ctx, 2*time.Second)
	defer cancel()
	for {
		var blocked bool
		must(t, f.a.QueryRow(watch, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE $1::integer=ANY(pg_blocking_pids(pid))
			AND query LIKE '%LOCK TABLE problem_reference_requests, problem_reference_worker_reports%')`, pid).Scan(&blocked))
		if blocked {
			break
		}
		select {
		case err := <-down:
			t.Fatalf("Down12 did not wait: %v", err)
		case <-watch.Done():
			t.Fatal("Down12 lock wait not observed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	must(t, tx.Commit(f.ctx))
	must(t, <-report)
	var pe *pgconn.PgError
	if err := <-down; !errors.As(err, &pe) || pe.Code != "P0001" || !strings.Contains(pe.Message, "cannot discard accepted reference report state") {
		t.Fatal("Down12 did not preserve accepted report", err)
	}
	must(t, claims.ReportReference(f.ctx, node, input))
}

func TestReferenceReportCommitsTerminalStateAndExactReplay(t *testing.T) {
	f, claims, node, input := acceptedReportFixture(t, "WA")
	f.limits(t, 1, 1)
	start := make(chan struct{})
	done := make(chan error, 2)
	for range 2 {
		go func() { <-start; done <- claims.ReportReference(f.ctx, node, input) }()
	}
	close(start)
	must(t, <-done)
	must(t, <-done)
	var saved []byte
	var count int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_reference_worker_reports`).Scan(&count))
	must(t, f.a.QueryRow(f.ctx, `SELECT evidence FROM problem_reference_worker_reports WHERE request_id=$1`, input.RequestID).Scan(&saved))
	if count != 1 || !bytes.Equal(saved, input.Evidence) {
		t.Fatal("accepted evidence changed or duplicated")
	}
	got, err := f.store.Cancel(f.ctx, f.actor, expectedPlanKey)
	must(t, err)
	if got.State != problemreference.Reported || got.CancelledAt != nil {
		t.Fatal("cancel changed reported intent")
	}
	const nextKey = "report-follow-up-request"
	next, err := f.store.Enqueue(f.ctx, f.actor, nextKey, f.draft.ID, f.catalog)
	must(t, err)
	prepareClaim(t, f, nextKey)
	selection := input.ClaimInput
	selection.RequestID = next.ID
	replacement, err := claims.ClaimReference(f.ctx, node, selection)
	must(t, err)
	must(t, claims.ReportReference(f.ctx, node, input))
	var token string
	var until time.Time
	must(t, f.a.QueryRow(f.ctx, `SELECT lease_token,lease_until FROM problem_reference_worker_claim WHERE singleton`).Scan(&token, &until))
	if token != replacement.Token || !until.Equal(replacement.Until) {
		t.Fatal("historical replay changed a new reservation")
	}
	conflict := input
	e, err := input.DecodeEvidence()
	must(t, err)
	e.CompileUsage.CPU++
	conflict.Evidence, err = json.Marshal(e)
	must(t, err)
	if err := claims.ReportReference(f.ctx, node, conflict); !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatal("changed report acknowledged", err)
	}
	if _, err := f.provider.DownTo(f.ctx, 11); err == nil || !strings.Contains(err.Error(), "cannot discard accepted reference report state") {
		t.Fatal("rollback discarded accepted evidence", err)
	}
	var state string
	must(t, f.a.QueryRow(f.ctx, `SELECT validation_state FROM problem_drafts WHERE id=$1`, f.draft.ID).Scan(&state))
	if state != "not-run" {
		t.Fatal("reference report changed draft validation")
	}
}

func TestReferenceReportRejectsStaleFirstAcceptance(t *testing.T) {
	for _, kind := range []string{"expired", "token", "author", "cancelled", "node rotated", "wrong plan"} {
		t.Run(kind, func(t *testing.T) {
			f, claims, node, input := acceptedReportFixture(t, "AC")
			want := problemreference.ErrClaimUnavailable
			switch kind {
			case "expired":
				_, err := f.a.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()-interval '1 second'`)
				must(t, err)
			case "token":
				input.Token = strings.Repeat("f", 64)
			case "author":
				f.role(t, f.actor.SubjectID, 1, false)
				f.role(t, f.actor.SubjectID, 2, true)
			case "cancelled":
				_, err := f.store.Cancel(f.ctx, f.actor, expectedPlanKey)
				must(t, err)
			case "node rotated":
				_, err := f.a.Exec(f.ctx, `UPDATE worker_nodes SET credential_version=2 WHERE id=$1`, node.ID)
				must(t, err)
				want = submission.ErrNode
			case "wrong plan":
				input.PlanSHA256 = strings.Repeat("f", 64)
			}
			if err := claims.ReportReference(f.ctx, node, input); !errors.Is(err, want) {
				t.Fatal("stale first report not denied", err)
			}
			var count int
			must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM problem_reference_worker_reports`).Scan(&count))
			if count != 0 {
				t.Fatal("denied report wrote a receipt")
			}
		})
	}
}

func TestReferenceReportLostCommitReplaysWithoutRenewing(t *testing.T) {
	f, clean, node, input := acceptedReportFixture(t, "AC")
	store, observed := lostReferenceCommitStore(t, f.referenceRequestFixture)
	faulty, err := referencepg.NewReferenceClaimer(store, submissionpg.LockReferenceNode, node.ID, f.request.Specification.Identity.SourceLockSHA256)
	must(t, err)
	if err := faulty.ReportReference(f.ctx, node, input); !errors.Is(err, problemreference.ErrCommitUnknown) {
		t.Fatal("lost commit was acknowledged", err)
	}
	select {
	case committed := <-observed:
		if !committed {
			t.Fatal("no COMMIT lost")
		}
	case <-f.ctx.Done():
		t.Fatal("commit fixture timed out")
	}
	_, err = clean.RenewReference(f.ctx, node, input.MaterialInput)
	if !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatal("reported request could renew", err)
	}
	must(t, clean.ReportReference(f.ctx, node, input))
	f.role(t, f.actor.SubjectID, 1, false)
	if err := clean.ReportReference(f.ctx, node, input); !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatal("revoked authority received report acknowledgement", err)
	}
}

func TestReferenceReportViaDedicatedMTLS(t *testing.T) {
	f, claims, _, input := acceptedReportFixture(t, "AC")
	service, err := problemreference.NewReferenceMaterials(claims, f.materials)
	must(t, err)
	ca := testpki.New(t)
	cert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Minute))
	server := httptest.NewUnstartedServer(workerapi.WithReferenceExecution(http.NotFoundHandler(), claims, claims, ca.Roots, service, claims))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots, Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()
	for _, namespace := range []string{"worker", "reference-worker"} {
		node, _, _ := ca.Issue(t, "spiffe://onlinejudge/"+namespace+"/reference-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
		a, err := workerclient.NewAcquirer(server.URL, ca.Roots, node)
		must(t, err)
		err = a.ReportReference(f.ctx, input)
		a.Close()
		if namespace == "worker" {
			if !errors.Is(err, submission.ErrNode) {
				t.Fatal("ordinary worker crossed report boundary", err)
			}
		} else {
			must(t, err)
		}
	}
}
