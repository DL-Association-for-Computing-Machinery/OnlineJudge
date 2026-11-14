//go:build integration && browser

package sessionprotocol_test

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

// Read operator pins for publications that the isolated driver prepares.
// The API never uses the authoring config or opens a source bundle in this mode.
func browserPublishedTraining(t *testing.T, root string) *sessionruntime.PublishedTrainingConfig {
	t.Helper()
	raw, err := os.ReadFile("/workspace/publication-pins.json")
	must(t, err)
	c := &sessionruntime.PublishedTrainingConfig{MaterialRoot: root, MaxInFlight: 2}
	must(t, json.Unmarshal(raw, &c.ManifestPins))
	if len(c.ManifestPins) != 2 || !filepath.IsAbs(root) {
		t.Fatal("two explicit material pins and an absolute object root are required")
	}
	must(t, c.Validate())
	return c
}

// publishBrowserTrainingFixtures uses the existing one-source authoring and
// approval paths before the standalone training API starts. As in the older
// browser fixture, review preparation seeds terminal reference evidence; actual
// execution evidence comes from the independently pinned admission and live Worker.
func publishBrowserTrainingFixtures(t *testing.T, f *fixture, root string) {
	t.Helper()
	access, err := identitypg.NewProblemAccess(f.pools[0])
	must(t, err)
	publisher := browserLocalPublisher(t, f, access)
	for n, source := range []string{"library-checker:aplusb", "library-checker:unionfind"} {
		author, reviewer := f.subject(t, false), f.subject(t, false)
		c := runtimeConfig(t, f, namespace(t)+"_publish_"+string(rune('a'+n)), true)
		c.PrivateDrafts = &sessionruntime.PrivateDraftsConfig{MaterialRoot: root, SourceIDs: []string{source},
			Timeout: 50 * time.Second, MaxInFlight: 1, ReferenceRequests: true, ReviewIntake: true,
			ReviewAccess: true, ReviewApprovals: true, StatementConfirmations: true, PublicCatalog: true}
		if n == 0 {
			browserLocalExecution(t, f, &c)
		} else {
			browserLocalTextAt(t, f, &c, "/local-text-evidence")
		}
		must(t, referencepg.Configure(f.ctx, f.pools[0], problemreference.Limits{Global: 4, Owner: 1}))
		_, err := access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: author.ID, Enabled: true, Operator: "browser-fixture", Reason: "separate fixture author"})
		must(t, err)
		_, err = access.SetReviewerRole(f.ctx, identity.ProblemRoleChange{SubjectID: reviewer.ID, Enabled: true, Operator: "browser-fixture", Reason: "separate fixture reviewer"})
		must(t, err)
		ca := testpki.New(t)
		proc := entryStart(t, entryBinary(t), entryConfigWithWriteTimeout(t, c, ca, 3*time.Second, 55*time.Second), "127.0.0.1:0")
		endpoint := entryClient(t, proc, ca)
		review := preparePublishedBrowserReview(t, f, endpoint, access, author, reviewer, source)
		proc.signal(t)
		proc.wait(t, true)
		args := []string{"publish-aplusb-local", "--review", review, "--reviewer", reviewer.ID,
			"--publisher", publisher, "--problem", strings.Repeat("0", 31) + string(rune('1'+n)),
			"--request-key", "multi-publication-acceptance-" + string(rune('a'+n)), "--material-root", root,
			"--admission-file", "/local-evidence/admission.json", "--database-socket", c.Database.Host,
			"--database-user", c.Database.Username, "--assignment-revision", "1", "--publisher-revision", "1"}
		if n == 1 {
			args[0] = "publish-text-local"
			for i := range args {
				if args[i] == "/local-evidence/admission.json" {
					args[i] = "/local-text-evidence/admission.json"
				}
			}
			args = append(args, "--bundle", c.PrivateDrafts.BundleDirectory, "--source-lock", c.PrivateDrafts.SourceLockFile)
		}
		browserPublishCommand(t, args)
	}
}

func browserPublishCommand(t *testing.T, args []string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), entryProgram(t, "problem-review"), args...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture publication failed: %s (%v)", out, err)
	}
}

func preparePublishedBrowserReview(t *testing.T, f *fixture, endpoint *loginEndpoint, access *identitypg.ProblemAccess, author, reviewer identity.Personal, sourceID string) string {
	t.Helper()
	cookiesFor := func(p identity.Personal) []*http.Cookie {
		pending := beginLogin(t, endpoint, p)
		credential := completedCredential(t, loginCall(endpoint, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
		return []*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}}
	}
	authorCookies := cookiesFor(author)
	sourceList := importGET(endpoint, entrySourcesPath, authorCookies)
	loginStatus(t, sourceList, 200)
	var catalog struct {
		Sources []problem.ImportSource `json:"sources"`
	}
	must(t, json.Unmarshal(sourceList.body, &catalog))
	sources := catalog.Sources
	if len(sources) != 1 || sources[0].SourceID != sourceID {
		t.Fatal("wrong authoring source")
	}
	imported := loginCall(endpoint, "/api/v1/problem-draft-management/imports", pinnedBody(t, "multi-publication-author-import", sourceID, sources[0].ImportRevision), authorCookies, func(r *http.Request) {
		r.Header.Set("X-OJ-Expected-Subject", author.ID)
	})
	loginStatus(t, imported, 200)
	var draft problem.ImportReceipt
	must(t, json.Unmarshal(imported.body, &draft))
	result := testEntryReferenceResult(t, f, endpoint, endpoint, authorCookies, author.ID, draft.DraftID, reviewer.ID)
	intent := problemreference.ReviewIntent{RequestKey: "multi-publication-review-intake", DraftID: draft.DraftID,
		ImportRevision: draft.ImportRevision, ReferenceRequestID: result}
	request := func(path, method, subject string, cookies []*http.Cookie, data any) loginResponse {
		body := ""
		if data != nil {
			raw, err := json.Marshal(data)
			must(t, err)
			body = string(raw)
		}
		got := loginCall(endpoint, path, body, cookies, func(r *http.Request) {
			r.Method = method
			r.Header.Set("X-OJ-Expected-Subject", subject)
		})
		if got.status != 200 {
			t.Fatalf("%s fixture request failed: %d %s", path, got.status, got.body)
		}
		return got
	}
	res := request("/api/v1/problem-draft-management/review-submissions", "POST", author.ID, authorCookies, intent)
	loginStatus(t, res, 200)
	var review problemreference.ReviewReceipt
	must(t, json.Unmarshal(res.body, &review))
	store, err := referencepg.NewReviewStore(f.pools[0], access)
	must(t, err)
	_, err = store.SetAssignment(f.ctx, problemreference.ReviewAssignmentChange{ReviewID: review.ID, ReviewerSubjectID: reviewer.ID,
		ExpectedReviewerRevision: 1, Operator: "browser-fixture", Reason: "assign local multi-publication review"})
	must(t, err)
	confirmBrowserLocalSolution(t, endpoint, authorCookies, author.ID, review)
	cookies := cookiesFor(reviewer)
	base := "/api/v1/problem-review/" + review.ID
	read := func() problemreference.ReviewEligibility {
		got := request(base+"/eligibility", "GET", reviewer.ID, cookies, nil)
		loginStatus(t, got, 200)
		var status problemreference.ReviewEligibility
		must(t, json.Unmarshal(got.body, &status))
		return status
	}
	status := read()
	confirm := problemreference.StatementConfirmationIntent{RequestKey: "multi-publication-confirm-statement", AssignmentRevision: 1,
		ImportRevision: status.Evidence.ImportRevision, EvidenceSHA256: status.Evidence.EvidenceSHA256, ChecklistVersion: problemreference.StatementChecklistVersion}
	loginStatus(t, request(base+"/statement-confirmation", "POST", reviewer.ID, cookies, confirm), 200)
	status = read()
	if !status.Eligible {
		t.Fatalf("fixture has review blockers: %v", status.Blockers)
	}
	approval := map[string]any{"request_key": "multi-publication-approve-review", "assignment_revision": 1,
		"snapshot_sha256": status.SnapshotSHA256, "reason": "confirm local fixture statement"}
	loginStatus(t, request(base+"/approve", "POST", reviewer.ID, cookies, approval), 200)
	return review.ID
}
