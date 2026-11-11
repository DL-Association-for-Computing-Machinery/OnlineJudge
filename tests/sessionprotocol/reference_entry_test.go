//go:build integration

package sessionprotocol_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
)

const entryReferencePath = "/api/v1/problem-draft-management/reference-requests"

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
	var draftState string
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT validation_state FROM problem_drafts WHERE id=$1`, draft).Scan(&draftState))
	if draftState != "not-run" {
		t.Fatal("author request changed draft validation")
	}
}
