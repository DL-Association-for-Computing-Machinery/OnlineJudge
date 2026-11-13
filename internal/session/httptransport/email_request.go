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

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
)

const (
	emailRegisterPath        = "/api/v1/account/register"
	emailRecoveryRequestPath = "/api/v1/account/request-password-reset"
)

// EmailRequest accepts a request for registration, activation resend, or
// password recovery. It must honor the context and complete the public intake
// without waiting for account-dependent work or delivery. An error is reserved
// for target-independent service unavailability; a quota denial is accepted.
type EmailRequest func(context.Context, netip.Addr, emailtoken.Purpose, identity.Email) error

// WithEmailRequest enables both public mail request endpoints.
func WithEmailRequest(base http.Handler, request EmailRequest) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || request == nil || h.emailRequest != nil {
		return nil, errors.New("invalid email request HTTP configuration")
	}
	next := *h
	next.emailRequest = request
	return &next, nil
}

// IsEmailRequestPath recognizes only the two exact request routes.
func IsEmailRequestPath(path string) bool {
	return path == emailRegisterPath || path == emailRecoveryRequestPath
}

func (h *handler) serveEmailRequest(w http.ResponseWriter, r *http.Request, source netip.Addr) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.EscapedPath() != r.URL.Path {
		invalidEmailRequest(w)
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
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
		problem(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 提交")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2048))
	defer clear(body)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		problem(w, 413, "BODY_TOO_LARGE", "请求过大")
		return
	}
	email, ok := parseEmailRequest(body)
	if err != nil || !ok {
		invalidEmailRequest(w)
		return
	}
	purpose := emailtoken.Activation
	if r.URL.Path == emailRecoveryRequestPath {
		purpose = emailtoken.Recovery
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	// Once intake confirms its outcome, late cancellation does not undo it.
	if err := h.emailRequest(ctx, source, purpose, email); err != nil {
		problem(w, 503, "EMAIL_REQUEST_UNAVAILABLE", "邮箱申请暂不可用")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func invalidEmailRequest(w http.ResponseWriter) {
	problem(w, 400, "INVALID_EMAIL_REQUEST", "请输入有效邮箱")
}

func parseEmailRequest(body []byte) (identity.Email, bool) {
	if !jsoninput.ValidUnicode(body) {
		return identity.Email{}, false
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if start, err := d.Token(); err != nil || start != json.Delim('{') {
		return identity.Email{}, false
	}
	if field, err := d.Token(); err != nil || field != "email" {
		return identity.Email{}, false
	}
	var raw string
	if d.Decode(&raw) != nil {
		return identity.Email{}, false
	}
	if end, err := d.Token(); err != nil || end != json.Delim('}') {
		return identity.Email{}, false
	}
	if _, err := d.Token(); err != io.EOF {
		return identity.Email{}, false
	}
	address, err := identity.ParseEmail(raw)
	return address, err == nil
}
