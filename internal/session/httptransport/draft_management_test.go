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

const managementSources = "/api/v1/problem-draft-management/sources"
const managementImports = "/api/v1/problem-draft-management/imports"

var subject = strings.Repeat("a", 32)

type managementUseCase struct {
	*importProtocolUseCase
	calls int
}

func managementHandler(t *testing.T, d *managementUseCase) http.Handler {
	t.Helper()
	h, err := transport.WithPrivateDrafts(newHandler(t, &fakeStore{}), d, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func (d *managementUseCase) SourcesForSubject(ctx context.Context, c redisstore.Credential, s string) ([]problem.ImportSource, error) {
	d.calls++
	if s != subject {
		return nil, errors.New("wrong subject")
	}
	return d.Sources(ctx, c)
}
func (d *managementUseCase) ImportPinnedForSubject(ctx context.Context, c redisstore.Credential, key, source, rev, s string) (problem.ImportReceipt, error) {
	d.calls++
	if s != subject {
		return problem.ImportReceipt{}, errors.New("wrong subject")
	}
	return d.ImportPinned(ctx, c, key, source, rev)
}
func (d *managementUseCase) ReceiptForSubject(ctx context.Context, c redisstore.Credential, key, s string) (problem.ImportReceipt, error) {
	d.calls++
	if s != subject {
		return problem.ImportReceipt{}, errors.New("wrong subject")
	}
	return d.Receipt(ctx, c, key)
}

func TestManagementCapabilityHeadersAndDispatch(t *testing.T) {
	d := &managementUseCase{importProtocolUseCase: &importProtocolUseCase{draftUseCase: &draftUseCase{}}}
	h := managementHandler(t, d)
	for _, path := range []string{managementSources, managementImports, managementImports + "/original-draft-http-key"} {
		method := "GET"
		if path == managementImports {
			method = "POST"
		}
		response(t, protocolHandler(t, d.importProtocolUseCase), protocolRequest(method, path, pinnedJSON), 404, false)
		for _, values := range [][]string{nil, {subject, subject}, {subject + "," + subject}, {strings.ToUpper(subject)}, {"short"}} {
			r := protocolRequest(method, path, pinnedJSON)
			for _, value := range values {
				r.Header.Add("X-OJ-Expected-Subject", value)
			}
			response(t, h, r, 400, false)
		}
		r := protocolRequest(method, path, pinnedJSON)
		r.Header.Set("X-OJ-Expected-Subject", subject)
		response(t, h, r, 200, false)
	}
	if d.calls != 3 || d.sources != 1 || d.pinned != 1 || d.receipts != 1 {
		t.Fatal("incorrect dispatch")
	}
	for _, path := range []string{managementSources + "?", managementImports + "/%6friginal-draft-http-key"} {
		r := protocolRequest("GET", path, "")
		r.Header.Set("X-OJ-Expected-Subject", subject)
		response(t, h, r, 400, false)
	}
}

func TestManagementSubjectErrorPrecedence(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{{problem.ErrSubjectChanged, 409, "DRAFT_SUBJECT_CHANGED"}, {errors.Join(problem.ErrSubjectChanged, redisstore.ErrUnknown), 503, "DRAFT_UNAVAILABLE"}} {
		d := &managementUseCase{importProtocolUseCase: &importProtocolUseCase{draftUseCase: &draftUseCase{err: tc.err}}}
		r := protocolRequest(http.MethodGet, managementSources, "")
		r.Header.Set("X-OJ-Expected-Subject", subject)
		w := response(t, managementHandler(t, d), r, tc.status, false)
		var body map[string]any
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || body["code"] != tc.code {
			t.Fatal("wrong error response")
		}
	}
}
