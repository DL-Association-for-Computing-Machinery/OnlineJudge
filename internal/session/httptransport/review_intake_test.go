package httptransport_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type reviewReferenceStub struct {
	transport.ReferenceRequests
	transport.ReferencePreparation
}

type reviewIntakeStub struct {
	calls int
	err   error
	want  problemreference.ReviewReceipt
}

func (s *reviewIntakeStub) SubmitForSubject(ctx context.Context, c redisstore.Credential, subject string, intent problemreference.ReviewIntent) (problemreference.ReviewReceipt, error) {
	if intent != s.want.ReviewIntent {
		return problemreference.ReviewReceipt{}, problemreference.ErrInvalid
	}
	return s.ReceiptForSubject(ctx, c, subject, intent.RequestKey)
}

func (s *reviewIntakeStub) ReceiptForSubject(_ context.Context, c redisstore.Credential, subject, key string) (problemreference.ReviewReceipt, error) {
	s.calls++
	if c.Value() != token || subject != s.want.Owner || key != s.want.RequestKey {
		return problemreference.ReviewReceipt{}, problemreference.ErrInvalid
	}
	return s.want, s.err
}

func TestReviewIntakeTransport(t *testing.T) {
	const path = "/api/v1/problem-draft-management/review-submissions"
	owner := strings.Repeat("a", 32)
	intent := problemreference.ReviewIntent{RequestKey: "review-original-http-key", DraftID: draftID,
		ReferenceRequestID: strings.Repeat("c", 32), ImportRevision: strings.Repeat("d", 64)}
	stub := &reviewIntakeStub{want: problemreference.ReviewReceipt{Owner: owner, ID: strings.Repeat("b", 32),
		ReviewIntent: intent, ContentVersion: strings.Repeat("e", 64), CreatedAt: time.Unix(100, 0).UTC()}}
	raw, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	post := func(body string) *http.Request {
		r := request(http.MethodPost, path)
		r.Body = io.NopCloser(strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-OJ-Expected-Subject", owner)
		return r
	}
	base := draftHandler(t, &draftUseCase{})
	if _, err := transport.WithReviewIntake(base, stub); err == nil {
		t.Fatal("review intake enabled without reference requests")
	}
	reference := reviewReferenceStub{}
	base, err = transport.WithReferenceRequests(base, reference, reference)
	if err != nil {
		t.Fatal(err)
	}
	response(t, base, post(string(raw)), 404, false)
	h, err := transport.WithReviewIntake(base, stub)
	if err != nil {
		t.Fatal(err)
	}
	r := post(string(raw))
	r.Header.Set("Origin", "https://other.example")
	response(t, h, r, 403, false)
	r = post(string(raw))
	r.Header.Add("X-OJ-Expected-Subject", owner)
	response(t, h, r, 400, false)
	r = post(string(raw))
	r.Header.Del("Cookie")
	response(t, h, r, 401, false)
	for _, extra := range []string{`"draft_id":"duplicate"`, `"formal_verdict":"AC"`} {
		response(t, h, post(string(raw[:len(raw)-1])+","+extra+"}"), 400, false)
	}
	if stub.calls != 0 {
		t.Fatal("rejected request reached review intake")
	}
	w := response(t, h, post(string(raw)), 200, false)
	if strings.Contains(w.Body.String(), owner) || strings.Contains(w.Body.String(), "verdict") {
		t.Fatal("receipt leaked internal evidence or identity")
	}
	get := request(http.MethodGet, path+"/"+intent.RequestKey)
	get.Header.Set("X-OJ-Expected-Subject", owner)
	w2 := response(t, h, get, 200, false)
	if w.Body.String() != w2.Body.String() || stub.calls != 2 {
		t.Fatal("POST and GET did not expose the same receipt")
	}
	stub.err = problemreference.ErrCommitUnknown
	w = response(t, h, post(string(raw)), 503, false)
	if !strings.Contains(w.Body.String(), "REVIEW_WRITE_UNKNOWN") || strings.Contains(w.Body.String(), stub.want.ID) {
		t.Fatal("unknown write was not returned as a recoverable failure")
	}
}
