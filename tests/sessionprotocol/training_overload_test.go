//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func TestTrainingOverloadRecovery(t *testing.T) {
	f := setup(t)
	const version, source = "synthetic-p6-training-overload", "int main() {}\n"
	first, err := submissionpg.New(f.pools[0], version, []byte(`{"fixture":"p6-training-overload"}`))
	must(t, err)
	second, err := submissionpg.New(f.pools[1], version, []byte(`{"fixture":"p6-training-overload"}`))
	must(t, err)
	must(t, first.RegisterFixture(f.ctx))
	must(t, submissionpg.ConfigureBacklog(f.ctx, f.pools[0], submissionpg.BacklogLimits{Global: 2, Subject: 1}))
	node := submission.NodeIdentity{ID: "training-overload-fixture", CredentialVersion: 1}
	_, err = f.pools[0].Exec(f.ctx, "INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)", node.ID)
	must(t, err)

	c := runtimeConfig(t, f, namespace(t), true)
	c.TrainingHistory, c.TrainingSubmissionDetail = true, true
	ca := testpki.New(t)
	proc := entryStart(t, entryBinary(t), entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	api := entryClient(t, proc, ca)
	login := loginManager(t, f.store(t, c.Namespace, 1, protocol.OrdinaryPolicy()), time.Minute, loginKey(t))
	type account struct {
		actor   identity.TrainingActor
		cookies []*http.Cookie
	}
	var accounts [4]account
	for i := range accounts {
		p := f.subject(t, false)
		setHistoryEmailVerified(t, f, p.ID, true)
		pending := beginLogin(t, api, p)
		credential := completedCredential(t, loginCall(api, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
		view, err := login.Read(f.ctx, credential)
		must(t, err)
		if view.SubjectID != p.ID {
			t.Fatal("HTTPS session resolved the wrong training account")
		}
		accounts[i] = account{identity.TrainingActor{SubjectID: view.SubjectID, SessionVersion: identity.SessionVersion{
			AuthGeneration: view.AuthGeneration, RecoveryID: view.RecoveryID}},
			[]*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}}}
	}

	counts := func(t *testing.T, accepted, unfinished int) {
		t.Helper()
		var total, pending int
		must(t, f.pools[1].QueryRow(f.ctx, `SELECT count(*), count(*) FILTER (WHERE j.state IN ('queued', 'running'))
			FROM submissions s JOIN judge_tasks j ON j.submission_id=s.id`).Scan(&total, &pending))
		if total != accepted || pending != unfinished {
			t.Fatalf("ledger accepted=%d pending=%d; want %d and %d", total, pending, accepted, unfinished)
		}
	}
	readTerminal := func(t *testing.T, i int, id string) {
		t.Helper()
		a := accounts[i]
		response := historyGET(api, "/"+id, a.actor.SubjectID, a.cookies)
		loginStatus(t, response, http.StatusOK)
		var detail submission.Detail
		must(t, json.Unmarshal(response.body, &detail))
		if detail.ID != id || detail.MaterialVersion != version || detail.State != "finished" ||
			detail.Verdict == nil || *detail.Verdict != "AC" || detail.Source != source {
			t.Fatal("HTTPS readback lost terminal state or pinned submission data")
		}
	}
	for round := 1; round <= 3; round++ {
		if !t.Run(fmt.Sprintf("round-%d", round), func(t *testing.T) {
			base := (round - 1) * len(accounts)
			counts(t, base, 0)
			var keys, ids [4]string
			for i := range keys {
				keys[i] = fmt.Sprintf("synthetic-p6-overload-%d-%d", round, i)
			}
			accept := func(i int) {
				t.Helper()
				r, created, err := first.AcceptTraining(f.ctx, accounts[i].actor, keys[i], "gnu++17", source)
				must(t, err)
				if !created || r.State != "queued" || r.MaterialVersion != version {
					t.Fatal("expected a new pinned submission")
				}
				ids[i] = r.ID
			}
			report := func(lease submission.Lease) {
				t.Helper()
				must(t, second.Report(f.ctx, node, lease.Token, submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("4", 64)}))
			}
			accept(0)
			accept(1)
			lease, err := first.Claim(f.ctx, node)
			must(t, err)
			// Prove the global limit includes one running and one queued task.
			var running, queued int
			must(t, f.pools[1].QueryRow(f.ctx, `SELECT count(*) FILTER (WHERE state='running'), count(*) FILTER (WHERE state='queued')
				FROM judge_tasks`).Scan(&running, &queued))
			if running != 1 || queued != 1 {
				t.Fatal("fixture did not saturate the queue with one running and one queued task")
			}
			counts(t, base+2, 2)
			if _, created, err := second.AcceptTraining(f.ctx, accounts[2].actor, keys[2], "gnu++17", source); created || !errors.Is(err, submission.ErrBacklog) {
				t.Fatalf("third subject was not rejected at the global limit: %v", err)
			}
			_, err = submissionpg.ReadTrainingReceipt(f.ctx, f.pools[1], accounts[2].actor, keys[2])
			wantError(t, err, submission.ErrNotFound)
			r, created, err := second.AcceptTraining(f.ctx, accounts[0].actor, keys[0], "gnu++17", source)
			must(t, err)
			if created || r.ID != ids[0] {
				t.Fatal("same-key replay at capacity changed the original receipt")
			}
			counts(t, base+2, 2)
			stopped := time.Now()
			report(lease)
			lease, err = first.Claim(f.ctx, node)
			must(t, err)
			report(lease)
			counts(t, base+2, 0)
			drained := time.Since(stopped)
			accept(2)
			accept(3)
			recovered := time.Since(stopped)
			if recovered > 5*time.Minute {
				t.Fatal("fresh admission did not recover within five minutes")
			}
			counts(t, base+4, 2)
			for range 2 {
				lease, err := first.Claim(f.ctx, node)
				must(t, err)
				report(lease)
			}
			counts(t, base+4, 0)
			for i, id := range ids {
				readTerminal(t, i, id)
			}
			t.Logf("local training admission: full=2, rejected_without_receipt=1, replay=1, terminal_https=4, drain=%s, fresh_admission=%s",
				drained.Round(time.Microsecond), recovered.Round(time.Microsecond))
		}) {
			break
		}
	}
	proc.signal(t)
	proc.wait(t, true)
	if _, ok := proc.event("API stopped gracefully"); !ok {
		t.Fatal("API did not confirm graceful shutdown")
	}
	if !t.Failed() {
		t.Run("independent acceptance export", func(t *testing.T) {
			testTrainingAcceptanceExport(t, f, version)
		})
	}
}
