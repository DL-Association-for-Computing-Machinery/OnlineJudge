//go:build integration

package sessionprotocol_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

const entryReferencePath = "/api/v1/problem-draft-management/reference-requests"
const entryTerminalReferenceKey = "terminal-reference-fixture"

func authorReferenceCall(endpoint *loginEndpoint, method, path, body string, cookies []*http.Cookie, expected string) loginResponse {
	return loginCall(endpoint, entryReferencePath+path, body, cookies, func(r *http.Request) {
		r.Method = method
		r.Header.Set("X-OJ-Expected-Subject", expected)
	})
}

func testEntryAuthorReferences(t *testing.T, f *fixture, x, y *loginEndpoint, cookies []*http.Cookie, owner, draft, otherDraft string) {
	t.Helper()
	must(t, referencepg.Configure(f.ctx, f.pools[0], problemreference.Limits{Global: 4, Owner: 1}))
	const key = "author-reference-fixture"
	const path = "/" + key
	const next = "second-reference-fixture"
	body := func(key, id string) string { return fmt.Sprintf(`{"request_key":%q,"draft_id":%q}`, key, id) }
	loginStatus(t, authorReferenceCall(x, "POST", "", body(key, draft), cookies, ""), 400)
	loginStatus(t, authorReferenceCall(x, "POST", "", body(key, draft), cookies, otherDraft), 409)
	loginStatus(t, authorReferenceCall(x, "POST", "", fmt.Sprintf(`{"request_key":%q,"request_key":%q,"draft_id":%q}`, key, next, draft), cookies, owner), 400)
	loginStatus(t, authorReferenceCall(x, "POST", "", fmt.Sprintf(`{"request_key":%q,"draft_id":%q,"profile":"untrusted"}`, key, draft), cookies, owner), 400)
	loginStatus(t, loginCall(x, entryReferencePath, body(key, draft), cookies, func(r *http.Request) {
		r.Header.Set("X-OJ-Expected-Subject", owner)
		r.Header.Set("Origin", "https://other.invalid")
	}), 403)
	var count int
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT count(*) FROM problem_reference_requests WHERE owner_subject_id=$1`, owner).Scan(&count))
	if count != 0 {
		t.Fatal("rejected HTTP wrote a reference request")
	}
	created := authorReferenceCall(x, "POST", "", body(key, draft), cookies, owner)
	loginStatus(t, created, 200)
	var summary map[string]any
	must(t, json.Unmarshal(created.body, &summary))
	id, _ := summary["id"].(string)
	if len(summary) != 6 || !identity.ValidID(id) || summary["request_key"] != key || summary["draft_id"] != draft ||
		summary["state"] != "queued" || summary["cancelled_at"] != nil || summary["created_at"] == nil {
		t.Fatal("invalid or overbroad author summary")
	}
	for _, readback := range []loginResponse{
		authorReferenceCall(y, "GET", path, "", cookies, owner),
		authorReferenceCall(y, "POST", "", body(key, draft), cookies, owner),
	} {
		loginStatus(t, readback, 200)
		if !bytes.Equal(readback.body, created.body) {
			t.Fatal("cross-process original-key receipt changed")
		}
	}
	loginStatus(t, authorReferenceCall(y, "POST", "", body(key, otherDraft), cookies, owner), 409)
	loginStatus(t, authorReferenceCall(y, "POST", "", body(next, otherDraft), cookies, owner), 429)
	loginStatus(t, authorReferenceCall(y, "GET", path+"/report", "", cookies, owner), 404)
	loginStatus(t, authorReferenceCall(x, "POST", path+"/prepare", "", cookies, otherDraft), 409)
	before := authorReferenceCall(y, "GET", path+"/plan", "", cookies, owner)
	loginStatus(t, before, 200)
	if string(before.body) != `{"ready":false}` {
		t.Fatal("unprepared request returned ready")
	}
	loginStatus(t, authorReferenceCall(x, "POST", path+"/prepare", "", cookies, owner), 202)
	for deadline := time.Now().Add(4 * time.Second); ; {
		got := authorReferenceCall(y, "GET", path+"/plan", "", cookies, owner)
		loginStatus(t, got, 200)
		if string(got.body) == `{"ready":true}` {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real ordinary-session preparation never became durable")
		}
		time.Sleep(25 * time.Millisecond)
	}
	repeated := authorReferenceCall(x, "POST", path+"/prepare", "", cookies, owner)
	loginStatus(t, repeated, 200)
	if string(repeated.body) != `{"ready":true}` {
		t.Fatal("already durable plan did not recover by original key")
	}
	loginStatus(t, authorReferenceCall(y, "POST", path+"/cancel", "{}", cookies, owner), 400)
	loginStatus(t, authorReferenceCall(y, "GET", path+"/cancel", "", cookies, owner), 405)
	loginStatus(t, authorReferenceCall(y, "GET", path+"?unexpected=1", "", cookies, owner), 400)
	cancelled := authorReferenceCall(y, "POST", path+"/cancel", "", cookies, owner)
	loginStatus(t, cancelled, 200)
	must(t, json.Unmarshal(cancelled.body, &summary))
	if summary["id"] != id || summary["state"] != "cancelled" || summary["cancelled_at"] == nil {
		t.Fatal("cancel did not preserve the intent and original key")
	}
	loginStatus(t, authorReferenceCall(x, "POST", path+"/prepare", "", cookies, owner), 409)
	replay := authorReferenceCall(x, "POST", path+"/cancel", "", cookies, owner)
	loginStatus(t, replay, 200)
	if !bytes.Equal(replay.body, cancelled.body) {
		t.Fatal("replay changed cancellation receipt")
	}
	loginStatus(t, authorReferenceCall(y, "POST", "", body(next, otherDraft), cookies, owner), 200)
	loginStatus(t, authorReferenceCall(x, "POST", "/"+next+"/cancel", "", cookies, owner), 200)
	loginStatus(t, authorReferenceCall(y, "GET", path+"/report", "", cookies, owner), 404)
	testEntryReferenceResult(t, f, x, y, cookies, owner, draft, otherDraft)
	var draftState string
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT validation_state FROM problem_drafts WHERE id=$1`, draft).Scan(&draftState))
	if draftState != "not-run" {
		t.Fatal("author request changed draft validation")
	}
}

func testEntryReferenceResult(t *testing.T, f *fixture, x, y *loginEndpoint, cookies []*http.Cookie, owner, draft, otherSubject string) {
	t.Helper()
	const key = entryTerminalReferenceKey
	const path = "/" + key
	created := authorReferenceCall(x, "POST", "", fmt.Sprintf(`{"request_key":%q,"draft_id":%q}`, key, draft), cookies, owner)
	loginStatus(t, created, 200)
	var request struct{ ID string }
	must(t, json.Unmarshal(created.body, &request))
	loginStatus(t, authorReferenceCall(x, "POST", path+"/prepare", "", cookies, owner), 202)
	for deadline := time.Now().Add(4 * time.Second); ; {
		ready := authorReferenceCall(y, "GET", path+"/plan", "", cookies, owner)
		loginStatus(t, ready, 200)
		if string(ready.body) == `{"ready":true}` {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("terminal report fixture plan was not prepared")
		}
		time.Sleep(25 * time.Millisecond)
	}

	// Install canonical stopped evidence; the separate mTLS test exercises
	// the actual report acceptance path.
	tx, err := f.pools[0].Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	var plan problem.ReferencePlan
	var encoding []byte
	var planSHA, profileSHA string
	must(t, tx.QueryRow(f.ctx, `SELECT p.plan_encoding,p.plan_sha256,r.profile_sha256
		FROM problem_reference_expected_plans p JOIN problem_reference_requests r ON r.id=p.request_id
		WHERE r.id=$1`, request.ID).Scan(&encoding, &planSHA, &profileSHA))
	must(t, json.Unmarshal(encoding, &plan))
	e := problemreference.WorkerEvidence{Schema: "oj.reference-worker-evidence.v1", ProfileSHA256: profileSHA,
		Verdict: "AC", CompileUsage: &execution.Usage{}, StopConfirmed: true, Slot: "retired-fixture-slot",
		ContainerID: strings.Repeat("a", 64), StartedAt: "2026-11-11T00:00:00Z"}
	for _, p := range plan.Cases {
		e.Points = append(e.Points, problemvalidation.Point{ID: p.ID, Verdict: "AC", Usage: &execution.Usage{}})
	}
	raw, err := json.Marshal(e)
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_reference_worker_reports
		(request_id,node_id,credential_version,token_sha256,plan_sha256,evidence,evidence_sha256)
		VALUES ($1,'retired-fixture-node',1,$2,$3,$4,encode(sha256($4),'hex'))`, request.ID, strings.Repeat("b", 64), planSHA, raw)
	must(t, err)
	_, err = tx.Exec(f.ctx, `UPDATE problem_reference_requests SET state='reported' WHERE id=$1`, request.ID)
	must(t, err)
	must(t, tx.Commit(f.ctx))

	got := authorReferenceCall(y, "GET", path+"/report", "", cookies, owner)
	loginStatus(t, got, 200)
	want := fmt.Sprintf(`{"request_id":%q,"request_key":%q,"draft_id":%q,"content_version":%q,"formal":{"verdict":"AC","passed":%d,"total":%d},"comparison":null}`,
		request.ID, key, draft, plan.Version, len(plan.Cases), len(plan.Cases))
	if string(got.body) != want {
		t.Fatal("terminal author response did not match its exact public shape")
	}
	replay := authorReferenceCall(x, "GET", path+"/report", "", cookies, owner)
	loginStatus(t, replay, 200)
	if !bytes.Equal(got.body, replay.body) {
		t.Fatal("terminal result differed across API instances")
	}
	loginStatus(t, authorReferenceCall(y, "GET", path+"/report", "", cookies, otherSubject), 409)
	loginStatus(t, authorReferenceCall(y, "GET", path+"/report", "", nil, owner), 401)
}
