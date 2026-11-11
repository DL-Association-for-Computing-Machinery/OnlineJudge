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

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
	draftproblem "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const reviewIntakeCollection = "/api/v1/problem-draft-management/review-submissions"

// ReviewIntake exposes only the subject-bound author write and receipt.
type ReviewIntake interface {
	SubmitForSubject(context.Context, redisstore.Credential, string, problemreference.ReviewIntent) (problemreference.ReviewReceipt, error)
	ReceiptForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewReceipt, error)
	AuthorReturnForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewReturnStatus, error)
}

// WithReviewIntake enables intake on the configured private author transport.
func WithReviewIntake(base http.Handler, intake ReviewIntake) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || h.references == nil || h.reviewIntake != nil || intake == nil {
		return nil, redisstore.ErrInvalid
	}
	return &handler{store: h.store, config: h.config, host: h.host, login: h.login, proxy: h.proxy,
		passwordChange: h.passwordChange, drafts: h.drafts, draftTimeout: h.draftTimeout,
		references: h.references, preparation: h.preparation, reviewIntake: intake}, nil
}

// IsReviewIntakePath only recognizes paths; it does not enable them.
func IsReviewIntakePath(path string) bool {
	method, _, _ := reviewIntakeRoute(path)
	return method != ""
}

func reviewIntakeRoute(path string) (method, key string, returned bool) {
	if path == reviewIntakeCollection {
		return http.MethodPost, "", false
	}
	if tail, ok := strings.CutPrefix(path, reviewIntakeCollection+"/"); ok {
		key, returned := strings.CutSuffix(tail, "/return")
		if draftproblem.ValidKey(key) {
			return http.MethodGet, key, returned
		}
	}
	return "", "", false
}

func (h *handler) serveReviewIntake(w http.ResponseWriter, r *http.Request, key string, returned bool) {
	expected, ok := expectedSubject(r)
	if !ok || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		invalidReviewIntake(w)
		return
	}
	credential, err := requestCredential(r)
	if err != nil {
		sessionProblemReview(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.draftTimeout)
	defer cancel()
	if returned {
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			invalidReviewIntake(w)
			return
		}
		result, err := h.reviewIntake.AuthorReturnForSubject(ctx, credential, expected, key)
		if err != nil {
			reviewIntakeError(w, err)
			return
		}
		if !result.Valid() || result.Owner != expected || result.IntakeKey != key || ctx.Err() != nil {
			reviewIntakeError(w, problemreference.ErrStore)
			return
		}
		writeReviewJSON(w, result)
		return
	}
	var receipt problemreference.ReviewReceipt
	if key == "" {
		input, ok := reviewIntakeBody(w, r)
		if !ok {
			return
		}
		key = input.RequestKey
		receipt, err = h.reviewIntake.SubmitForSubject(ctx, credential, expected, input)
		if err == nil && receipt.ReviewIntent != input {
			err = problemreference.ErrStore
		}
	} else {
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			invalidReviewIntake(w)
			return
		}
		receipt, err = h.reviewIntake.ReceiptForSubject(ctx, credential, expected, key)
	}
	if err != nil {
		reviewIntakeError(w, err)
		return
	}
	if err := ctx.Err(); err != nil {
		if r.Method == http.MethodPost {
			err = errors.Join(problemreference.ErrCommitUnknown, err)
		}
		reviewIntakeError(w, err)
		return
	}
	if !receipt.Valid() || receipt.Owner != expected || receipt.RequestKey != key {
		reviewIntakeError(w, problemreference.ErrStore)
		return
	}
	writeReviewJSON(w, receipt)
}

func reviewIntakeBody(w http.ResponseWriter, r *http.Request) (problemreference.ReviewIntent, bool) {
	var input problemreference.ReviewIntent
	fields := map[string]any{"request_key": &input.RequestKey, "draft_id": &input.DraftID,
		"import_revision": &input.ImportRevision, "reference_request_id": &input.ReferenceRequestID}
	if !readReviewFields(w, r, 1024, fields, invalidReviewIntake) {
		return problemreference.ReviewIntent{}, false
	}
	if !input.Valid() {
		invalidReviewIntake(w)
		return problemreference.ReviewIntent{}, false
	}
	return input, true
}

func readReviewFields(w http.ResponseWriter, r *http.Request, limitBytes int64, fields map[string]any, invalid func(http.ResponseWriter)) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		problem(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 提交")
		return false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limitBytes))
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		problem(w, 413, "BODY_TOO_LARGE", "请求过大")
		return false
	}
	if err != nil || !jsoninput.ValidUnicode(raw) {
		invalid(w)
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	start, _ := d.Token()
	if start != json.Delim('{') {
		invalid(w)
		return false
	}
	for d.More() {
		token, _ := d.Token()
		name, _ := token.(string)
		p := fields[name]
		if p == nil || d.Decode(p) != nil {
			invalid(w)
			return false
		}
		delete(fields, name)
	}
	if len(fields) != 0 {
		invalid(w)
		return false
	}
	return true
}

func writeReviewJSON(w http.ResponseWriter, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		reviewIntakeError(w, problemreference.ErrStore)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func invalidReviewIntake(w http.ResponseWriter) {
	problem(w, 400, "INVALID_REVIEW_SUBMISSION", "送审申请格式无效")
}

func sessionProblemReview(w http.ResponseWriter) {
	problem(w, 401, "SESSION_REQUIRED", "会话无效或已过期")
}

func reviewIntakeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, problemreference.ErrCommitUnknown):
		problem(w, 503, "REVIEW_WRITE_UNKNOWN", "送审结果未确认，请使用原请求键查看回执")
	case sessionDenied(err):
		sessionProblemReview(w)
	case errors.Is(err, identity.ErrForbidden):
		problem(w, 403, "PROBLEM_AUTHOR_REQUIRED", "需要出题权限")
	case errors.Is(err, problemreference.ErrSubjectChanged):
		problem(w, 409, "REVIEW_SUBJECT_CHANGED", "会话用户已变化")
	case errors.Is(err, problemreference.ErrNotFound):
		problem(w, 404, "REVIEW_NOT_FOUND", "草稿、已接纳报告或送审记录不存在")
	case errors.Is(err, problemreference.ErrConflict):
		problem(w, 409, "REVIEW_KEY_CONFLICT", "请求键已用于其他送审材料")
	case errors.Is(err, problemreference.ErrSourceChanged):
		problem(w, 409, "REVIEW_SOURCE_CHANGED", "草稿修订与送审材料不一致")
	case errors.Is(err, problemreference.ErrReviewReturned):
		problem(w, 409, "REVIEW_MATERIALS_RETURNED", "相同材料已退回，请更新草稿修订或验证证据后重送")
	default:
		problem(w, 503, "REVIEW_UNAVAILABLE", "送审暂不可用")
	}
}
