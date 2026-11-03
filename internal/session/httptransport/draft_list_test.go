package httptransport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const ownListPath = "/api/v1/problem-draft-management/drafts"

type ownListUseCase struct {
	*draftUseCase
	calls            int
	cursor, expected string
}

func (d *ownListUseCase) ListOwnForSubject(_ context.Context, _ redisstore.Credential, cursor, expected string) (problem.OwnDraftPage, error) {
	d.calls++
	d.cursor = cursor
	d.expected = expected
	return problem.OwnDraftPage{Drafts: []problem.ImportReceipt{}}, d.err
}
func ownListHandler(t *testing.T, d *ownListUseCase) http.Handler {
	t.Helper()
	h, err := transport.WithPrivateDrafts(newHandler(t, &fakeStore{}), d, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func TestOwnDraftListCapabilityAndStrictQueries(t *testing.T) {
	d := &ownListUseCase{draftUseCase: &draftUseCase{}}
	h := ownListHandler(t, d)
	response(t, protocolHandler(t, &importProtocolUseCase{draftUseCase: &draftUseCase{}}), protocolRequest("GET", ownListPath, ""), 404, false)
	for _, query := range []string{"?", "?cursor=", "?cursor=a&cursor=b", "?limit=20", "?cursor=a&other=b", "?%63ursor=a", "?cursor=%61", "?cursor=a;other=b", "?cursor=a+b"} {
		r := protocolRequest("GET", ownListPath+query, "")
		r.Header.Set("X-OJ-Expected-Subject", subject)
		response(t, h, r, 400, false)
	}
	if d.calls != 0 {
		t.Fatal("invalid query reached use case")
	}
	for _, values := range [][]string{nil, {subject, subject}, {strings.ToUpper(subject)}, {subject + "," + subject}} {
		r := protocolRequest("GET", ownListPath, "")
		for _, s := range values {
			r.Header.Add("X-OJ-Expected-Subject", s)
		}
		response(t, h, r, 400, false)
	}
	for _, cursor := range []string{"", "abc_DEF-123"} {
		path := ownListPath
		if cursor != "" {
			path += "?cursor=" + cursor
		}
		r := protocolRequest("GET", path, "")
		r.Header.Set("X-OJ-Expected-Subject", subject)
		w := response(t, h, r, 200, false)
		var body map[string]json.RawMessage
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body) != 2 || string(body["drafts"]) != "[]" || string(body["next_cursor"]) != `""` || d.cursor != cursor || d.expected != subject {
			t.Fatal("list shape or precondition changed")
		}
	}
}
func TestOwnListDeterministicCursorErrorAndUnknownPrecedence(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{problem.ErrInvalidCursor, 400, "DRAFT_CURSOR_INVALID"}, {errors.Join(problem.ErrInvalidCursor, problem.ErrStore), 503, "DRAFT_UNAVAILABLE"}, {problem.ErrSubjectChanged, 409, "DRAFT_SUBJECT_CHANGED"}} {
		d := &ownListUseCase{draftUseCase: &draftUseCase{err: tc.err}}
		r := protocolRequest("GET", ownListPath, "")
		r.Header.Set("X-OJ-Expected-Subject", subject)
		w := response(t, ownListHandler(t, d), r, tc.status, false)
		var body map[string]any
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || body["code"] != tc.code {
			t.Fatal("wrong cursor failure")
		}
	}
}
