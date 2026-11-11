//go:build integration

package sessionprotocol_test

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func testEntryPrivateDrafts(t *testing.T, f *fixture, binary string) {
	for _, mode := range []string{"direct", "proxy"} {
		t.Run(mode, func(t *testing.T) { testEntryDraftMode(t, f, binary, mode) })
	}
}

func testEntryDraftMode(t *testing.T, f *fixture, binary, mode string) {
	bundle := referenceEntryBundle(t)
	root := filepath.Join(t.TempDir(), "materials")
	must(t, os.Mkdir(root, 0700))
	lock := filepath.Join(t.TempDir(), "trusted-lock.json")
	must(t, os.WriteFile(lock, bundle.Lock, 0600))
	c := runtimeConfig(t, f, namespace(t), true)
	c.PrivateDrafts = &sessionruntime.PrivateDraftsConfig{MaterialRoot: root, BundleDirectory: bundle.Directory, SourceLockFile: lock,
		SourceIDs: []string{"library-checker:aplusb"}, Timeout: 3 * time.Second, MaxInFlight: 1,
		ReferenceRequests: true, ReviewIntake: true, ReviewAccess: true, ReviewReturns: true}
	ca := testpki.New(t)
	var proxyCert tls.Certificate
	if mode == "proxy" {
		proxyCert, _, _ = ca.Issue(t, "spiffe://onlinejudge/proxy/drafts", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
		c.Mode = sessionruntime.ProxyMTLS
		c.Proxy.CertificateSHA256 = [][32]byte{sha256.Sum256(proxyCert.Certificate[0])}
	}
	endpoint := func(p *entryProcess) *loginEndpoint {
		if mode == "proxy" {
			return proxyClient(t, entryProxy(t, p, ca, proxyCert), "127.0.0.2", nil)
		}
		return entryClient(t, p, ca)
	}
	a := entryStart(t, binary, entryConfig(t, c, ca, 3*time.Second), "127.0.0.1:0")
	x := endpoint(a)
	other := c
	other.Database.ApplicationName += "_b"
	readOnly := *c.PrivateDrafts
	readOnly.ReviewReturns = false
	other.PrivateDrafts = &readOnly
	b := entryStart(t, binary, entryConfig(t, other, ca, 3*time.Second), "127.0.0.1:0")
	y := endpoint(b)
	if mode == "proxy" {
		bare := entryClient(t, a, ca)
		if result := loginCall(bare, "/api/v1/problem-drafts", "{}", nil, nil); result.err == nil {
			t.Fatal("draft API admitted connection without proxy certificate")
		}
		unpinned, _, _ := ca.Issue(t, "spiffe://onlinejudge/proxy/drafts-other", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
		bare.client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{unpinned}
		loginStatus(t, loginCall(bare, "/api/v1/problem-drafts", "{}", nil, func(r *http.Request) {
			r.Header.Set("X-Forwarded-For", "192.0.2.1")
			r.Header.Set("X-Forwarded-Host", "127.0.0.1")
			r.Header.Set("X-Forwarded-Proto", "https")
		}), 403)
	}
	subject := f.subject(t, false)
	pending := beginLogin(t, x, subject)
	credential := completedCredential(t, loginCall(y, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
	cookies := []*http.Cookie{{Name: transport.CookieName, Value: credential.Value()}}
	const collection = "/api/v1/problem-drafts"
	const body = `{"source_id":"library-checker:aplusb","request_key":"entry-private-draft-request"}`
	unknown := strings.Replace(body, "library-checker:aplusb", "unknown", 1)
	for _, candidate := range []string{body, unknown} {
		loginStatus(t, loginCall(x, collection, candidate, nil, nil), 401)
		loginStatus(t, loginCall(x, collection, candidate, cookies, nil), 403)
	}
	access, err := identitypg.NewProblemAccess(f.pools[0])
	must(t, err)
	testEntryImportPreflight(t, x, cookies)
	loginStatus(t, authorReferenceCall(x, "GET", "/author-reference-fixture", "", nil, subject.ID), 401)
	loginStatus(t, authorReferenceCall(x, "GET", "/author-reference-fixture", "", cookies, subject.ID), 403)
	loginStatus(t, ownListGET(x, nil, subject.ID, ""), 401)
	loginStatus(t, ownListGET(x, cookies, subject.ID, ""), 403)
	_, err = access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: subject.ID, Enabled: true, ExpectedRevision: 0, Operator: "entry-fixture", Reason: "synthetic private draft author"})
	must(t, err)
	checkEntryOwnList(t, x, cookies, subject.ID, 0)
	loginStatus(t, loginCall(x, collection, body, cookies, func(r *http.Request) { r.Header.Set("Origin", "https://other.invalid") }), 403)
	loginStatus(t, loginCall(x, collection, unknown, cookies, nil), 400)
	var zero int
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT count(*) FROM problem_drafts WHERE owner_subject_id=$1`, subject.ID).Scan(&zero))
	if zero != 0 {
		t.Fatal("denied HTTP created metadata")
	}
	created := loginCall(x, collection, body, cookies, nil)
	loginStatus(t, created, 200)
	var draft struct {
		ID              string         `json:"id"`
		State           string         `json:"state"`
		ValidationState string         `json:"validation_state"`
		Document        map[string]any `json:"document"`
		References      []any          `json:"references"`
	}
	must(t, json.Unmarshal(created.body, &draft))
	if !identity.ValidID(draft.ID) || draft.State != "private-draft" || draft.ValidationState != "not-run" || draft.Document["title"] != "Synthetic A + B" || len(draft.References) != 13 {
		t.Fatal("HTTP did not return complete private metadata")
	}
	// Discard the first response: the other API recovers exactly the same receipt.
	replayed := loginCall(y, collection, body, cookies, nil)
	loginStatus(t, replayed, 200)
	if string(replayed.body) != string(created.body) {
		t.Fatal("cross-API original-key recovery changed receipt")
	}
	pinnedReceipt := testEntryPinnedImport(t, x, y, cookies)
	checkEntryOwnList(t, y, cookies, subject.ID, 2)
	if pinnedReceipt.DraftID == draft.ID {
		t.Fatal("different request keys shared a draft")
	}
	referenceID := testEntryAuthorReferences(t, f, x, y, cookies, subject.ID, draft.ID, pinnedReceipt.DraftID)
	reviewID := testEntryReviewAccess(t, f, x, y, access, cookies, subject.ID, draft.ID, referenceID, root, c.Namespace)
	get := func(endpoint *loginEndpoint, id string, c []*http.Cookie) loginResponse {
		return loginCall(endpoint, collection+"/"+id, "", c, func(r *http.Request) { r.Method = "GET" })
	}
	got := get(y, draft.ID, cookies)
	loginStatus(t, got, 200)
	if string(got.body) != string(created.body) {
		t.Fatal("owner read changed immutable metadata")
	}
	preview := get(y, draft.ID+"/preview", cookies)
	loginStatus(t, preview, 200)
	var content problem.Preview
	must(t, json.Unmarshal(preview.body, &content))
	if content.DraftID != draft.ID || content.Markdown != "# Synthetic A + B\n" || content.Title != "Synthetic A + B" ||
		len(content.Examples) != 1 || content.Examples[0].ID != "case-0001" || content.Examples[0].Input != "1 2\n" || content.Examples[0].Output != "3\n" {
		t.Fatal("actual HTTP preview changed private statement or sample bytes")
	}
	changed := referenceEntryBundle(t)
	changed.Document["provenance"].(map[string]any)["original_title"] = "changed private metadata"
	changed.Write(t)
	cChanged := c
	cChanged.Database.ApplicationName += "_changed"
	dChanged := *c.PrivateDrafts
	dChanged.BundleDirectory = changed.Directory
	dChanged.ReferenceRequests, dChanged.ReviewIntake, dChanged.ReviewAccess, dChanged.ReviewReturns = false, false, false, false
	cChanged.PrivateDrafts = &dChanged
	third := entryStart(t, binary, entryConfig(t, cChanged, ca, 3*time.Second), "127.0.0.1:0")
	thirdEndpoint := endpoint(third)
	testEntryChangedImportCatalog(t, thirdEndpoint, cookies, pinnedReceipt)
	loginStatus(t, authorReferenceCall(thirdEndpoint, "GET", "/author-reference-fixture", "", cookies, subject.ID), 404)
	loginStatus(t, reviewEvidenceGET(thirdEndpoint, reviewID, cookies, subject.ID), 404)
	loginStatus(t, reviewMaterialsGET(thirdEndpoint, reviewID, cookies, subject.ID), 404)
	loginStatus(t, reviewReadGET(thirdEndpoint, reviewID+"/comparison", cookies, subject.ID), 404)
	checkEntryOwnList(t, thirdEndpoint, cookies, subject.ID, 2)
	loginStatus(t, loginCall(thirdEndpoint, collection, body, cookies, nil), 409)
	oldVersion := get(thirdEndpoint, draft.ID+"/preview", cookies)
	loginStatus(t, oldVersion, 200)
	if string(oldVersion.body) != string(preview.body) {
		t.Fatal("preview followed current import catalog instead of persisted version")
	}
	third.signal(t)
	third.wait(t, true)
	noRuntimeConnections(t, f, cChanged.Database.ApplicationName)
	stranger := f.subject(t, false)
	pending = beginLogin(t, y, stranger)
	strangerCredential := completedCredential(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil))
	_, err = access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: stranger.ID, Enabled: true, ExpectedRevision: 0, Operator: "entry-fixture", Reason: "synthetic second author"})
	must(t, err)
	strangerCookies := []*http.Cookie{{Name: transport.CookieName, Value: strangerCredential.Value()}}
	checkEntryOwnList(t, x, strangerCookies, stranger.ID, 0)
	loginStatus(t, ownListGET(x, strangerCookies, subject.ID, ""), 409)
	testEntryReceiptOwner(t, x, strangerCookies, pinnedReceipt.RequestKey)
	loginStatus(t, authorReferenceCall(x, "POST", "/author-reference-fixture/cancel", "", strangerCookies, subject.ID), 409)
	foreignRef := authorReferenceCall(x, "GET", "/author-reference-fixture", "", strangerCookies, stranger.ID)
	missingRef := authorReferenceCall(x, "GET", "/missing-reference-fixture", "", strangerCookies, stranger.ID)
	loginStatus(t, foreignRef, 404)
	loginStatus(t, missingRef, 404)
	if string(foreignRef.body) != string(missingRef.body) {
		t.Fatal("reference receipt disclosed another author's existence")
	}
	resultPath := "/" + entryTerminalReferenceKey + "/report"
	foreignReport := authorReferenceCall(x, "GET", resultPath, "", strangerCookies, stranger.ID)
	missingReport := authorReferenceCall(x, "GET", "/missing-reference-fixture/report", "", strangerCookies, stranger.ID)
	loginStatus(t, foreignReport, 404)
	loginStatus(t, missingReport, 404)
	if string(foreignReport.body) != string(missingReport.body) {
		t.Fatal("reference report disclosed another author's existence")
	}
	foreign, missing := get(x, draft.ID, strangerCookies), get(x, strings.Repeat("f", 32), strangerCookies)
	loginStatus(t, foreign, 404)
	loginStatus(t, missing, 404)
	if string(foreign.body) != string(missing.body) {
		t.Fatal("foreign existence disclosed")
	}
	foreign, missing = get(x, draft.ID+"/preview", strangerCookies), get(x, strings.Repeat("f", 32)+"/preview", strangerCookies)
	loginStatus(t, foreign, 404)
	loginStatus(t, missing, 404)
	if string(foreign.body) != string(missing.body) {
		t.Fatal("preview disclosed another author's existence")
	}
	_, err = access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: subject.ID, Enabled: false, ExpectedRevision: 1, Operator: "entry-fixture", Reason: "revoke author"})
	must(t, err)
	loginStatus(t, get(y, draft.ID, cookies), 403)
	loginStatus(t, authorReferenceCall(y, "GET", "/author-reference-fixture", "", cookies, subject.ID), 403)
	loginStatus(t, ownListGET(y, cookies, subject.ID, ""), 403)
	loginStatus(t, get(y, draft.ID+"/preview", cookies), 403)
	loginStatus(t, loginCall(y, collection, body, cookies, nil), 403)
	loginStatus(t, importGET(y, entrySourcesPath, cookies), 403)
	loginStatus(t, importGET(y, entryImportsPath+"/"+pinnedReceipt.RequestKey, cookies), 403)
	loginStatus(t, loginCall(y, entryImportsPath, pinnedBody(t, pinnedReceipt.RequestKey, pinnedReceipt.SourceID, pinnedReceipt.ImportRevision), cookies, nil), 403)
	loginStatus(t, authorReferenceCall(y, "GET", resultPath, "", cookies, subject.ID), 403)
	_, err = access.SetAuthorRole(f.ctx, identity.ProblemRoleChange{SubjectID: subject.ID, Enabled: true, ExpectedRevision: 2, Operator: "entry-fixture", Reason: "restore author"})
	must(t, err)
	loginStatus(t, authorReferenceCall(y, "GET", resultPath, "", cookies, subject.ID), 200)
	loginStatus(t, loginCall(x, "/api/v1/session/logout", "", cookies, nil), 204)
	loginStatus(t, get(y, draft.ID, cookies), 401)
	loginStatus(t, authorReferenceCall(y, "GET", "/author-reference-fixture", "", cookies, subject.ID), 401)
	loginStatus(t, authorReferenceCall(y, "GET", resultPath, "", cookies, subject.ID), 401)
	loginStatus(t, ownListGET(y, cookies, subject.ID, ""), 401)
	loginStatus(t, get(y, draft.ID+"/preview", cookies), 401)
	loginStatus(t, importGET(y, entrySourcesPath, cookies), 401)
	loginStatus(t, importGET(y, entryImportsPath+"/"+pinnedReceipt.RequestKey, cookies), 401)
	var count, refs int
	must(t, f.pools[0].QueryRow(f.ctx, `SELECT count(*),(SELECT count(*) FROM problem_draft_materials WHERE draft_id=$2) FROM problem_drafts WHERE owner_subject_id=$1`, subject.ID, draft.ID).Scan(&count, &refs))
	if count != 2 || refs != 13 {
		t.Fatal("HTTP replay duplicated/partially wrote metadata")
	}
	a.signal(t)
	b.signal(t)
	a.wait(t, true)
	b.wait(t, true)
	noRuntimeConnections(t, f, c.Database.ApplicationName)
	noRuntimeConnections(t, f, other.Database.ApplicationName)
	t.Log("two real TLS API processes, Redis login and PG/private materials: import, original-key recovery, owner isolation, role revocation, logout and graceful shutdown passed")
}

func referenceEntryBundle(t *testing.T) *problemfixture.Bundle {
	t.Helper()
	bundle := problemfixture.New(t)
	bundle.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("synthetic A+B material; not executed by this test\n"))
	bundle.BindReferenceCases(t)
	bundle.Write(t)
	return bundle
}
