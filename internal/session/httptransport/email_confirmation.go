package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
)

const (
	emailActivationPath = "/api/v1/account/activate"
	emailResetPath      = "/api/v1/account/reset-password"
)

// EmailConfirmationResult describes a redacted service outcome.
type EmailConfirmationResult uint8

const (
	EmailConfirmationUnavailable EmailConfirmationResult = iota // Includes unknown commit outcomes.
	EmailConfirmationCommitted
	EmailConfirmationInvalidLink
	EmailConfirmationRateLimited
	EmailConfirmationWeakPassword
	EmailConfirmationInvalidPassword
)

// EmailConfirmation consumes a purpose-bound link. The source comes from the
// trusted transport, and the implementation must not retain the password.
type EmailConfirmation func(context.Context, netip.Addr, emailtoken.Purpose, string, []byte) EmailConfirmationResult

// WithEmailConfirmation enables the two public confirmation endpoints.
func WithEmailConfirmation(base http.Handler, confirm EmailConfirmation) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || confirm == nil || h.emailConfirmation != nil {
		return nil, errors.New("invalid email confirmation HTTP configuration")
	}
	next := *h
	next.emailConfirmation = confirm
	return &next, nil
}

// IsEmailConfirmationPath recognizes the two exact confirmation paths.
func IsEmailConfirmationPath(path string) bool {
	return path == emailActivationPath || path == emailResetPath
}

func (h *handler) serveEmailConfirmation(w http.ResponseWriter, r *http.Request, source netip.Addr) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.EscapedPath() != r.URL.Path {
		problem(w, 400, "INVALID_EMAIL_CONFIRMATION", "确认请求格式无效")
		return
	}
	if !source.IsValid() {
		var err error
		source, err = loginSource(r.RemoteAddr)
		if err != nil {
			problem(w, 403, "ORIGIN_REJECTED", "请求来源不允许")
			return
		}
	}
	token, password, ok := emailConfirmationBody(w, r)
	if !ok {
		return
	}
	defer clear(password)
	purpose := emailtoken.Activation
	if r.URL.Path == emailResetPath {
		purpose = emailtoken.Recovery
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	result := h.emailConfirmation(ctx, source, purpose, token, password)
	if ctx.Err() != nil {
		result = EmailConfirmationUnavailable
	}
	switch result {
	case EmailConfirmationCommitted:
		w.WriteHeader(http.StatusNoContent)
	case EmailConfirmationInvalidLink:
		problem(w, 400, "EMAIL_LINK_UNAVAILABLE", "链接无效或已过期")
	case EmailConfirmationRateLimited:
		problem(w, 429, "EMAIL_CONFIRMATION_RATE_LIMITED", "请求过于频繁，请稍后重试")
	case EmailConfirmationWeakPassword:
		problem(w, 400, "WEAK_PASSWORD", "请勿使用常见弱密码")
	case EmailConfirmationInvalidPassword:
		problem(w, 400, "INVALID_NEW_PASSWORD", "密码长度不符合站点要求")
	default:
		problem(w, 503, "EMAIL_CONFIRMATION_UNAVAILABLE", "无法确认操作结果，请先尝试登录")
	}
}

func emailConfirmationBody(w http.ResponseWriter, r *http.Request) (string, []byte, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
		problem(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 提交")
		return "", nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	defer clear(body)
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		problem(w, 413, "BODY_TOO_LARGE", "请求过大")
		return "", nil, false
	}
	if err == nil {
		if token, password, ok := parseEmailConfirmation(body); ok {
			return token, []byte(password), true
		}
	}
	problem(w, 400, "INVALID_EMAIL_CONFIRMATION", "确认请求格式无效")
	return "", nil, false
}

func parseEmailConfirmation(body []byte) (string, string, bool) {
	if !jsoninput.ValidUnicode(body) {
		return "", "", false
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if start, err := d.Token(); err != nil || start != json.Delim('{') {
		return "", "", false
	}
	var token, password string
	for d.More() {
		field, err := d.Token()
		if err != nil {
			return "", "", false
		}
		var value string
		if d.Decode(&value) != nil || value == "" {
			return "", "", false
		}
		switch {
		case field == "token" && token == "":
			token = value
		case field == "new_password" && password == "":
			password = value
		default:
			return "", "", false
		}
	}
	if end, err := d.Token(); err != nil || end != json.Delim('}') || token == "" || password == "" {
		return "", "", false
	}
	_, err := d.Token()
	return token, password, err == io.EOF
}
