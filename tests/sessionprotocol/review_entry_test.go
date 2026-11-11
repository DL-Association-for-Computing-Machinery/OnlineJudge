//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
)

func reviewEvidenceGET(e *loginEndpoint, id string, cookies []*http.Cookie, subject string) loginResponse {
	return reviewReadGET(e, id+"/evidence", cookies, subject)
}

func reviewMaterialsGET(e *loginEndpoint, id string, cookies []*http.Cookie, subject string) loginResponse {
	return reviewReadGET(e, id+"/materials", cookies, subject)
}

func reviewReadGET(e *loginEndpoint, tail string, cookies []*http.Cookie, subject string) loginResponse {
	return loginCall(e, "/api/v1/problem-review/"+tail, "", cookies, func(r *http.Request) {
		r.Method = http.MethodGet
		r.Header.Set("X-OJ-Expected-Subject", subject)
	})
}

func testEntryReviewAccess(t *testing.T, f *fixture, x, y *loginEndpoint, access *identitypg.ProblemAccess, authorCookies []*http.Cookie, owner, draftID, referenceID, root, namespace string) string {
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
	loginStatus(t, reviewMaterialsGET(x, receipt.ID, nil, reviewer.ID), 401)
	loginStatus(t, reviewMaterialsGET(x, receipt.ID, cookies, owner), 409)
	loginStatus(t, reviewMaterialsGET(x, receipt.ID, cookies, reviewer.ID), 403)
	comparison := func(endpoint *loginEndpoint, id string, c []*http.Cookie, subject string) loginResponse {
		return reviewReadGET(endpoint, id+"/comparison", c, subject)
	}
	eligibility := func(endpoint *loginEndpoint, id string, c []*http.Cookie, subject string) loginResponse {
		return reviewReadGET(endpoint, id+"/eligibility", c, subject)
	}
	loginStatus(t, comparison(x, receipt.ID, nil, reviewer.ID), 401)
	loginStatus(t, comparison(x, receipt.ID, cookies, owner), 409)
	loginStatus(t, comparison(x, receipt.ID, cookies, reviewer.ID), 403)
	loginStatus(t, eligibility(x, receipt.ID, nil, reviewer.ID), 401)
	loginStatus(t, eligibility(x, receipt.ID, cookies, owner), 409)
	loginStatus(t, eligibility(x, receipt.ID, cookies, reviewer.ID), 403)
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
	loginStatus(t, reviewMaterialsGET(y, receipt.ID, cookies, reviewer.ID), 404)
	loginStatus(t, comparison(y, receipt.ID, cookies, reviewer.ID), 404)
	loginStatus(t, eligibility(y, receipt.ID, cookies, reviewer.ID), 404)
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
	assertEligibility := func(result loginResponse, returned, confirmed bool) {
		t.Helper()
		loginStatus(t, result, 200)
		var fields map[string]json.RawMessage
		must(t, json.Unmarshal(result.body, &fields))
		var view problemreference.ReviewEligibility
		must(t, json.Unmarshal(result.body, &view))
		if len(fields) != 4 || string(fields["evidence"]) != string(assigned.body) || string(fields["eligible"]) != "false" ||
			view.PolicyVersion != "oj.review-approval.v4" || view.IndependentCorrectness != nil || view.WrongSolution != nil ||
			!slices.Contains(view.Blockers, "independent_correctness_missing") ||
			view.InputValidator != nil || !slices.Contains(view.Blockers, "input_validator_missing") ||
			slices.Contains(view.Blockers, "solution_confirmation_missing") == confirmed || slices.Contains(view.Blockers, "formal_failed") ||
			slices.Contains(view.Blockers, "materials_returned") != returned {
			t.Fatal("eligibility lost the accepted evidence or incorrectly treated formal AC/return status")
		}
	}
	assertEligibility(eligibility(x, receipt.ID, cookies, reviewer.ID), false, false)
	loginStatus(t, eligibility(y, receipt.ID, authorCookies, owner), 404)
	loginStatus(t, eligibility(y, referenceID, cookies, reviewer.ID), 404)
	loginStatus(t, reviewReadGET(y, receipt.ID+"/eligibility?policy=older", cookies, reviewer.ID), 400)
	var solutionHash string
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT convert_from(plan_encoding,'UTF8')::jsonb #>> '{source,sha256}'
		FROM problem_reference_expected_plans WHERE request_id=$1`, referenceID).Scan(&solutionHash))
	testEntryAuthorSolution(t, x, y, receipt, got, authorCookies, cookies, owner, reviewer.ID,
		func() func() { return corruptReviewSource(t, root, solutionHash) })
	assertEligibility(eligibility(y, receipt.ID, cookies, reviewer.ID), false, true)
	for _, endpoint := range []*loginEndpoint{x, y} {
		result := comparison(endpoint, receipt.ID, cookies, reviewer.ID)
		loginStatus(t, result, 200)
		var body map[string]json.RawMessage
		must(t, json.Unmarshal(result.body, &body))
		if len(body) != 2 || string(body["comparison"]) != "null" || string(body["evidence"]) != string(assigned.body) {
			t.Fatal("formal-only evidence did not return the same summary and explicit null comparison")
		}
	}
	loginStatus(t, comparison(x, draftID, cookies, reviewer.ID), 404)
	loginStatus(t, comparison(x, receipt.ID, authorCookies, owner), 404)
	loginStatus(t, reviewReadGET(y, receipt.ID+"/comparison?hash="+got.EvidenceSHA256, cookies, reviewer.ID), 400)
	loginStatus(t, reviewReadGET(y, receipt.ID+"/%63omparison", cookies, reviewer.ID), 400)
	loginStatus(t, loginCall(y, "/api/v1/problem-review/"+receipt.ID+"/comparison", "{}", cookies, func(r *http.Request) {
		r.Method = http.MethodGet
		r.Header.Set("X-OJ-Expected-Subject", reviewer.ID)
	}), 400)
	material := reviewMaterialsGET(y, receipt.ID, cookies, reviewer.ID)
	loginStatus(t, material, 200)
	assertReviewMaterials(t, material, receipt, owner, reviewer.ID)
	t.Run("corrupt_frozen_source", func(t *testing.T) {
		defer corruptReviewSource(t, root, solutionHash)()
		failed := reviewMaterialsGET(y, receipt.ID, cookies, reviewer.ID)
		loginStatus(t, failed, 503)
		if strings.Contains(string(failed.body), "Synthetic A + B") || strings.Contains(string(failed.body), "standard_solution") {
			t.Fatal("corrupt source returned partial private material")
		}
	})
	loginStatus(t, reviewMaterialsGET(x, draftID, cookies, reviewer.ID), 404)
	loginStatus(t, reviewMaterialsGET(x, referenceID, cookies, reviewer.ID), 404)
	loginStatus(t, reviewMaterialsGET(x, receipt.ID, authorCookies, owner), 404)
	loginStatus(t, reviewReadGET(y, receipt.ID+"/materials?hash="+got.EvidenceSHA256, cookies, reviewer.ID), 400)
	loginStatus(t, reviewReadGET(y, receipt.ID+"/materials/"+got.EvidenceSHA256, cookies, reviewer.ID), 404)
	testEntryReviewReturn(t, x, y, receipt, got, cookies, authorCookies, reviewer.ID, owner)
	assertEligibility(eligibility(y, receipt.ID, cookies, reviewer.ID), true, true)
	grant(reviewer.ID, 1, false)
	loginStatus(t, reviewEvidenceGET(x, receipt.ID, cookies, reviewer.ID), 403)
	loginStatus(t, reviewMaterialsGET(x, receipt.ID, cookies, reviewer.ID), 403)
	loginStatus(t, comparison(x, receipt.ID, cookies, reviewer.ID), 403)
	loginStatus(t, eligibility(x, receipt.ID, cookies, reviewer.ID), 403)
	grant(reviewer.ID, 2, true)
	loginStatus(t, reviewEvidenceGET(y, receipt.ID, cookies, reviewer.ID), 404)
	loginStatus(t, reviewMaterialsGET(y, receipt.ID, cookies, reviewer.ID), 404)
	loginStatus(t, comparison(y, receipt.ID, cookies, reviewer.ID), 404)
	loginStatus(t, eligibility(y, receipt.ID, cookies, reviewer.ID), 404)
	testReviewMaterialRaces(t, f, access, store, root, namespace, reviewer.ID, credential, receipt)
	return receipt.ID
}

func assertReviewMaterials(t *testing.T, material loginResponse, receipt problemreference.ReviewReceipt, owner, reviewer string) {
	t.Helper()
	var got problemreference.ReviewMaterials
	must(t, json.Unmarshal(material.body, &got))
	if got.Evidence.ReviewID != receipt.ID || got.Evidence.ImportRevision != receipt.ImportRevision ||
		got.Statement.DraftID != receipt.DraftID || got.Statement.Version != receipt.ContentVersion ||
		got.Statement.Markdown != "# Synthetic A + B\n" || len(got.Statement.Examples) != 1 ||
		got.Statement.Examples[0].Input != "1 2\n" || got.Statement.Examples[0].Output != "3\n" ||
		got.StandardSolution.Source != "synthetic A+B material; not executed by this test\n" ||
		len(got.StandardSolution.Dependencies) != 0 {
		t.Fatal("review materials did not match exact frozen bytes and identity")
	}
	for _, denied := range []string{owner, reviewer, "retired-fixture", "original/source/", "correct.cpp", "request_key", "token_sha256", "references"} {
		if strings.Contains(string(material.body), denied) {
			t.Fatal("materials exposed private authority, internal paths or raw transport fields")
		}
	}
}
