package httptransport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
)

func loginBody(w http.ResponseWriter, r *http.Request) (identity.Email, []byte, error) {
	bad := errors.New("invalid login body")
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		problem(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 提交")
		return identity.Email{}, nil, bad
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	defer clear(body)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			problem(w, 413, "BODY_TOO_LARGE", "请求过大")
		} else {
			problem(w, 400, "INVALID_LOGIN", "无法读取登录请求")
		}
		return identity.Email{}, nil, bad
	}
	if !jsoninput.ValidUnicode(body) {
		problem(w, 400, "INVALID_LOGIN", "登录请求格式无效")
		return identity.Email{}, nil, bad
	}
	values := make(map[string]string, 2)
	dec := json.NewDecoder(bytes.NewReader(body))
	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		problem(w, 400, "INVALID_LOGIN", "登录请求格式无效")
		return identity.Email{}, nil, bad
	}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return invalidLoginBody(w, bad)
		}
		name, ok := key.(string)
		if !ok || (name != "email" && name != "password") {
			return invalidLoginBody(w, bad)
		}
		if _, exists := values[name]; exists {
			return invalidLoginBody(w, bad)
		}
		var value string
		if dec.Decode(&value) != nil {
			return invalidLoginBody(w, bad)
		}
		values[name] = value
	}
	if _, err = dec.Token(); err != nil || len(values) != 2 {
		return invalidLoginBody(w, bad)
	}
	if _, err = dec.Token(); err != io.EOF {
		return invalidLoginBody(w, bad)
	}
	email, err := identity.ParseEmail(values["email"])
	if err != nil || values["password"] == "" {
		return invalidLoginBody(w, bad)
	}
	return email, []byte(values["password"]), nil
}

func invalidLoginBody(w http.ResponseWriter, err error) (identity.Email, []byte, error) {
	problem(w, 400, "INVALID_LOGIN", "登录请求格式无效")
	return identity.Email{}, nil, err
}
