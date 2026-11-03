//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

const entrySourcesPath = "/api/v1/problem-draft-sources"
const entryImportsPath = "/api/v1/problem-draft-imports"

func importGET(e *loginEndpoint, path string, cookies []*http.Cookie) loginResponse {
	return loginCall(e, path, "", cookies, func(r *http.Request) { r.Method = http.MethodGet })
}

func pinnedBody(t *testing.T, key, source, revision string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"request_key": key, "source_id": source, "expected_revision": revision})
	must(t, err)
	return string(raw)
}

func entrySources(t *testing.T, endpoint *loginEndpoint, cookies []*http.Cookie) []problem.ImportSource {
	t.Helper()
	result := importGET(endpoint, entrySourcesPath, cookies)
	loginStatus(t, result, 200)
	var catalog struct {
		Sources []problem.ImportSource `json:"sources"`
	}
	must(t, json.Unmarshal(result.body, &catalog))
	if len(catalog.Sources) != 1 || catalog.Sources[0].SourceID != "library-checker:aplusb" || catalog.Sources[0].Title != "Synthetic A + B" || !problem.ValidImportRevision(catalog.Sources[0].ImportRevision) || !problem.ValidImportRevision(catalog.Sources[0].Version) {
		t.Fatal("configured candidate identity changed")
	}
	return catalog.Sources
}

func testEntryImportPreflight(t *testing.T, endpoint *loginEndpoint, cookies []*http.Cookie) {
	t.Helper()
	for _, path := range []string{entrySourcesPath, entryImportsPath + "/original-pinned-entry-key"} {
		loginStatus(t, importGET(endpoint, path, nil), 401)
		loginStatus(t, importGET(endpoint, path, cookies), 403)
	}
	for _, source := range []string{"library-checker:aplusb", "unknown"} {
		body := pinnedBody(t, "original-pinned-entry-key", source, strings.Repeat("f", 64))
		loginStatus(t, loginCall(endpoint, entryImportsPath, body, nil, nil), 401)
		loginStatus(t, loginCall(endpoint, entryImportsPath, body, cookies, nil), 403)
	}
}

func testEntryPinnedImport(t *testing.T, first, second *loginEndpoint, cookies []*http.Cookie) problem.ImportReceipt {
	t.Helper()
	source := entrySources(t, first, cookies)[0]
	const key = "original-pinned-entry-key"
	body := pinnedBody(t, key, source.SourceID, source.ImportRevision)
	loginStatus(t, loginCall(first, entryImportsPath, body, cookies, func(r *http.Request) { r.Header.Set("Origin", "https://other.invalid") }), 403)
	// Read success headers, then close without consuming the receipt body. A
	// browser with a missing/malformed body must recover without another write.
	r, err := http.NewRequest(http.MethodPost, loginOrigin+entryImportsPath, strings.NewReader(body))
	must(t, err)
	r.Header.Set("Origin", loginOrigin)
	r.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	res, err := first.client.Do(r)
	must(t, err)
	status := res.StatusCode
	must(t, res.Body.Close())
	if status != http.StatusOK {
		t.Fatal("pinned import did not commit")
	}
	read := importGET(second, entryImportsPath+"/"+key, cookies)
	loginStatus(t, read, 200)
	var receipt problem.ImportReceipt
	must(t, json.Unmarshal(read.body, &receipt))
	var fields map[string]any
	must(t, json.Unmarshal(read.body, &fields))
	if !identity.ValidID(receipt.DraftID) || receipt.SourceID != source.SourceID || receipt.Version != source.Version || receipt.ImportRevision != source.ImportRevision || receipt.RequestKey != key || len(fields) != 8 || fields["document"] != nil || fields["references"] != nil {
		t.Fatal("read-only recovery changed or expanded receipt")
	}
	replay := loginCall(second, entryImportsPath, body, cookies, nil)
	loginStatus(t, replay, 200)
	if string(replay.body) != string(read.body) {
		t.Fatal("pinned retry did not return the original receipt")
	}
	metadata := importGET(first, "/api/v1/problem-drafts/"+receipt.DraftID, cookies)
	loginStatus(t, metadata, 200)
	var draft struct {
		References []any `json:"references"`
	}
	must(t, json.Unmarshal(metadata.body, &draft))
	if len(draft.References) != 10 {
		t.Fatal("new pinned import did not commit full material references")
	}
	return receipt
}

func testEntryChangedImportCatalog(t *testing.T, endpoint *loginEndpoint, cookies []*http.Cookie, original problem.ImportReceipt) {
	t.Helper()
	source := entrySources(t, endpoint, cookies)[0]
	if source.ImportRevision == original.ImportRevision {
		t.Fatal("changed catalog fixture retained original full revision")
	}
	for _, key := range []string{original.RequestKey, "never-committed-pinned-entry-key"} {
		changed := loginCall(endpoint, entryImportsPath, pinnedBody(t, key, original.SourceID, original.ImportRevision), cookies, nil)
		loginStatus(t, changed, 409)
		var failure struct {
			Code string `json:"code"`
		}
		must(t, json.Unmarshal(changed.body, &failure))
		if failure.Code != "DRAFT_SOURCE_CHANGED" {
			t.Fatal("revision drift was not distinguished from a request-key conflict")
		}
	}
	read := importGET(endpoint, entryImportsPath+"/"+original.RequestKey, cookies)
	loginStatus(t, read, 200)
	var got problem.ImportReceipt
	must(t, json.Unmarshal(read.body, &got))
	if got != original {
		t.Fatal("new deployment failed to recover the committed old revision")
	}
	loginStatus(t, importGET(endpoint, entryImportsPath+"/never-committed-pinned-entry-key", cookies), 404)
}

func testEntryReceiptOwner(t *testing.T, endpoint *loginEndpoint, cookies []*http.Cookie, key string) {
	t.Helper()
	foreign := importGET(endpoint, entryImportsPath+"/"+key, cookies)
	missing := importGET(endpoint, entryImportsPath+"/no-such-pinned-entry-request", cookies)
	loginStatus(t, foreign, 404)
	loginStatus(t, missing, 404)
	if string(foreign.body) != string(missing.body) {
		t.Fatal("receipt revealed another owner's request key")
	}
}
