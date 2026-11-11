//go:build integration

package sessionprotocol_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

func testEntryReviewReturn(t *testing.T, x, y *loginEndpoint, receipt problemreference.ReviewReceipt,
	e problemreference.ReviewEvidence, reviewerCookies, authorCookies []*http.Cookie, reviewer, owner string) {
	t.Helper()
	const originalKey = "entry-return-original-key"
	const reason = "请补充边界情况的说明。"
	reviewerPath := "/api/v1/problem-review/" + receipt.ID + "/return"
	authorPath := "/api/v1/problem-draft-management/review-submissions/" + receipt.RequestKey + "/return"
	i := problemreference.ReturnIntent{RequestKey: originalKey, AssignmentRevision: e.AssignmentRevision,
		ImportRevision: e.ImportRevision, EvidenceSHA256: e.EvidenceSHA256, Reason: reason}
	raw, err := json.Marshal(i)
	must(t, err)
	post := func(endpoint *loginEndpoint, body string, cookies []*http.Cookie, expected string) loginResponse {
		return loginCall(endpoint, reviewerPath, body, cookies, func(r *http.Request) {
			r.Header.Set("X-OJ-Expected-Subject", expected)
		})
	}
	get := func(endpoint *loginEndpoint, path string, cookies []*http.Cookie, expected string) loginResponse {
		return loginCall(endpoint, path, "", cookies, func(r *http.Request) {
			r.Method = http.MethodGet
			r.Header.Set("X-OJ-Expected-Subject", expected)
		})
	}
	loginStatus(t, post(y, string(raw), reviewerCookies, reviewer), 404)
	loginStatus(t, post(x, string(raw), nil, reviewer), 401)
	loginStatus(t, post(x, string(raw), reviewerCookies, owner), 409)
	loginStatus(t, post(x, string(raw), authorCookies, owner), 404)
	loginStatus(t, post(x, `{"reason":"duplicate",`+string(raw[1:]), reviewerCookies, reviewer), 400)
	loginStatus(t, get(y, authorPath, authorCookies, owner), 404)
	created := post(x, string(raw), reviewerCookies, reviewer)
	loginStatus(t, created, 200)
	var got problemreference.ReviewReturnReceipt
	must(t, json.Unmarshal(created.body, &got))
	if got.ReviewID != receipt.ID || got.ReturnIntent != i || got.ReturnedAt.IsZero() ||
		strings.Contains(string(created.body), reviewer) || strings.Contains(string(created.body), owner) {
		t.Fatal("return receipt drifted or exposed identity")
	}
	for _, r := range []loginResponse{post(x, string(raw), reviewerCookies, reviewer),
		get(x, reviewerPath+"/"+originalKey, reviewerCookies, reviewer)} {
		loginStatus(t, r, 200)
		if !bytes.Equal(r.body, created.body) {
			t.Fatal("retry or original-key read changed the return receipt")
		}
	}
	loginStatus(t, get(x, reviewerPath+"/"+originalKey, authorCookies, owner), 404)
	loginStatus(t, get(y, authorPath, reviewerCookies, reviewer), 403)
	feedback := get(y, authorPath, authorCookies, owner)
	loginStatus(t, feedback, 200)
	var author problemreference.ReviewReturnStatus
	must(t, json.Unmarshal(feedback.body, &author))
	if author.ReviewID != receipt.ID || author.ImportRevision != i.ImportRevision || author.Reason != reason ||
		strings.Contains(string(feedback.body), originalKey) || strings.Contains(string(feedback.body), reviewer) {
		t.Fatal("author feedback changed the decision or disclosed the reviewer key")
	}
	newIntake := receipt.ReviewIntent
	newIntake.RequestKey = "new-key-for-returned-material"
	body, err := json.Marshal(newIntake)
	must(t, err)
	rejected := loginCall(y, "/api/v1/problem-draft-management/review-submissions", string(body), authorCookies, func(r *http.Request) {
		r.Header.Set("X-OJ-Expected-Subject", owner)
	})
	loginStatus(t, rejected, 409)
	if !bytes.Contains(rejected.body, []byte("REVIEW_MATERIALS_RETURNED")) {
		t.Fatal("new key was not rejected for returned materials")
	}
}
