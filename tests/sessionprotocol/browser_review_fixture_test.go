//go:build integration && browser

package sessionprotocol_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
)

type browserReviewTransport struct {
	origin *url.URL
	tls    *http.Transport
}

func (tr browserReviewTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	r := request.Clone(request.Context())
	r.URL.Scheme, r.URL.Host, r.Host = tr.origin.Scheme, tr.origin.Host, tr.origin.Host
	if r.Header.Get("Origin") == loginOrigin {
		r.Header.Set("Origin", tr.origin.String())
	}
	return tr.tls.RoundTrip(r)
}

// seedBrowserReview creates an assigned item only in the disposable browser
// fixture. Reference-result seeding is synthetic; real Worker execution is a
// separate test. Browser reads still cross the real HTTPS API and stores.
func seedBrowserReview(t *testing.T, f *fixture, origin *url.URL, roots *x509.CertPool,
	author, reviewer identity.Personal, access *identitypg.ProblemAccess, root string, localExecution bool) (string, string, string) {
	t.Helper()
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", "127.0.0.1:9443")
		},
	}
	t.Cleanup(tr.CloseIdleConnections)
	endpoint := &loginEndpoint{client: &http.Client{Transport: browserReviewTransport{origin: origin, tls: tr}, Timeout: 8 * time.Second}}
	pending := beginLogin(t, endpoint, author)
	credential := completedCredential(t, loginCall(endpoint, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
	cookies := []*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}}
	var source problem.ImportSource
	if localExecution {
		source = browserLocalSource(t, endpoint, cookies)
	} else {
		source = entrySources(t, endpoint, cookies)[0]
	}
	imported := loginCall(endpoint, entryImportsPath, pinnedBody(t, "browser-review-materials-fixture", source.SourceID, source.ImportRevision), cookies, nil)
	loginStatus(t, imported, 200)
	var draft problem.ImportReceipt
	must(t, json.Unmarshal(imported.body, &draft))
	result := testEntryReferenceResult(t, f, endpoint, endpoint, cookies, author.ID, draft.DraftID, reviewer.ID)
	intent := problemreference.ReviewIntent{RequestKey: "browser-review-intake-fixture", DraftID: draft.DraftID,
		ImportRevision: draft.ImportRevision, ReferenceRequestID: result}
	body, err := json.Marshal(intent)
	must(t, err)
	res := loginCall(endpoint, "/api/v1/problem-draft-management/review-submissions", string(body), cookies, func(r *http.Request) {
		r.Header.Set("X-OJ-Expected-Subject", author.ID)
	})
	loginStatus(t, res, 200)
	var review problemreference.ReviewReceipt
	must(t, json.Unmarshal(res.body, &review))
	_, err = access.SetReviewerRole(f.ctx, identity.ProblemRoleChange{SubjectID: reviewer.ID, Enabled: true, Operator: "browser-fixture", Reason: "synthetic non-author reviewer"})
	must(t, err)
	_, err = access.SetReviewerRole(f.ctx, identity.ProblemRoleChange{SubjectID: author.ID, Enabled: true, Operator: "browser-fixture", Reason: "verify self-review denial"})
	must(t, err)
	store, err := referencepg.NewReviewStore(f.pools[0], access)
	must(t, err)
	_, err = store.SetAssignment(f.ctx, problemreference.ReviewAssignmentChange{ReviewID: review.ID, ReviewerSubjectID: reviewer.ID,
		ExpectedReviewerRevision: 1, Operator: "browser-fixture", Reason: "assign synthetic browser review"})
	must(t, err)
	if localExecution {
		confirmBrowserLocalSolution(t, endpoint, cookies, author.ID, review)
		return review.ID, draft.DraftID, ""
	}
	// Keep the real review fixture unapproved. The catalog fixture is a
	// historical row with a different assignment, and cannot satisfy one
	// of the ordinary review's authorization or execution predicates.
	catalogID := seedBrowserCatalog(t, f, access, root, author, reviewer, review, result)
	return review.ID, draft.DraftID, catalogID
}

func seedBrowserCatalog(t *testing.T, f *fixture, access *identitypg.ProblemAccess, root string, author, reviewer identity.Personal,
	review problemreference.ReviewReceipt, reference string) string {
	t.Helper()
	recovery, err := f.identities[0].RecoveryState(f.ctx)
	must(t, err)
	repo, err := problempg.New(f.pools[0], access)
	must(t, err)
	actor := identity.ProblemActor{SubjectID: author.ID, SessionVersion: identity.SessionVersion{
		AuthGeneration: author.AuthGeneration, RecoveryID: recovery.Generation}}
	draft, err := repo.GetOwn(f.ctx, actor, review.DraftID)
	must(t, err)
	binding, err := draft.MaterialInventoryIdentity()
	must(t, err)
	var raw []byte
	var digest string
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT plan_encoding,plan_sha256 FROM problem_reference_expected_plans WHERE request_id=$1`, reference).Scan(&raw, &digest))
	var plan problem.ReferencePlan
	must(t, json.Unmarshal(raw, &plan))
	const id, history = "00000000000000000000000000000501", "00000000000000000000000000000502"
	planned, err := draft.PlanPublicationMaterials(id, draft.ID, author.ID, binding, plan, digest)
	must(t, err)
	objects, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	must(t, err)
	sealed, err := planned.Seal(f.ctx, objects)
	var display problem.PublicationStatement
	if err == nil {
		display, err = problem.ReadSealedPublicationStatement(f.ctx, objects, sealed)
	}
	must(t, errors.Join(err, objects.Close()))
	_, err = access.SetPublisherRole(f.ctx, identity.ProblemRoleChange{SubjectID: author.ID, Enabled: true,
		Operator: "browser-fixture", Reason: "synthetic historical publisher"})
	must(t, err)
	tx, err := f.pools[0].Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_submissions
		SELECT $1,owner_subject_id,'catalog-review-fixture',draft_id,source_id,content_version,
			import_revision,source_lock_sha256,reference_request_id,formal_plan_sha256,profile_sha256,evidence_sha256,
			formal_verdict,differential_plan_sha256,differential_outcome,auth_generation,recovery_id,author_revision,now()
		FROM problem_review_submissions WHERE id=$2`, history, review.ID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_assignment_history (review_id,revision,reviewer_subject_id,reviewer_revision,maintenance_operator,reason)
		VALUES ($1,1,$2,1,'browser-fixture','historical catalog data')`, history, reviewer.ID)
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_approvals
		(review_id,assignment_revision,reviewer_subject_id,reviewer_revision,request_key,material_sha256,snapshot_sha256,
		policy_version,reason,auth_generation,recovery_id)
		VALUES ($1,1,$2,1,'catalog-approval-fixture',repeat('b',64),repeat('c',64),'synthetic-history','fixture only',$3,$4)`,
		history, reviewer.ID, reviewer.AuthGeneration, recovery.Generation)
	must(t, err)
	const version = "catalog-browser-fixture"
	execution := strings.Repeat("a", 64)
	_, err = tx.Exec(f.ctx, `INSERT INTO submission_materials (version_id,manifest_sha256,manifest) VALUES ($1,$2,'{}')`, version, execution)
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_execution_admissions
		(import_revision,formal_plan_sha256,execution_version,execution_sha256) VALUES ($1,$2,$3,$4)`,
		binding.ImportRevision, digest, version, execution)
	must(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_publications
		(problem_id,revision,version,title,statement_sha256,statement_size,import_revision,formal_plan_sha256,
		execution_version,execution_sha256,review_id,assignment_revision,publisher_id,publisher_revision,request_key)
		VALUES ($1,1,$2,$3,$4,$5,$6,$7,$8,$9,$10,1,$11,1,'catalog-publish-fixture')`,
		id, sealed.Version, display.Title, sealed.Statement.SHA256, sealed.Statement.Size, binding.ImportRevision,
		digest, version, execution, history, author.ID)
	must(t, err)
	// Withdrawing the old assignment keeps it out of the current work list.
	_, err = tx.Exec(f.ctx, `INSERT INTO problem_review_assignment_history
		(review_id,revision,maintenance_operator,reason) VALUES ($1,2,'browser-fixture','historical assignment closed')`, history)
	must(t, err)
	must(t, tx.Commit(f.ctx))
	return id
}
