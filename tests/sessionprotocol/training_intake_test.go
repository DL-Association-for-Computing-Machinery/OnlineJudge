//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"errors"
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

func testTrainingIntake(t *testing.T, f *fixture, binary string) {
	const version = "synthetic-authenticated-training"
	const key = "synthetic-authenticated-training-key"
	const source = "int main() {}\n"
	const verdict = "AC"
	queue, err := submissionpg.New(f.pools[0], version, []byte(`{"training":"synthetic-session-only"}`))
	must(t, err)
	must(t, queue.RegisterFixture(f.ctx))
	must(t, submissionpg.ConfigureBacklog(f.ctx, f.pools[0], submissionpg.BacklogLimits{Global: 20, Subject: 2}))
	node := submission.NodeIdentity{ID: "training-session-fixture", CredentialVersion: 1}
	_, err = f.pools[0].Exec(f.ctx, "INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)", node.ID)
	must(t, err)

	owner, other := f.subject(t, false), f.subject(t, false)
	for _, p := range []identity.Personal{owner, other} {
		setHistoryEmailVerified(t, f, p.ID, true)
	}
	s := f.store(t, namespace(t), 0, protocol.OrdinaryPolicy())
	login := loginManager(t, s, time.Minute, loginKey(t))
	loginAPI := loginServer(t, login)
	authenticate := func(p identity.Personal) protocol.Credential {
		t.Helper()
		pending := beginLogin(t, loginAPI, p)
		return completedCredential(t, loginCall(loginAPI, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
	}
	credential := authenticate(owner)
	otherCredential := authenticate(other)
	view, err := login.Read(f.ctx, credential)
	must(t, err)
	if view.SubjectID != owner.ID {
		t.Fatal("session resolved a different training account")
	}
	actor := identity.TrainingActor{SubjectID: view.SubjectID,
		SessionVersion: identity.SessionVersion{AuthGeneration: view.AuthGeneration, RecoveryID: view.RecoveryID}}
	// Submit through the storage boundary using the session observation. The
	// shipping HTTP handler is not given a fixture resolver or a submit route.
	r, created, err := queue.AcceptTraining(f.ctx, actor, key, "gnu++17", source)
	must(t, err)
	if !created || r.State != "queued" || r.MaterialVersion != version {
		t.Fatal("ordinary session did not enqueue the pinned fixture")
	}
	recovered, err := submissionpg.ReadTrainingReceipt(f.ctx, f.pools[1], actor, key)
	must(t, err)
	if recovered.ID != r.ID {
		t.Fatal("receipt key did not recover the original submission")
	}
	lease, err := queue.Claim(f.ctx, node)
	must(t, err)
	if lease.SubmissionID != r.ID || lease.MaterialVersion != version {
		t.Fatal("queue leased a different immutable submission")
	}
	// Exercise lease result persistence with a controlled report. Actual
	// execution belongs to the separately isolated fixed-material suite.
	must(t, queue.Report(f.ctx, node, lease.Token, submission.Result{Verdict: verdict, EvidenceSHA256: strings.Repeat("4", 64)}))

	c := runtimeConfig(t, f, namespace(t), true)
	c.TrainingHistory, c.TrainingSubmissionDetail = true, true
	ca := testpki.New(t)
	p := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	api := entryClient(t, p, ca)
	ownerCookie := []*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}}
	otherCookie := []*http.Cookie{{Name: transport.CookieName, Value: otherCredential.Value()}}
	path := "/" + r.ID
	response := historyGET(api, path, owner.ID, ownerCookie)
	loginStatus(t, response, 200)
	var detail submission.Detail
	must(t, json.Unmarshal(response.body, &detail))
	if detail.ID != r.ID || detail.State != "finished" || detail.Verdict == nil || *detail.Verdict != verdict || detail.Source != source {
		t.Fatal("owner did not read the original source and terminal result")
	}
	loginStatus(t, historyGET(api, path, other.ID, otherCookie), 404)
	loginStatus(t, loginCall(api, entryTrainingHistory, "{}", ownerCookie, nil), 405)
	setHistoryEmailVerified(t, f, owner.ID, false)
	if _, err := submissionpg.ReadTrainingReceipt(f.ctx, f.pools[1], actor, key); !errors.Is(err, identity.ErrTrainingAccountRequired) {
		t.Fatalf("revoked training account could recover the original receipt: %v", err)
	}
	loginStatus(t, historyGET(api, path, owner.ID, ownerCookie), 403)
	p.signal(t)
	p.wait(t, true)
}
