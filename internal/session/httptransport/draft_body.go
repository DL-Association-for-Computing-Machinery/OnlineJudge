package httptransport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
	draftproblem "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func draftBody(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	values, ok := draftFields(w, r, false)
	return values["request_key"], values["source_id"], ok
}

func pinnedDraftBody(w http.ResponseWriter, r *http.Request) (string, string, string, bool) {
	values, ok := draftFields(w, r, true)
	return values["request_key"], values["source_id"], values["expected_revision"], ok
}

func draftFields(w http.ResponseWriter, r *http.Request, pinned bool) (map[string]string, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		problem(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 提交")
		return nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			problem(w, 413, "BODY_TOO_LARGE", "请求过大")
		} else {
			problem(w, 400, "INVALID_DRAFT_REQUEST", "无法读取草稿请求")
		}
		return nil, false
	}
	invalid := func() (map[string]string, bool) {
		problem(w, 400, "INVALID_DRAFT_REQUEST", "草稿请求格式无效")
		return nil, false
	}
	if !jsoninput.ValidUnicode(body) {
		return invalid()
	}
	d := json.NewDecoder(bytes.NewReader(body))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return invalid()
	}
	fields := 2
	if pinned {
		fields = 3
	}
	values := make(map[string]string, fields)
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		allowed := name == "source_id" || name == "request_key" || (pinned && name == "expected_revision")
		if err != nil || !ok || !allowed {
			return invalid()
		}
		if _, exists := values[name]; exists {
			return invalid()
		}
		var value string
		if d.Decode(&value) != nil || value == "" {
			return invalid()
		}
		values[name] = value
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') || len(values) != fields || !draftproblem.ValidKey(values["request_key"]) || len(values["source_id"]) > 128 ||
		(pinned && !draftproblem.ValidImportRevision(values["expected_revision"])) {
		return invalid()
	}
	if _, err = d.Token(); err != io.EOF {
		return invalid()
	}
	return values, true
}
