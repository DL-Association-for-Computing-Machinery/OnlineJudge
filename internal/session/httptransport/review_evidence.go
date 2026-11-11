package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const reviewEvidencePrefix = "/api/v1/problem-review/"

// ReviewEvidenceReader exposes only the scoped read operation to HTTP.
type ReviewEvidenceReader interface {
	EvidenceForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewEvidence, error)
}

// WithReviewEvidence adds reviewer access to the configured draft runtime.
func WithReviewEvidence(base http.Handler, reader ReviewEvidenceReader) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || h.drafts == nil || h.reviewEvidence != nil || reader == nil {
		return nil, redisstore.ErrInvalid
	}
	return &handler{store: h.store, config: h.config, host: h.host, login: h.login, proxy: h.proxy,
		passwordChange: h.passwordChange, drafts: h.drafts, draftTimeout: h.draftTimeout,
		references: h.references, preparation: h.preparation, reviewIntake: h.reviewIntake, reviewEvidence: reader}, nil
}

// IsReviewEvidencePath recognizes the single scoped GET path.
func IsReviewEvidencePath(path string) bool { return reviewEvidenceID(path) != "" }

func reviewEvidenceID(path string) string {
	tail, ok := strings.CutPrefix(path, reviewEvidencePrefix)
	if id, suffix := strings.CutSuffix(tail, "/evidence"); ok && suffix && identity.ValidID(id) {
		return id
	}
	return ""
}

func (h *handler) serveReviewEvidence(w http.ResponseWriter, r *http.Request, id string) {
	expected, ok := expectedSubject(r)
	if !ok || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "INVALID_REVIEW_EVIDENCE_REQUEST", "审核证据请求格式无效")
		return
	}
	credential, err := requestCredential(r)
	if err != nil {
		sessionProblemReview(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.draftTimeout)
	defer cancel()
	e, err := h.reviewEvidence.EvidenceForSubject(ctx, credential, expected, id)
	switch {
	case ctx.Err() != nil:
		problem(w, 503, "REVIEW_UNAVAILABLE", "审核证据暂不可用")
		return
	case sessionDenied(err):
		sessionProblemReview(w)
		return
	case errors.Is(err, identity.ErrReviewerRequired):
		problem(w, 403, "PROBLEM_REVIEWER_REQUIRED", "需要审核权限")
		return
	case errors.Is(err, problemreference.ErrNotFound):
		problem(w, 404, "REVIEW_EVIDENCE_NOT_FOUND", "审核项不存在或未分配")
		return
	case errors.Is(err, problemreference.ErrSubjectChanged):
		problem(w, 409, "REVIEW_SUBJECT_CHANGED", "会话用户已变化")
		return
	}
	if err != nil || !e.Valid() || e.ReviewID != id || e.Reviewer != expected {
		problem(w, 503, "REVIEW_UNAVAILABLE", "审核证据暂不可用")
		return
	}
	body, err := json.Marshal(e)
	if err != nil || ctx.Err() != nil {
		problem(w, 503, "REVIEW_UNAVAILABLE", "审核证据暂不可用")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
