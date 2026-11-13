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
const maxReviewSummaryResponse = 4096

// ReviewEvidenceReader exposes only the scoped validation results to HTTP.
type ReviewEvidenceReader interface {
	EvidenceForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewEvidence, error)
	ComparisonForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewComparison, error)
	EligibilityForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewEligibility, error)
}

// ReviewMaterialsReader exposes the bounded, reauthorized material projection.
type ReviewMaterialsReader interface {
	MaterialsForSubject(context.Context, redisstore.Credential, string, string) (problemreference.ReviewMaterials, error)
}

// WithReviewEvidence adds reviewer access to the configured draft runtime.
// A nil materials reader leaves the validation result routes available.
func WithReviewEvidence(base http.Handler, reader ReviewEvidenceReader, materials ReviewMaterialsReader, returns ReviewReturns, statements StatementConfirmations) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || h.drafts == nil || h.reviewEvidence != nil || reader == nil {
		return nil, redisstore.ErrInvalid
	}
	return &handler{store: h.store, config: h.config, host: h.host, login: h.login, proxy: h.proxy,
		passwordChange: h.passwordChange, emailConfirmation: h.emailConfirmation, emailRequest: h.emailRequest, drafts: h.drafts, draftTimeout: h.draftTimeout,
		references: h.references, preparation: h.preparation, reviewIntake: h.reviewIntake, reviewSolution: h.reviewSolution,
		reviewEvidence: reader, reviewMaterials: materials, reviewReturns: returns, statementConfirmations: statements}, nil
}

// IsReviewReadPath recognizes scoped reviewer GET routes.
func IsReviewReadPath(path string) bool {
	id, _ := reviewReadRoute(path)
	return id != ""
}

func reviewReadRoute(path string) (id string, leaf string) {
	tail, ok := strings.CutPrefix(path, reviewEvidencePrefix)
	if !ok {
		return "", ""
	}
	id, leaf, found := strings.Cut(tail, "/")
	if !found || !identity.ValidID(id) {
		return "", ""
	}
	switch leaf {
	case "evidence", "comparison", "materials", "eligibility":
		return id, leaf
	default:
		return "", ""
	}
}

func (h *handler) serveReviewRead(w http.ResponseWriter, r *http.Request, id, leaf string) {
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
	var result any
	var e problemreference.ReviewEvidence
	var valid bool
	limit := maxReviewSummaryResponse
	switch leaf {
	case "materials":
		limit = problemreference.MaxReviewMaterialResponse
		var m problemreference.ReviewMaterials
		m, err = h.reviewMaterials.MaterialsForSubject(ctx, credential, expected, id)
		result, e, valid = m, m.Evidence, m.Valid()
	case "comparison":
		var c problemreference.ReviewComparison
		c, err = h.reviewEvidence.ComparisonForSubject(ctx, credential, expected, id)
		result, e, valid = c, c.Evidence, c.Valid()
	case "eligibility":
		var status problemreference.ReviewEligibility
		status, err = h.reviewEvidence.EligibilityForSubject(ctx, credential, expected, id)
		result, e = status, status.Evidence
		valid = e.Valid() && status.PolicyVersion == problemreference.ReviewPolicyVersion &&
			!status.Eligible && len(status.Blockers) > 0
	case "evidence":
		e, err = h.reviewEvidence.EvidenceForSubject(ctx, credential, expected, id)
		result, valid = e, e.Valid()
	}
	switch {
	case ctx.Err() != nil, errors.Is(err, problemreference.ErrStore):
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
	if err != nil || !valid || e.ReviewID != id || e.Reviewer != expected {
		problem(w, 503, "REVIEW_UNAVAILABLE", "审核证据暂不可用")
		return
	}
	body, err := json.Marshal(result)
	if err != nil || len(body) > limit || ctx.Err() != nil {
		problem(w, 503, "REVIEW_UNAVAILABLE", "审核证据暂不可用")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
