package httptransport_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const draftURL = "/api/v1/problem-drafts"
const draftID = "11111111111111111111111111111111"
const draftJSON = `{"source_id":"library-checker:aplusb","request_key":"original-draft-http-key"}`

type draftUseCase struct {
	imports, gets int
	err           error
	operation     func(context.Context)
}

func (d *draftUseCase) call(ctx context.Context, c redisstore.Credential) (problem.Draft, error) {
	if c.Value() != token {
		return problem.Draft{}, errors.New("wrong opaque credential")
	}
	if _, ok := ctx.Deadline(); !ok {
		return problem.Draft{}, errors.New("unbounded draft operation")
	}
	if d.operation != nil {
		d.operation(ctx)
	}
	return problem.Draft{ID: draftID, RequestKey: "original-draft-http-key", SourceID: "library-checker:aplusb", State: "private-draft", ValidationState: "not-run", Document: []byte(`{"title":"私有题目"}`)}, d.err
}
func (d *draftUseCase) Import(ctx context.Context, c redisstore.Credential, key, source string) (problem.Draft, error) {
	d.imports++
	if key != "original-draft-http-key" || source != "library-checker:aplusb" {
		return problem.Draft{}, errors.New("body changed")
	}
	return d.call(ctx, c)
}
func (d *draftUseCase) GetOwn(ctx context.Context, c redisstore.Credential, id string) (problem.Draft, error) {
	d.gets++
	if id != draftID {
		return problem.Draft{}, errors.New("owner lookup changed")
	}
	return d.call(ctx, c)
}

func draftHandler(t *testing.T, d *draftUseCase) http.Handler {
	t.Helper()
	h, err := transport.WithPrivateDrafts(newHandler(t, &fakeStore{}), d, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func draftRequest(body string) *http.Request {
	r := request("POST", draftURL)
	r.Body = io.NopCloser(strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestDraftResponseAndOptionalRoutes(t *testing.T) {
	d := &draftUseCase{}
	base := newHandler(t, &fakeStore{})
	h, err := transport.WithPrivateDrafts(base, d, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response(t, base, draftRequest(draftJSON), 404, false)
	w := response(t, h, draftRequest(draftJSON), 200, false)
	var got struct {
		ID       string         `json:"id"`
		Document map[string]any `json:"document"`
	}
	if json.Unmarshal(w.Body.Bytes(), &got) != nil || got.ID != draftID || got.Document["title"] != "私有题目" || strings.Contains(w.Body.String(), "owner") {
		t.Fatal("draft metadata not detached JSON")
	}
	response(t, h, request("GET", draftURL+"/"+draftID), 200, false)
	if d.imports != 1 || d.gets != 1 {
		t.Fatal("unexpected use-case calls")
	}
	for _, path := range []string{draftURL + "/", draftURL + "/" + draftID + "/", draftURL + "/" + strings.Repeat("A", 32), draftURL + "/" + draftID + "/materials"} {
		response(t, h, request("GET", path), 404, false)
	}
	for _, path := range []string{draftURL, draftURL + "/" + draftID} {
		w := response(t, h, request("DELETE", path), 405, false)
		if w.Header().Get("Allow") == "" {
			t.Fatal("missing allowed method")
		}
	}
}

func TestDraftPreflightAndStrictBody(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		status int
		change func(*http.Request)
	}{
		{"plain", draftJSON, 403, func(r *http.Request) { r.TLS = nil }},
		{"host", draftJSON, 403, func(r *http.Request) { r.Host = "other.invalid" }},
		{"cross-origin", draftJSON, 403, func(r *http.Request) { r.Header.Set("Origin", "https://evil.invalid") }},
		{"missing-origin", draftJSON, 403, func(r *http.Request) { r.Header.Del("Origin") }},
		{"duplicate-origin", draftJSON, 403, func(r *http.Request) { r.Header.Add("Origin", origin) }},
		{"cross-site", draftJSON, 403, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }},
		{"no-cookie", draftJSON, 401, func(r *http.Request) { r.Header.Del("Cookie") }},
		{"duplicate-cookie", draftJSON, 401, func(r *http.Request) { r.AddCookie(&http.Cookie{Name: transport.CookieName, Value: token}) }},
		{"media", draftJSON, 415, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }},
		{"oversize", strings.Repeat(" ", 1025), 413, nil},
		{"missing", `{}`, 400, nil},
		{"array", `[]`, 400, nil},
		{"duplicate", `{"source_id":"a","source_id":"b","request_key":"original-draft-http-key"}`, 400, nil},
		{"case-alias", `{"Source_id":"a","request_key":"original-draft-http-key"}`, 400, nil},
		{"path", `{"source_id":"a","request_key":"original-draft-http-key","path":"/private"}`, 400, nil},
		{"null", `{"source_id":null,"request_key":"original-draft-http-key"}`, 400, nil},
		{"key", `{"source_id":"a","request_key":"short"}`, 400, nil},
		{"trailing", draftJSON + `{}`, 400, nil},
		{"unicode", `{"source_id":"\ud800","request_key":"original-draft-http-key"}`, 400, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &draftUseCase{}
			r := draftRequest(tc.body)
			if tc.change != nil {
				tc.change(r)
			}
			response(t, draftHandler(t, d), r, tc.status, false)
			if d.imports != 0 || d.gets != 0 {
				t.Fatal("rejected transport reached application")
			}
		})
	}
}

func TestDraftErrorOutcomes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{redisstore.ErrStale, 401}, {identity.ErrForbidden, 403}, {problem.ErrNotFound, 404},
		{problem.ErrUnknownSource, 400}, {problem.ErrConflict, 409}, {problem.ErrCommitUnknown, 503},
		{problem.ErrInvalid, 503}, {problem.ErrBusy, 503}, {materialstore.ErrIntegrity, 503},
		{errors.Join(identity.ErrForbidden, materialstore.ErrCommitUnknown), 503},
		{errors.Join(identity.ErrForbidden, materialstore.ErrIntegrity), 503},
		{errors.Join(redisstore.ErrStale, context.DeadlineExceeded), 503},
		{errors.Join(redisstore.ErrStale, identity.ErrGuardedOperation), 401},
		{errors.Join(problem.ErrConflict, identity.ErrGuardedOperation), 503},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			d := &draftUseCase{err: tc.err}
			response(t, draftHandler(t, d), draftRequest(draftJSON), tc.status, false)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	d := &draftUseCase{operation: func(context.Context) { cancel() }}
	response(t, draftHandler(t, d), draftRequest(draftJSON).WithContext(ctx), 503, false)
}
