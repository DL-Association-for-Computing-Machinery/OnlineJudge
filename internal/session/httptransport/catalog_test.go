package httptransport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
)

func TestCatalogRequiresTrustedHTTPSAndNeverReadsSessions(t *testing.T) {
	const path = "/api/v1/problems"
	base := newHandler(t, &fakeStore{})
	listCalls, detailCalls := 0, 0
	enabled, err := transport.WithCatalog(base, transport.CatalogReader{
		List: func(_ context.Context, after, query string) ([]transport.CatalogEntry, string, error) {
			listCalls++
			if after != draftID || query != "A+B" {
				t.Fatalf("unexpected filter: %q, %q", after, query)
			}
			return []transport.CatalogEntry{}, "", nil
		},
		Detail: func(_ context.Context, id string) (problem.PublicationStatement, error) {
			detailCalls++
			if id == draftID {
				return problem.PublicationStatement{}, problemreference.ErrNotFound
			}
			return problem.PublicationStatement{}, problemreference.ErrStore
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response(t, httpapi.NewSessionHandler(base), httptest.NewRequest("GET", origin+path, nil), 404, false)
	h := httpapi.NewSessionHandler(enabled)
	w := response(t, h, httptest.NewRequest("GET", origin+path+"?q=A%2BB&after="+draftID, nil), 200, false)
	if strings.TrimSpace(w.Body.String()) != `{"problems":[],"next":""}` {
		t.Fatal("unexpected public catalog projection")
	}
	for _, c := range []struct {
		method, uri string
		status      int
	}{
		{"HEAD", origin + path, 405},
		{"POST", origin + path, 405},
		{"GET", origin + path + "?q=a&q=b", 400},
		{"GET", origin + path + "?draft_id=" + draftID, 400},
		{"GET", origin + path + "/" + draftID + "?q=a", 400},
		{"GET", origin + path + "/%31" + draftID[1:], 400},
		{"GET", origin + path + "/" + draftID + "/materials", 404},
		{"GET", origin + path + "/" + draftID, 404},
		{"GET", origin + path + "/" + strings.Repeat("b", 32), 503},
		{"GET", "http://example.test" + path, 403},
	} {
		response(t, h, httptest.NewRequest(c.method, c.uri, nil), c.status, false)
	}
	response(t, h, httptest.NewRequest(http.MethodGet, origin+path, strings.NewReader("x")), 400, false)
	if listCalls != 1 || detailCalls != 2 {
		t.Fatalf("unexpected reader calls: %d, %d", listCalls, detailCalls)
	}
}
