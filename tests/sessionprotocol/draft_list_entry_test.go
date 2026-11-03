//go:build integration

package sessionprotocol_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

const entryOwnDrafts = "/api/v1/problem-draft-management/drafts"

func ownListGET(endpoint *loginEndpoint, cookies []*http.Cookie, subject, cursor string) loginResponse {
	path := entryOwnDrafts
	if cursor != "" {
		path += "?cursor=" + cursor
	}
	return loginCall(endpoint, path, "", cookies, func(r *http.Request) { r.Method = http.MethodGet; r.Header.Set("X-OJ-Expected-Subject", subject) })
}

func checkEntryOwnList(t *testing.T, endpoint *loginEndpoint, cookies []*http.Cookie, subject string, count int) {
	t.Helper()
	result := ownListGET(endpoint, cookies, subject, "")
	loginStatus(t, result, 200)
	var page problem.OwnDraftPage
	must(t, json.Unmarshal(result.body, &page))
	if len(page.Drafts) != count || page.NextCursor != "" {
		t.Fatal("owner list changed or expanded scope")
	}
	for _, receipt := range page.Drafts {
		if !receipt.Valid() {
			t.Fatal("owner list returned invalid metadata")
		}
	}
	var fields map[string]json.RawMessage
	must(t, json.Unmarshal(result.body, &fields))
	if len(fields) != 2 {
		t.Fatal("list expanded response")
	}
}
