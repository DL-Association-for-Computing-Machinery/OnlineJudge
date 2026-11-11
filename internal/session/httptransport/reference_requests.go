package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
	draftproblem "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const referenceCollection = "/api/v1/problem-draft-management/reference-requests"

// ReferenceRequests is the ordinary-session, subject-bound author capability.
type ReferenceRequests interface {
	EnqueueForSubject(context.Context, redisstore.Credential, string, string, string) (problemreference.Request, error)
	ReceiptForSubject(context.Context, redisstore.Credential, string, string) (problemreference.Request, error)
	CancelForSubject(context.Context, redisstore.Credential, string, string) (problemreference.Request, error)
}

// ReferencePreparation starts bounded work and exposes only durable readiness.
type ReferencePreparation interface {
	Start(context.Context, redisstore.Credential, string, string) (bool, error)
	Ready(context.Context, redisstore.Credential, string, string) (bool, error)
}

// WithReferenceRequests enables the capability on a private draft handler copy.
func WithReferenceRequests(base http.Handler, requests ReferenceRequests, preparation ReferencePreparation) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || h.drafts == nil || h.references != nil || requests == nil || preparation == nil {
		return nil, redisstore.ErrInvalid
	}
	return &handler{store: h.store, config: h.config, host: h.host, login: h.login, proxy: h.proxy,
		passwordChange: h.passwordChange, drafts: h.drafts, draftTimeout: h.draftTimeout, references: requests, preparation: preparation}, nil
}

// IsReferenceRequestPath recognizes route shapes without granting the capability.
func IsReferenceRequestPath(path string) bool {
	method, _ := referenceRequestRoute(path)
	return method != ""
}

func referenceRequestRoute(path string) (method, key string) {
	if path == referenceCollection {
		return http.MethodPost, ""
	}
	tail, ok := strings.CutPrefix(path, referenceCollection+"/")
	if !ok {
		return "", ""
	}
	if key, ok := strings.CutSuffix(tail, "/plan"); ok && draftproblem.ValidKey(key) {
		return http.MethodGet, key
	}
	key, mutation := strings.CutSuffix(tail, "/prepare")
	if !mutation {
		key, mutation = strings.CutSuffix(tail, "/cancel")
	}
	if !draftproblem.ValidKey(key) {
		return "", ""
	}
	if mutation {
		return http.MethodPost, key
	}
	return http.MethodGet, key
}

func (h *handler) serveReferenceRequest(w http.ResponseWriter, r *http.Request, key string) {
	expected, ok := expectedSubject(r)
	if !ok || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		invalidReferenceRequest(w)
		return
	}
	credential, err := requestCredential(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "SESSION_REQUIRED", "会话无效或已过期")
		return
	}
	var draftID string
	if key == "" {
		key, draftID, ok = referenceRequestBody(w, r)
		if !ok {
			return
		}
	} else if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		invalidReferenceRequest(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.draftTimeout)
	defer cancel()
	if strings.HasSuffix(r.URL.Path, "/prepare") || strings.HasSuffix(r.URL.Path, "/plan") {
		h.serveReferencePreparation(ctx, w, r.Method, credential, expected, key)
		return
	}
	var result problemreference.Request
	switch {
	case draftID != "":
		result, err = h.references.EnqueueForSubject(ctx, credential, expected, key, draftID)
	case r.Method == http.MethodPost:
		result, err = h.references.CancelForSubject(ctx, credential, expected, key)
	default:
		result, err = h.references.ReceiptForSubject(ctx, credential, expected, key)
	}
	if err != nil {
		referenceRequestError(w, err)
		return
	}
	if ctx.Err() != nil || result.RequestKey != key || result.Owner != expected ||
		(draftID != "" && result.DraftID != draftID) || !identity.ValidID(result.ID) || !identity.ValidID(result.DraftID) ||
		(result.State != problemreference.Queued && result.State != problemreference.Cancelled && result.State != problemreference.Reported) ||
		(result.CancelledAt != nil) != (result.State == problemreference.Cancelled) || result.CreatedAt.IsZero() {
		referenceRequestError(w, problemreference.ErrStore)
		return
	}
	body, err := json.Marshal(struct {
		ID          string     `json:"id"`
		RequestKey  string     `json:"request_key"`
		DraftID     string     `json:"draft_id"`
		State       string     `json:"state"`
		CreatedAt   time.Time  `json:"created_at"`
		CancelledAt *time.Time `json:"cancelled_at"`
	}{result.ID, result.RequestKey, result.DraftID, result.State, result.CreatedAt, result.CancelledAt})
	if err != nil || ctx.Err() != nil {
		referenceRequestError(w, problemreference.ErrStore)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *handler) serveReferencePreparation(ctx context.Context, w http.ResponseWriter, method string, credential redisstore.Credential, expected, key string) {
	var ready bool
	var err error
	if method == http.MethodPost {
		ready, err = h.preparation.Start(ctx, credential, expected, key)
	} else {
		ready, err = h.preparation.Ready(ctx, credential, expected, key)
	}
	if err = errors.Join(err, ctx.Err()); err != nil {
		referenceRequestError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if ready {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ready":true}`))
		return
	}
	if method == http.MethodPost {
		w.WriteHeader(http.StatusAccepted)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_, _ = w.Write([]byte(`{"ready":false}`))
}

func referenceRequestBody(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		problem(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 提交")
		return "", "", false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		problem(w, 413, "BODY_TOO_LARGE", "请求过大")
		return "", "", false
	}
	if err == nil && jsoninput.ValidUnicode(body) {
		// ValidUnicode also validates the entire JSON syntax, including trailing data.
		d := json.NewDecoder(bytes.NewReader(body))
		start, _ := d.Token()
		if start == json.Delim('{') {
			fields := make(map[string]string, 2)
			for d.More() {
				token, _ := d.Token()
				name, _ := token.(string)
				var value string
				if (name != "request_key" && name != "draft_id") || fields[name] != "" || d.Decode(&value) != nil || value == "" {
					invalidReferenceRequest(w)
					return "", "", false
				}
				fields[name] = value
			}
			if draftproblem.ValidKey(fields["request_key"]) && identity.ValidID(fields["draft_id"]) {
				return fields["request_key"], fields["draft_id"], true
			}
		}
	}
	invalidReferenceRequest(w)
	return "", "", false
}

func invalidReferenceRequest(w http.ResponseWriter) {
	problem(w, 400, "INVALID_REFERENCE_REQUEST", "参考验证申请格式无效")
}

func referenceRequestError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, problemreference.ErrCommitUnknown):
		problem(w, 503, "REFERENCE_WRITE_UNKNOWN", "结果未确认，请使用原请求键查看回执")
	case errors.Is(err, problemreference.ErrStore), errors.Is(err, problemreference.ErrPolicy),
		errors.Is(err, identity.ErrStore), errors.Is(err, identity.ErrRecovering), errors.Is(err, redisstore.ErrStore),
		errors.Is(err, redisstore.ErrUnknown), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		problem(w, 503, "REFERENCE_UNAVAILABLE", "参考验证申请暂不可用")
	case sessionDenied(err):
		problem(w, 401, "SESSION_REQUIRED", "会话无效或已过期")
	case errors.Is(err, identity.ErrForbidden):
		problem(w, 403, "PROBLEM_AUTHOR_REQUIRED", "需要出题权限")
	case errors.Is(err, problemreference.ErrSubjectChanged):
		problem(w, 409, "REFERENCE_SUBJECT_CHANGED", "会话用户已变化")
	case errors.Is(err, problemreference.ErrNotFound):
		problem(w, 404, "REFERENCE_NOT_FOUND", "草稿或参考验证申请不存在")
	case errors.Is(err, problemreference.ErrConflict):
		problem(w, 409, "REFERENCE_KEY_CONFLICT", "请求键已用于其他草稿")
	case errors.Is(err, problemreference.ErrSourceChanged):
		problem(w, 409, "REFERENCE_SOURCE_CHANGED", "当前草稿版本不可申请参考验证")
	case errors.Is(err, problemreference.ErrFull):
		problem(w, 429, "REFERENCE_QUEUE_FULL", "参考验证申请队列已满")
	case errors.Is(err, problemreference.ErrPreparationBusy):
		problem(w, 429, "REFERENCE_PREPARATION_BUSY", "材料准备繁忙，请稍后重试")
	case errors.Is(err, problemreference.ErrAuthorityChanged), errors.Is(err, problemreference.ErrNotQueued):
		problem(w, 409, "REFERENCE_PREPARATION_DENIED", "当前申请不可准备材料")
	default:
		problem(w, 503, "REFERENCE_UNAVAILABLE", "参考验证申请暂不可用")
	}
}
