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
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const sourcePath = "/api/v1/problem-draft-sources"
const importPath = "/api/v1/problem-draft-imports"

var expectedRevision = strings.Repeat("a", 64)
var pinnedJSON = `{"source_id":"library-checker:aplusb","request_key":"original-draft-http-key","expected_revision":"` + expectedRevision + `"}`

type importProtocolUseCase struct {
	*draftUseCase
	sources, pinned, receipts int
}

func (d *importProtocolUseCase) Sources(ctx context.Context, c redisstore.Credential) ([]problem.ImportSource, error) {
	d.sources++
	_, err := d.call(ctx, c)
	return []problem.ImportSource{{SourceID: "library-checker:aplusb", Title: "私有候选", Version: strings.Repeat("b", 64), ImportRevision: expectedRevision}}, err
}
func (d *importProtocolUseCase) ImportPinned(ctx context.Context, c redisstore.Credential, key, source, revision string) (problem.ImportReceipt, error) {
	d.pinned++
	if key != "original-draft-http-key" || source != "library-checker:aplusb" || revision != expectedRevision {
		return problem.ImportReceipt{}, errors.New("pinned body changed")
	}
	return d.summary(ctx, c)
}
func (d *importProtocolUseCase) Receipt(ctx context.Context, c redisstore.Credential, key string) (problem.ImportReceipt, error) {
	d.receipts++
	if key != "original-draft-http-key" {
		return problem.ImportReceipt{}, errors.New("receipt key changed")
	}
	return d.summary(ctx, c)
}
func (d *importProtocolUseCase) summary(ctx context.Context, c redisstore.Credential) (problem.ImportReceipt, error) {
	_, err := d.call(ctx, c)
	return problem.ImportReceipt{DraftID: draftID, RequestKey: "original-draft-http-key", SourceID: "library-checker:aplusb", Version: strings.Repeat("b", 64), ImportRevision: expectedRevision, State: "private-draft", ValidationState: "not-run", CreatedAt: time.Unix(1000, 0).UTC()}, err
}
func protocolHandler(t *testing.T, d *importProtocolUseCase) http.Handler {
	t.Helper()
	h, err := transport.WithPrivateDrafts(newHandler(t, &fakeStore{}), d, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func protocolRequest(method, path, body string) *http.Request {
	r := request(method, path)
	r.Body = io.NopCloser(strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestImportProtocolSummaryAndOptionalCapability(t *testing.T) {
	d := &importProtocolUseCase{draftUseCase: &draftUseCase{}}
	h := protocolHandler(t, d)
	for _, path := range []string{sourcePath, importPath, importPath + "/original-draft-http-key"} {
		method := "GET"
		if path == importPath {
			method = "POST"
		}
		response(t, draftHandler(t, &draftUseCase{}), protocolRequest(method, path, pinnedJSON), 404, false)
	}
	w := response(t, h, protocolRequest("GET", sourcePath, ""), 200, false)
	var source struct {
		Sources []map[string]any `json:"sources"`
	}
	if json.Unmarshal(w.Body.Bytes(), &source) != nil || len(source.Sources) != 1 || len(source.Sources[0]) != 4 || source.Sources[0]["import_revision"] != expectedRevision || source.Sources[0]["title"] != "私有候选" {
		t.Fatal("catalog did not return detached exact summary")
	}
	created := response(t, h, protocolRequest("POST", importPath, pinnedJSON), 200, false)
	read := response(t, h, protocolRequest("GET", importPath+"/original-draft-http-key", ""), 200, false)
	if created.Body.String() != read.Body.String() || created.Header().Get("Content-Type") != "application/json" {
		t.Fatal("committed import summary and receipt differ")
	}
	var receipt map[string]any
	if json.Unmarshal(read.Body.Bytes(), &receipt) != nil || len(receipt) != 8 || receipt["draft_id"] != draftID || receipt["import_revision"] != expectedRevision || receipt["document"] != nil || receipt["references"] != nil || receipt["owner"] != nil {
		t.Fatal("receipt disclosed general private metadata")
	}
	if read.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("private import enabled CORS")
	}
	// No missing pin can silently fall back to the original two-field API.
	response(t, h, protocolRequest("POST", importPath, draftJSON), 400, false)
	response(t, h, draftRequest(pinnedJSON), 400, false)
	response(t, h, draftRequest(draftJSON), 200, false)
	if d.sources != 1 || d.pinned != 1 || d.receipts != 1 || d.imports != 1 {
		t.Fatal("unexpected compatibility or use-case calls")
	}
}

func TestPinnedBodyStrictnessAndNoRejectedCalls(t *testing.T) {
	d := &importProtocolUseCase{draftUseCase: &draftUseCase{}}
	h := protocolHandler(t, d)
	for _, body := range []string{
		draftJSON, strings.Replace(pinnedJSON, "expected_revision", "expected_version", 1),
		strings.Replace(pinnedJSON, "expected_revision", "Expected_Revision", 1),
		strings.TrimSuffix(pinnedJSON, "}") + `,"expected_revision":"` + expectedRevision + `"}`,
		strings.Replace(pinnedJSON, expectedRevision, strings.Repeat("A", 64), 1),
		strings.Replace(pinnedJSON, expectedRevision, "", 1),
		strings.Replace(pinnedJSON, `"`+expectedRevision+`"`, "null", 1),
		strings.Replace(pinnedJSON, `"`+expectedRevision+`"`, "42", 1),
		strings.Replace(pinnedJSON, expectedRevision, `\ud800`, 1),
		pinnedJSON + "{}", `[]`, "null", `{`,
	} {
		response(t, h, protocolRequest("POST", importPath, body), 400, false)
	}
	response(t, h, protocolRequest("POST", importPath, strings.Repeat("x", 1025)), 413, false)
	r := protocolRequest("POST", importPath, pinnedJSON)
	r.Header.Set("Content-Type", "text/plain")
	response(t, h, r, 415, false)
	if d.pinned != 0 || d.imports != 0 {
		t.Fatal("rejected pin reached an import")
	}
}

func TestImportProtocolRouteAndTransportBoundaries(t *testing.T) {
	d := &importProtocolUseCase{draftUseCase: &draftUseCase{}}
	h := protocolHandler(t, d)
	for _, path := range []string{sourcePath + "/", importPath + "/", importPath + "/short", importPath + "/original-draft-http-key/extra"} {
		response(t, h, protocolRequest("GET", path, ""), 404, false)
	}
	for _, path := range []string{sourcePath + "?owner=another", sourcePath + "?", importPath + "/%6friginal-draft-http-key"} {
		response(t, h, protocolRequest("GET", path, ""), 400, false)
	}
	for _, path := range []string{sourcePath, importPath, importPath + "/original-draft-http-key"} {
		method, wrongMethod, allow := "GET", "POST", "GET"
		if path == importPath {
			method, wrongMethod, allow = "POST", "GET", "POST"
		}
		w := response(t, h, protocolRequest(wrongMethod, path, pinnedJSON), 405, false)
		if w.Header().Get("Allow") != allow {
			t.Fatal("wrong method grant")
		}
		r := protocolRequest(method, path, pinnedJSON)
		r.Header.Del("Cookie")
		response(t, h, r, 401, false)
		r = protocolRequest(method, path, pinnedJSON)
		r.TLS = nil
		response(t, h, r, 403, false)
	}
	r := protocolRequest("POST", importPath, pinnedJSON)
	r.Header.Set("Origin", "https://other.invalid")
	response(t, h, r, 403, false)
	if d.sources+d.receipts+d.pinned != 0 {
		t.Fatal("untrusted route reached import protocol")
	}
}

func TestImportProtocolErrorsAndCancellationReturnNoPartialSummary(t *testing.T) {
	for _, operation := range []string{"sources", "import", "receipt"} {
		for _, test := range []struct {
			err    error
			status int
			code   string
		}{
			{problem.ErrSourceChanged, 409, "DRAFT_SOURCE_CHANGED"}, {problem.ErrNotFound, 404, "DRAFT_NOT_FOUND"},
			{identity.ErrForbidden, 403, "PROBLEM_AUTHOR_REQUIRED"}, {redisstore.ErrStale, 401, "SESSION_REQUIRED"},
			{errors.Join(problem.ErrSourceChanged, problem.ErrCommitUnknown), 503, "DRAFT_UNAVAILABLE"},
		} {
			t.Run(operation+"/"+test.code, func(t *testing.T) {
				d := &importProtocolUseCase{draftUseCase: &draftUseCase{err: test.err}}
				method, path := "GET", sourcePath
				if operation == "import" {
					method, path = "POST", importPath
				}
				if operation == "receipt" {
					path = importPath + "/original-draft-http-key"
				}
				w := response(t, protocolHandler(t, d), protocolRequest(method, path, pinnedJSON), test.status, false)
				var body map[string]any
				if json.Unmarshal(w.Body.Bytes(), &body) != nil || body["code"] != test.code || body["sources"] != nil || body["draft_id"] != nil {
					t.Fatal("error included partial private data")
				}
			})
		}
		ctx, cancel := context.WithCancel(t.Context())
		d := &importProtocolUseCase{draftUseCase: &draftUseCase{operation: func(context.Context) { cancel() }}}
		method, path := "GET", sourcePath
		if operation == "import" {
			method, path = "POST", importPath
		}
		if operation == "receipt" {
			path = importPath + "/original-draft-http-key"
		}
		response(t, protocolHandler(t, d), protocolRequest(method, path, pinnedJSON).WithContext(ctx), 503, false)
		cancel()
	}
}
