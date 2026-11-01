package httptransport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
)

func passwordChangeBody(w http.ResponseWriter, r *http.Request) ([]byte, []byte, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		problem(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 提交")
		return nil, nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	defer clear(body)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			problem(w, 413, "BODY_TOO_LARGE", "请求过大")
		} else {
			problem(w, 400, "INVALID_PASSWORD_CHANGE", "无法读取修改请求")
		}
		return nil, nil, false
	}
	invalid := func() ([]byte, []byte, bool) {
		problem(w, 400, "INVALID_PASSWORD_CHANGE", "修改请求格式无效")
		return nil, nil, false
	}
	if !jsoninput.ValidUnicode(body) {
		return invalid()
	}
	d := json.NewDecoder(bytes.NewReader(body))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return invalid()
	}
	values := make(map[string]string, 2)
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		if err != nil || !ok || (name != "current_password" && name != "new_password") {
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
	if err != nil || end != json.Delim('}') || len(values) != 2 {
		return invalid()
	}
	if _, err = d.Token(); err != io.EOF {
		return invalid()
	}
	return []byte(values["current_password"]), []byte(values["new_password"]), true
}
