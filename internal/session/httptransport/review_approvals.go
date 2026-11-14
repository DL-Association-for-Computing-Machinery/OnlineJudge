package httptransport

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const approvalAction = "approve"

// ReviewApprovals is the scoped approval and original-key recovery capability.
type ReviewApprovals interface {
	ApprovalForSubject(context.Context, redisstore.Credential, string, string, problemreference.ApprovalIntent) (problemreference.ReviewApprovalReceipt, error)
	ApprovalReceiptForSubject(context.Context, redisstore.Credential, string, string, string) (problemreference.ReviewApprovalReceipt, error)
}

// IsReviewApprovalPath recognizes both reviewer approval paths.
func IsReviewApprovalPath(path string) bool {
	_, _, method := reviewActionRoute(path, approvalAction)
	return method != ""
}

// WithReviewApprovals adds approval to an existing reviewer runtime.
func WithReviewApprovals(base http.Handler, approvals ReviewApprovals) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || h.reviewEvidence == nil || h.reviewApprovals != nil || approvals == nil {
		return nil, redisstore.ErrInvalid
	}
	next := *h
	next.reviewApprovals = approvals
	return &next, nil
}

func (h *handler) serveReviewApproval(w http.ResponseWriter, r *http.Request, id, key string) {
	subject, ok := expectedSubject(r)
	if !ok || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" ||
		(key != "" && (r.ContentLength != 0 || len(r.TransferEncoding) != 0)) {
		invalidReviewApproval(w)
		return
	}
	credential, err := requestCredential(r)
	if err != nil {
		sessionProblemReview(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.draftTimeout)
	defer cancel()
	var receipt problemreference.ReviewApprovalReceipt
	if key == "" {
		var i problemreference.ApprovalIntent
		fields := map[string]any{"request_key": &i.RequestKey, "assignment_revision": &i.AssignmentRevision,
			"snapshot_sha256": &i.SnapshotSHA256, "reason": &i.Reason}
		if !readReviewFields(w, r, 4096, fields, invalidReviewApproval) {
			return
		}
		if !i.Valid() {
			invalidReviewApproval(w)
			return
		}
		key = i.RequestKey
		receipt, err = h.reviewApprovals.ApprovalForSubject(ctx, credential, subject, id, i)
		if err == nil && receipt.ApprovalIntent != i {
			err = problemreference.ErrCommitUnknown
		}
	} else {
		receipt, err = h.reviewApprovals.ApprovalReceiptForSubject(ctx, credential, subject, id, key)
	}
	if err == nil && (ctx.Err() != nil || !receipt.Valid() || receipt.Binding.Reviewer != subject ||
		receipt.Binding.ReviewID != id || receipt.RequestKey != key) {
		err = problemreference.ErrStore
		if r.Method == http.MethodPost {
			err = problemreference.ErrCommitUnknown
		}
	}
	if err != nil {
		reviewApprovalError(w, err)
		return
	}
	writeReviewJSON(w, struct {
		ReviewID           string    `json:"review_id"`
		RequestKey         string    `json:"request_key"`
		AssignmentRevision int64     `json:"assignment_revision"`
		SnapshotSHA256     string    `json:"snapshot_sha256"`
		ApprovedAt         time.Time `json:"approved_at"`
	}{id, key, receipt.AssignmentRevision, receipt.SnapshotSHA256, receipt.ApprovedAt})
}

func invalidReviewApproval(w http.ResponseWriter) {
	problem(w, 400, "INVALID_REVIEW_APPROVAL", "审核批准请求格式无效")
}

func reviewApprovalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, problemreference.ErrCommitUnknown):
		problem(w, 503, "REVIEW_APPROVAL_UNKNOWN", "批准结果未确认，请使用原请求键查看回执")
	case errors.Is(err, problemreference.ErrStore):
		problem(w, 503, "REVIEW_UNAVAILABLE", "审核暂不可用")
	case sessionDenied(err):
		sessionProblemReview(w)
	case errors.Is(err, identity.ErrReviewerRequired):
		problem(w, 403, "PROBLEM_REVIEWER_REQUIRED", "需要审核权限")
	case errors.Is(err, problemreference.ErrNotFound):
		problem(w, 404, "REVIEW_APPROVAL_NOT_FOUND", "审核项未分配或批准回执不存在")
	case errors.Is(err, problemreference.ErrSubjectChanged):
		problem(w, 409, "REVIEW_SUBJECT_CHANGED", "会话用户已变化")
	case errors.Is(err, problemreference.ErrReviewApprovalBlocked):
		problem(w, 409, "REVIEW_APPROVAL_BLOCKED", "审核项尚未满足全部批准条件")
	case errors.Is(err, problemreference.ErrReviewChanged), errors.Is(err, problemreference.ErrReviewAlreadyApproved):
		problem(w, 409, "REVIEW_CHANGED", "请重新读取审核项")
	case errors.Is(err, problemreference.ErrConflict):
		problem(w, 409, "REVIEW_APPROVAL_KEY_CONFLICT", "请求键与原批准不匹配")
	default:
		problem(w, 503, "REVIEW_UNAVAILABLE", "审核暂不可用")
	}
}
