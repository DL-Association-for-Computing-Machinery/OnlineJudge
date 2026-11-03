package httptransport_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type previewUseCase struct {
	*draftUseCase
	previews int
}

func (p *previewUseCase) Preview(ctx context.Context, credential redisstore.Credential, id string) (problem.Preview, error) {
	p.previews++
	_, err := p.call(ctx, credential)
	if id != draftID {
		return problem.Preview{}, problem.ErrNotFound
	}
	return problem.Preview{DraftID: id, Title: "题面", Language: "en", Markdown: "<script>data</script>\r\n", Examples: []problem.Example{{ID: "case-0001", Input: "\ufeff \t\r\n\x00", Output: ""}}}, err
}

func TestDraftPreviewCapabilityAndExactJSONText(t *testing.T) {
	p := &previewUseCase{draftUseCase: &draftUseCase{}}
	h, err := transport.WithPrivateDrafts(newHandler(t, &fakeStore{}), p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	path := draftURL + "/" + draftID + "/preview"
	response(t, draftHandler(t, &draftUseCase{}), request("GET", path), 404, false)
	w := response(t, h, request("GET", path), 200, false)
	var result problem.Preview
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Markdown != "<script>data</script>\r\n" ||
		len(result.Examples) != 1 || result.Examples[0].Input != "\ufeff \t\r\n\x00" || result.Examples[0].Output != "" || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal("preview was rendered or text bytes changed")
	}
	response(t, h, request("POST", path), 405, false)
	for _, suffix := range []string{"/", "/materials", "/preview"} {
		response(t, h, request("GET", path+suffix), 404, false)
	}
	bad := request("GET", path)
	bad.Header.Del("Cookie")
	response(t, h, bad, 401, false)
	bad = request("GET", path)
	bad.TLS = nil
	response(t, h, bad, 403, false)
	if p.previews != 1 {
		t.Fatal("rejected route reached preview use case")
	}
	ctx, cancel := context.WithCancel(t.Context())
	p.operation = func(context.Context) { cancel() }
	w = response(t, h, request("GET", path).WithContext(ctx), 503, false)
	var failure map[string]any
	if json.Unmarshal(w.Body.Bytes(), &failure) != nil || failure["markdown"] != nil {
		t.Fatal("failed preview returned partial private text")
	}
}
