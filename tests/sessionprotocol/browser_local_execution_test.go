//go:build integration && browser

package sessionprotocol_test

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func browserLocalSource(t *testing.T, endpoint *loginEndpoint, cookies []*http.Cookie) problem.ImportSource {
	t.Helper()
	answer := importGET(endpoint, entrySourcesPath, cookies)
	loginStatus(t, answer, 200)
	var got struct{ Sources []problem.ImportSource }
	must(t, json.Unmarshal(answer.body, &got))
	if len(got.Sources) != 1 || got.Sources[0].SourceID != "library-checker:aplusb" ||
		got.Sources[0].ImportRevision != "e96d280c89a6e5d3da76ee5347c0581d4153d7052e9afceddffb2fdbb3d2036d" {
		t.Fatal("local fixture did not load the frozen A+B revision")
	}
	return got.Sources[0]
}

func browserLocalExecution(t *testing.T, f *fixture, c *sessionruntime.Config) {
	t.Helper()
	raw, err := os.ReadFile("/local-evidence/admission.json")
	must(t, err)
	var in problemvalidation.APlusBLocalAdmissionInput
	must(t, json.Unmarshal(raw, &in))
	admission, err := problemvalidation.ReadAPlusBLocalAdmission(t.Context(), in)
	must(t, err)
	bad := in
	bad.ReportSHA256[1] = bad.ReportSHA256[0]
	_, err = problemvalidation.ReadAPlusBLocalAdmission(t.Context(), bad)
	if err == nil {
		t.Fatal("wrong report pin enabled local execution")
	}
	_, version, _ := admission.Binding()
	materials, err := submissionpg.New(f.pools[0], version, admission.Manifest())
	must(t, err)
	must(t, materials.RegisterFixture(t.Context()))
	access, err := identitypg.NewProblemAccess(f.pools[0])
	must(t, err)
	reviews, err := referencepg.NewReviewStore(f.pools[0], access)
	must(t, err)
	reviews, err = reviews.WithLocalExecution(admission)
	must(t, err)
	must(t, reviews.RegisterLocalExecution(t.Context()))

	c.PrivateDrafts.BundleDirectory = "/local-evidence/bundle"
	c.PrivateDrafts.SourceLockFile = "/local-evidence/library-checker.json"
	c.PrivateDrafts.Timeout = 50 * time.Second
	c.StartupTimeout = 40 * time.Second
	c.PrivateDrafts.LocalExecution = &in
}

func browserLocalPublisher(t *testing.T, f *fixture, access *identitypg.ProblemAccess) string {
	t.Helper()
	publisher := f.subject(t, false)
	_, err := access.SetPublisherRole(f.ctx, identity.ProblemRoleChange{SubjectID: publisher.ID, Enabled: true,
		Operator: "local-functional-acceptance", Reason: "separate local publisher"})
	must(t, err)
	return publisher.ID
}

func confirmBrowserLocalSolution(t *testing.T, endpoint *loginEndpoint, cookies []*http.Cookie, author string, review problemreference.ReviewReceipt) {
	t.Helper()
	base := "/api/v1/problem-draft-management/review-submissions/" + review.RequestKey
	view := loginCall(endpoint, base+"/solution", "", cookies, func(r *http.Request) {
		r.Method = http.MethodGet
		r.Header.Set("X-OJ-Expected-Subject", author)
	})
	loginStatus(t, view, 200)
	var solution problemreference.AuthorSolution
	must(t, json.Unmarshal(view.body, &solution))
	raw, err := json.Marshal(solution.SolutionBinding)
	must(t, err)
	got := loginCall(endpoint, base+"/solution-confirmation", string(raw), cookies, func(r *http.Request) {
		r.Header.Set("X-OJ-Expected-Subject", author)
	})
	loginStatus(t, got, 200)
}
