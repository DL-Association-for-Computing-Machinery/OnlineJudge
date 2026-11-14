package httptransport_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type approvalFixture struct {
	writes, reads int
}

func (f *approvalFixture) ApprovalForSubject(_ context.Context, _ redisstore.Credential, _, _ string, _ problemreference.ApprovalIntent) (problemreference.ReviewApprovalReceipt, error) {
	f.writes++
	return problemreference.ReviewApprovalReceipt{}, problemreference.ErrReviewApprovalBlocked
}

func (f *approvalFixture) ApprovalReceiptForSubject(_ context.Context, _ redisstore.Credential, _, _, _ string) (problemreference.ReviewApprovalReceipt, error) {
	f.reads++
	return problemreference.ReviewApprovalReceipt{}, problemreference.ErrNotFound
}

func TestApprovalRoutesRequireOptInAndSameOrigin(t *testing.T) {
	base, err := transport.WithReviewEvidence(draftHandler(t, &draftUseCase{}), unavailableEvidence{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	f := &approvalFixture{}
	h, err := transport.WithReviewApprovals(base, f)
	if err != nil {
		t.Fatal(err)
	}
	const key = "approval-request-key"
	path := "/api/v1/problem-review/" + draftID + "/approve"
	makeRequest := func(method, path string) *http.Request {
		r := request(method, path)
		r.Header.Set("X-OJ-Expected-Subject", strings.Repeat("e", 32))
		if method == http.MethodPost {
			body := `{"request_key":"` + key + `","assignment_revision":1,"snapshot_sha256":"` + strings.Repeat("a", 64) + `","reason":"Checked"}`
			r.Header.Set("Content-Type", "application/json")
			r.Body = io.NopCloser(strings.NewReader(body))
			r.ContentLength = int64(len(body))
		}
		return r
	}
	response(t, base, makeRequest(http.MethodPost, path), 404, false)
	foreign := makeRequest(http.MethodPost, path)
	foreign.Header.Set("Origin", "https://other.example")
	response(t, h, foreign, 403, false)
	response(t, h, makeRequest(http.MethodGet, path), 405, false)
	response(t, h, makeRequest(http.MethodPost, path+"/"+key), 405, false)
	if f.writes != 0 || f.reads != 0 {
		t.Fatal("rejected HTTP request reached the approval service")
	}
	w := response(t, h, makeRequest(http.MethodPost, path), 409, false)
	if !strings.Contains(w.Body.String(), "REVIEW_APPROVAL_BLOCKED") ||
		w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("missing stable approval failure or referrer policy")
	}
	response(t, h, makeRequest(http.MethodGet, path+"/"+key), 404, false)
	if f.writes != 1 || f.reads != 1 {
		t.Fatal("POST and original-key GET did not dispatch separately")
	}
}
