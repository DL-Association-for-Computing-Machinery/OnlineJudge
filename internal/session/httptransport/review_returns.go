package httptransport

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	draftproblem "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ReviewReturns is the optional reviewer return write and original-key recovery.
type ReviewReturns interface {
	ReturnForSubject(context.Context, redisstore.Credential, string, string, problemreference.ReturnIntent) (problemreference.ReviewReturnReceipt, error)
	ReturnReceiptForSubject(context.Context, redisstore.Credential, string, string, string) (problemreference.ReviewReturnReceipt, error)
}

// IsReviewReturnPath recognizes the two scoped reviewer return endpoints.
func IsReviewReturnPath(path string) bool {
	_, _, method := reviewReturnRoute(path)
	return method != ""
}

func reviewReturnRoute(path string) (id, key, method string) {
	return reviewActionRoute(path, "return")
}

func reviewActionRoute(path, action string) (id, key, method string) {
	tail, ok := strings.CutPrefix(path, reviewEvidencePrefix)
	if !ok {
		return "", "", ""
	}
	id, rest, ok := strings.Cut(tail, "/")
	if !ok || !identity.ValidID(id) {
		return "", "", ""
	}
	if rest == action {
		return id, "", http.MethodPost
	}
	if key, ok := strings.CutPrefix(rest, action+"/"); ok && draftproblem.ValidKey(key) {
		return id, key, http.MethodGet
	}
	return "", "", ""
}

func (h *handler) serveReviewReturn(w http.ResponseWriter, r *http.Request, id, key string) {
	expected, ok := expectedSubject(r)
	if !ok || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" ||
		(key != "" && (r.ContentLength != 0 || len(r.TransferEncoding) != 0)) {
		invalidReviewReturn(w)
		return
	}
	credential, err := requestCredential(r)
	if err != nil {
		sessionProblemReview(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.draftTimeout)
	defer cancel()
	var receipt problemreference.ReviewReturnReceipt
	if key == "" {
		var i problemreference.ReturnIntent
		fields := map[string]any{"request_key": &i.RequestKey, "assignment_revision": &i.AssignmentRevision,
			"import_revision": &i.ImportRevision, "evidence_sha256": &i.EvidenceSHA256, "reason": &i.Reason}
		if !readReviewFields(w, r, 4096, fields, invalidReviewReturn) {
			return
		}
		if !i.Valid() {
			invalidReviewReturn(w)
			return
		}
		key = i.RequestKey
		receipt, err = h.reviewReturns.ReturnForSubject(ctx, credential, expected, id, i)
		if err == nil && receipt.ReturnIntent != i {
			err = problemreference.ErrStore
		}
	} else {
		receipt, err = h.reviewReturns.ReturnReceiptForSubject(ctx, credential, expected, id, key)
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
		if r.Method == http.MethodPost {
			err = errors.Join(problemreference.ErrCommitUnknown, err)
		}
	}
	if err != nil {
		reviewReturnError(w, err)
		return
	}
	if !receipt.Valid() || receipt.Reviewer != expected || receipt.ReviewID != id || receipt.RequestKey != key {
		reviewReturnError(w, problemreference.ErrStore)
		return
	}
	writeReviewJSON(w, receipt)
}

func invalidReviewReturn(w http.ResponseWriter) {
	problem(w, 400, "INVALID_REVIEW_RETURN", "审核退回请求格式无效")
}

func reviewReturnError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, problemreference.ErrCommitUnknown):
		problem(w, 503, "REVIEW_RETURN_UNKNOWN", "退回结果未确认，请使用原请求键查看回执")
	case errors.Is(err, problemreference.ErrStore):
		problem(w, 503, "REVIEW_UNAVAILABLE", "审核暂不可用")
	case sessionDenied(err):
		sessionProblemReview(w)
	case errors.Is(err, identity.ErrReviewerRequired):
		problem(w, 403, "PROBLEM_REVIEWER_REQUIRED", "需要审核权限")
	case errors.Is(err, problemreference.ErrNotFound):
		problem(w, 404, "REVIEW_RETURN_NOT_FOUND", "审核项未分配或退回回执不存在")
	case errors.Is(err, problemreference.ErrSubjectChanged):
		problem(w, 409, "REVIEW_SUBJECT_CHANGED", "会话用户已变化")
	case errors.Is(err, problemreference.ErrReviewChanged), errors.Is(err, problemreference.ErrReviewAlreadyApproved):
		problem(w, 409, "REVIEW_CHANGED", "分配或证据已变化，请重新读取审核项")
	case errors.Is(err, problemreference.ErrReviewReturned):
		problem(w, 409, "REVIEW_MATERIALS_RETURNED", "相同材料已有退回决定")
	case errors.Is(err, problemreference.ErrConflict):
		problem(w, 409, "REVIEW_RETURN_KEY_CONFLICT", "请求键已用于其他退回决定")
	default:
		problem(w, 503, "REVIEW_UNAVAILABLE", "审核暂不可用")
	}
}
