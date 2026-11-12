//go:build integration

package sessionprotocol_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

func configureReferenceRestore(t *testing.T, f *fixture, c *sessionruntime.Config, authors ...string) {
	t.Helper()
	bundle := referenceEntryBundle(t)
	root := t.TempDir()
	lock := filepath.Join(t.TempDir(), "trusted-lock.json")
	must(t, os.WriteFile(lock, bundle.Lock, 0600))
	c.PrivateDrafts = &sessionruntime.PrivateDraftsConfig{MaterialRoot: root, BundleDirectory: bundle.Directory, SourceLockFile: lock,
		SourceIDs: []string{"library-checker:aplusb"}, Timeout: 3 * time.Second, MaxInFlight: 1, ReferenceRequests: true}
	must(t, referencepg.Configure(f.ctx, f.pools[0], problemreference.Limits{Global: 2, Owner: 1}))
	access, err := identitypg.NewProblemAccess(f.pools[0])
	must(t, err)
	for _, author := range authors {
		_, err := access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: author, Enabled: true,
			ExpectedRevision: 0, Operator: "entry-fixture", Reason: "synthetic reference recovery author"})
		must(t, err)
	}
}

type archivedReferenceRequest struct {
	id, owner, key, draft string
	receipt               []byte
}

func referenceRequestBody(key, draft string) string {
	return fmt.Sprintf(`{"request_key":%q,"draft_id":%q}`, key, draft)
}

func queueReferenceForRestore(t *testing.T, e *loginEndpoint, owner string, session protocol.Credential) archivedReferenceRequest {
	t.Helper()
	cookies := credentialCookies(session)
	imported := loginCall(e, "/api/v1/problem-drafts", `{"source_id":"library-checker:aplusb","request_key":"restore-reference-draft"}`, cookies, nil)
	loginStatus(t, imported, 200)
	var draft struct{ ID string }
	must(t, json.Unmarshal(imported.body, &draft))
	key := "restore-reference-" + owner
	created := authorReferenceCall(e, http.MethodPost, "", referenceRequestBody(key, draft.ID), cookies, owner)
	loginStatus(t, created, 200)
	var row struct{ ID, State string }
	must(t, json.Unmarshal(created.body, &row))
	if !identity.ValidID(draft.ID) || !identity.ValidID(row.ID) || row.State != "queued" {
		t.Fatal("archive fixture did not create a queued request")
	}
	return archivedReferenceRequest{id: row.ID, owner: owner, key: key, draft: draft.ID, receipt: created.body}
}

func (r archivedReferenceRequest) cancel(t *testing.T, e *loginEndpoint, session protocol.Credential) []byte {
	t.Helper()
	got := authorReferenceCall(e, http.MethodPost, "/"+r.key+"/cancel", "", credentialCookies(session), r.owner)
	loginStatus(t, got, 200)
	var row struct {
		ID, State   string
		CancelledAt *time.Time `json:"cancelled_at"`
	}
	must(t, json.Unmarshal(got.body, &row))
	if row.ID != r.id || row.State != "cancelled" || row.CancelledAt == nil {
		t.Fatal("cancellation did not return the original request and its terminal timestamp")
	}
	return got.body
}

func testRecoveredReferenceQueue(t *testing.T, pool *pgxpool.Pool, x, y *loginEndpoint, old, fresh protocol.Credential,
	archivedEpoch, openedEpoch string, original, disabled archivedReferenceRequest) {
	t.Helper()
	cookies := credentialCookies(fresh)
	path := "/" + original.key
	const newKey = "reissued-after-recovery"
	newBody := referenceRequestBody(newKey, original.draft)

	loginStatus(t, authorReferenceCall(y, http.MethodPost, path+"/cancel", "", credentialCookies(old), original.owner), 401)
	loginStatus(t, authorReferenceCall(y, http.MethodPost, "/"+disabled.key+"/cancel", "", cookies, original.owner), 404)
	restored := authorReferenceCall(y, http.MethodGet, path, "", cookies, original.owner)
	loginStatus(t, restored, 200)
	if !bytes.Equal(restored.body, original.receipt) {
		t.Fatal("recovered API did not return the exact archived queued receipt")
	}
	loginStatus(t, authorReferenceCall(x, http.MethodPost, "", newBody, cookies, original.owner), 429)

	cancelled := original.cancel(t, y, fresh)
	if replay := original.cancel(t, x, fresh); !bytes.Equal(replay, cancelled) {
		t.Fatal("another replica changed the cancellation receipt on replay")
	}
	retry := authorReferenceCall(x, http.MethodPost, "", referenceRequestBody(original.key, original.draft), cookies, original.owner)
	loginStatus(t, retry, 200)
	if !bytes.Equal(retry.body, cancelled) {
		t.Fatal("resending the old creation key reopened the cancelled request")
	}
	created := authorReferenceCall(x, http.MethodPost, "", newBody, cookies, original.owner)
	loginStatus(t, created, 200)
	var row struct{ ID, State string }
	must(t, json.Unmarshal(created.body, &row))
	if !identity.ValidID(row.ID) || row.ID == original.id || row.State != "queued" {
		t.Fatal("freed quota did not admit a distinct new request")
	}

	for _, want := range []struct{ id, owner, key, draft, epoch, state string }{
		{original.id, original.owner, original.key, original.draft, archivedEpoch, "cancelled"},
		{disabled.id, disabled.owner, disabled.key, disabled.draft, archivedEpoch, "queued"},
		{row.ID, original.owner, newKey, original.draft, openedEpoch, "queued"},
	} {
		var got = want
		must(t, pool.QueryRow(t.Context(), `SELECT owner_subject_id,request_key,draft_id,recovery_id,state
			FROM problem_reference_requests WHERE id=$1`, want.id).Scan(&got.owner, &got.key, &got.draft, &got.epoch, &got.state))
		if got != want {
			t.Fatal("recovered reference ledger did not retain old bindings and isolate the new epoch")
		}
	}
	var queued int
	must(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM problem_reference_requests WHERE state='queued'`).Scan(&queued))
	if queued != 2 {
		t.Fatal("queue did not retain exactly the disabled author's old request and the new request")
	}
	t.Log("fresh author cancelled the archived request across replicas and used the freed quota; disabled author's request stayed queued")
}
