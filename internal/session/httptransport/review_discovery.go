package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const reviewDiscoveryPath = "/api/v1/problem-review/assignments"
const maxReviewDiscoveryResponse = 20480

// ReviewDiscoveryReader lists only assignments of the expected current subject.
type ReviewDiscoveryReader interface {
	ListForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewDiscoveryPage, error)
}

// WithReviewDiscovery enables the collection alongside single-item reviewer reads.
func WithReviewDiscovery(base http.Handler, reader ReviewDiscoveryReader) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || h.reviewEvidence == nil || reader == nil || h.reviewDiscovery != nil {
		return nil, redisstore.ErrInvalid
	}
	next := *h
	next.reviewDiscovery = reader
	return &next, nil
}

func (h *handler) serveReviewDiscovery(w http.ResponseWriter, r *http.Request) {
	expected, pinned := expectedSubject(r)
	cursor, valid := listCursor(r)
	if !pinned || !valid || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "INVALID_REVIEW_ASSIGNMENTS_REQUEST", "审核分配请求格式无效")
		return
	}
	credential, err := requestCredential(r)
	if err != nil {
		sessionProblemReview(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.draftTimeout)
	defer cancel()
	page, err := h.reviewDiscovery.ListForSubject(ctx, credential, expected, cursor)
	switch {
	case ctx.Err() != nil:
		problem(w, 503, "REVIEW_UNAVAILABLE", "暂时无法读取审核分配")
	case errors.Is(err, problemreference.ErrReviewCursor):
		problem(w, 400, "REVIEW_CURSOR_INVALID", "分页已失效，请重新检查第一页")
	case errors.Is(err, problemreference.ErrSubjectChanged):
		problem(w, 409, "REVIEW_SUBJECT_CHANGED", "会话账号已变化，请重新检查")
	case errors.Is(err, identity.ErrReviewerRequired):
		problem(w, 403, "REVIEW_FORBIDDEN", "当前会话无法读取审核分配")
	case errors.Is(err, redisstore.ErrNotFound), errors.Is(err, identity.ErrConflict):
		sessionProblemReview(w)
	case err != nil:
		problem(w, 503, "REVIEW_UNAVAILABLE", "暂时无法读取审核分配")
	default:
		body, err := json.Marshal(page)
		if err != nil || len(body) > maxReviewDiscoveryResponse || ctx.Err() != nil {
			problem(w, 503, "REVIEW_UNAVAILABLE", "暂时无法读取审核分配")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}
