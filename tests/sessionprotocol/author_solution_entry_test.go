//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

func testEntryAuthorSolution(t *testing.T, x, y *loginEndpoint, receipt problemreference.ReviewReceipt,
	evidence problemreference.ReviewEvidence, authorCookies, reviewerCookies []*http.Cookie, owner, reviewer string,
	corrupt func() func(),
) {
	t.Helper()
	base := "/api/v1/problem-draft-management/review-submissions/" + receipt.RequestKey
	get := func(endpoint *loginEndpoint, path string, cookies []*http.Cookie, subject string) loginResponse {
		return loginCall(endpoint, base+path, "", cookies, func(r *http.Request) {
			r.Method = http.MethodGet
			r.Header.Set("X-OJ-Expected-Subject", subject)
		})
	}
	post := func(endpoint *loginEndpoint, input problemreference.SolutionBinding, cookies []*http.Cookie, subject string) loginResponse {
		t.Helper()
		raw, err := json.Marshal(input)
		must(t, err)
		return loginCall(endpoint, base+"/solution-confirmation", string(raw), cookies, func(r *http.Request) {
			r.Header.Set("X-OJ-Expected-Subject", subject)
		})
	}
	loginStatus(t, get(x, "/solution", nil, owner), 401)
	loginStatus(t, get(x, "/solution", authorCookies, reviewer), 409)
	loginStatus(t, get(y, "/solution?path=solution", authorCookies, owner), 400)
	loginStatus(t, get(y, "/solution-confirmation", authorCookies, owner), 404)
	preview := get(x, "/solution", authorCookies, owner)
	loginStatus(t, preview, 200)
	var view problemreference.AuthorSolution
	must(t, json.Unmarshal(preview.body, &view))
	var fields map[string]json.RawMessage
	must(t, json.Unmarshal(preview.body, &fields))
	if len(fields) != 3 || view.ImportRevision != receipt.ImportRevision || view.FormalPlanSHA256 != evidence.FormalPlanSHA256 ||
		view.StandardSolution.Source != "synthetic A+B material; not executed by this test\n" ||
		len(view.StandardSolution.Dependencies) != 0 {
		t.Fatal("author view did not project the exact frozen solution")
	}
	bad := view.SolutionBinding
	bad.FormalPlanSHA256 = strings.Repeat("f", 64)
	loginStatus(t, post(y, bad, authorCookies, owner), 409)
	loginStatus(t, post(y, view.SolutionBinding, reviewerCookies, reviewer), 403)
	t.Run("broken_solution_cannot_be_confirmed", func(t *testing.T) {
		defer corrupt()()
		loginStatus(t, get(x, "/solution", authorCookies, owner), 503)
		loginStatus(t, post(y, view.SolutionBinding, authorCookies, owner), 503)
	})
	loginStatus(t, get(x, "/solution-confirmation", authorCookies, owner), 404)
	wrote := post(y, view.SolutionBinding, authorCookies, owner)
	loginStatus(t, wrote, 200)
	var recorded problemreference.SolutionConfirmation
	must(t, json.Unmarshal(wrote.body, &recorded))
	recovered := get(x, "/solution-confirmation", authorCookies, owner)
	loginStatus(t, recovered, 200)
	replay := post(x, view.SolutionBinding, authorCookies, owner)
	loginStatus(t, replay, 200)
	if recorded.IntakeKey != receipt.RequestKey || recorded.SolutionBinding != view.SolutionBinding || recorded.ConfirmedAt.IsZero() ||
		string(recovered.body) != string(wrote.body) || string(replay.body) != string(wrote.body) ||
		strings.Contains(string(wrote.body), owner) || strings.Contains(string(wrote.body), "synthetic A+B") {
		t.Fatal("author confirmation did not return a stable private receipt")
	}
}
