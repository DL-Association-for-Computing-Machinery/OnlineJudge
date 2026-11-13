//go:build integration && browser

package sessionprotocol_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
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
func seedBrowserReview(t *testing.T, f *fixture, origin *url.URL, roots *x509.CertPool, author, reviewer identity.Personal, access *identitypg.ProblemAccess) (string, string) {
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
	source := entrySources(t, endpoint, cookies)[0]
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
	return review.ID, draft.DraftID
}
