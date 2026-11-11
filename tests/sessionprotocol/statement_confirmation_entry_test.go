//go:build integration

package sessionprotocol_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

func testEntryStatementConfirmation(t *testing.T, x, y *loginEndpoint, e problemreference.ReviewEvidence,
	cookies, authorCookies []*http.Cookie, reviewer, owner string, corrupt func() func()) {
	t.Helper()
	leaf := e.ReviewID + "/statement-confirmation"
	i := problemreference.StatementConfirmationIntent{RequestKey: "entry-statement-confirmation",
		AssignmentRevision: e.AssignmentRevision, ImportRevision: e.ImportRevision, EvidenceSHA256: e.EvidenceSHA256,
		ChecklistVersion: problemreference.StatementChecklistVersion}
	raw, err := json.Marshal(i)
	must(t, err)
	post := func(endpoint *loginEndpoint, body string, c []*http.Cookie, subject string) loginResponse {
		return loginCall(endpoint, "/api/v1/problem-review/"+leaf, body, c, func(r *http.Request) {
			r.Header.Set("X-OJ-Expected-Subject", subject)
		})
	}
	loginStatus(t, post(y, string(raw), cookies, reviewer), 404)
	loginStatus(t, post(x, string(raw), nil, reviewer), 401)
	loginStatus(t, post(x, string(raw), authorCookies, owner), 404)
	loginStatus(t, post(x, string(raw), cookies, owner), 409)
	loginStatus(t, post(x, strings.Replace(string(raw), i.ChecklistVersion, "unknown", 1), cookies, reviewer), 400)
	loginStatus(t, post(x, string(raw[:len(raw)-1]), cookies, reviewer), 400)
	loginStatus(t, post(x, string(raw)+" {}", cookies, reviewer), 400)
	loginStatus(t, post(x, strings.Replace(string(raw), i.ImportRevision, strings.Repeat("f", 64), 1), cookies, reviewer), 409)
	func() {
		defer corrupt()()
		loginStatus(t, post(x, string(raw), cookies, reviewer), 503)
		loginStatus(t, reviewReadGET(x, leaf+"/"+i.RequestKey, cookies, reviewer), 404)
	}()
	created := post(x, string(raw), cookies, reviewer)
	loginStatus(t, created, 200)
	var got problemreference.StatementConfirmationReceipt
	must(t, json.Unmarshal(created.body, &got))
	if got.ReviewID != e.ReviewID || got.StatementConfirmationIntent != i || got.ConfirmedAt.IsZero() ||
		strings.Contains(string(created.body), reviewer) || strings.Contains(string(created.body), owner) {
		t.Fatal("confirmation receipt drifted or exposed private identity")
	}
	for _, replay := range []loginResponse{post(x, string(raw), cookies, reviewer),
		reviewReadGET(x, leaf+"/"+i.RequestKey, cookies, reviewer)} {
		loginStatus(t, replay, 200)
		if !bytes.Equal(replay.body, created.body) {
			t.Fatal("confirmation replay changed the receipt")
		}
	}
	loginStatus(t, reviewReadGET(x, leaf+"/"+i.RequestKey, authorCookies, owner), 404)
	view := reviewReadGET(y, e.ReviewID+"/eligibility", cookies, reviewer)
	loginStatus(t, view, 200)
	var eligibility problemreference.ReviewEligibility
	must(t, json.Unmarshal(view.body, &eligibility))
	if !eligibility.StatementSemanticsConfirmed || eligibility.Eligible || eligibility.StatementRendering != nil ||
		!slices.Contains(eligibility.Blockers, "statement_quality_missing") {
		t.Fatal("human confirmation incorrectly satisfied the missing rendering evidence")
	}
}
