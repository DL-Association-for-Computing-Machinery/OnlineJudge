//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
)

func reviewEvidenceGET(e *loginEndpoint, id string, cookies []*http.Cookie, subject string) loginResponse {
	return loginCall(e, "/api/v1/problem-review/"+id+"/evidence", "", cookies, func(r *http.Request) {
		r.Method = http.MethodGet
		r.Header.Set("X-OJ-Expected-Subject", subject)
	})
}

func testEntryReviewAccess(t *testing.T, f *fixture, x, y *loginEndpoint, access *identitypg.ProblemAccess, authorCookies []*http.Cookie, owner, draftID, referenceID string) string {
	t.Helper()
	var revision string
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT request_sha256 FROM problem_drafts WHERE id=$1`, draftID).Scan(&revision))
	intent := problemreference.ReviewIntent{RequestKey: "review-entry-original-key", DraftID: draftID,
		ImportRevision: revision, ReferenceRequestID: referenceID}
	raw, err := json.Marshal(intent)
	must(t, err)
	submit := loginCall(x, "/api/v1/problem-draft-management/review-submissions", string(raw), authorCookies, func(r *http.Request) {
		r.Header.Set("X-OJ-Expected-Subject", owner)
	})
	loginStatus(t, submit, 200)
	var receipt problemreference.ReviewReceipt
	must(t, json.Unmarshal(submit.body, &receipt))
	if !identity.ValidID(receipt.ID) || receipt.ReviewIntent != intent {
		t.Fatal("author intake returned a different review intent")
	}
	reviewer := f.subject(t, false)
	pending := beginLogin(t, x, reviewer)
	credential := completedCredential(t, loginCall(y, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
	cookies := []*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}}
	loginStatus(t, reviewEvidenceGET(x, receipt.ID, nil, reviewer.ID), 401)
	loginStatus(t, reviewEvidenceGET(x, receipt.ID, cookies, owner), 409)
	loginStatus(t, reviewEvidenceGET(x, receipt.ID, cookies, reviewer.ID), 403)
	grant := func(subject string, revision int64, enabled bool) {
		t.Helper()
		_, err := access.SetReviewerRole(f.ctx, identity.ProblemRoleChange{SubjectID: subject, ExpectedRevision: revision,
			Enabled: enabled, Operator: "entry-fixture", Reason: "synthetic reviewer boundary"})
		must(t, err)
	}
	grant(owner, 0, true)
	grant(reviewer.ID, 0, true)
	loginStatus(t, reviewEvidenceGET(y, receipt.ID, authorCookies, owner), 404)
	loginStatus(t, reviewEvidenceGET(y, receipt.ID, cookies, reviewer.ID), 404)
	store, err := referencepg.NewReviewStore(f.pools[0], access)
	must(t, err)
	_, err = store.SetAssignment(f.ctx, problemreference.ReviewAssignmentChange{ReviewID: receipt.ID,
		ReviewerSubjectID: reviewer.ID, ExpectedReviewerRevision: 1, Operator: "entry-fixture", Reason: "assign synthetic non-author"})
	must(t, err)
	assigned := reviewEvidenceGET(y, receipt.ID, cookies, reviewer.ID)
	loginStatus(t, assigned, 200)
	var got problemreference.ReviewEvidence
	must(t, json.Unmarshal(assigned.body, &got))
	if got.ReviewID != receipt.ID || got.DraftID != draftID || got.ImportRevision != revision ||
		got.ReferenceRequestID != referenceID || got.FormalVerdict != "AC" || got.ComparisonOutcome != "" ||
		strings.Contains(string(assigned.body), "retired-fixture") || strings.Contains(string(assigned.body), owner) ||
		strings.Contains(string(assigned.body), reviewer.ID) {
		t.Fatal("assigned response drifted or exposed private authority/transport fields")
	}
	loginStatus(t, reviewEvidenceGET(x, draftID, cookies, reviewer.ID), 404)
	loginStatus(t, reviewEvidenceGET(x, receipt.ID, authorCookies, owner), 404)
	grant(reviewer.ID, 1, false)
	loginStatus(t, reviewEvidenceGET(x, receipt.ID, cookies, reviewer.ID), 403)
	grant(reviewer.ID, 2, true)
	loginStatus(t, reviewEvidenceGET(y, receipt.ID, cookies, reviewer.ID), 404)
	return receipt.ID
}
