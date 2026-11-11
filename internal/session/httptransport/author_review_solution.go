package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// AuthorReviewSolution is the scoped author preview and confirmation capability.
type AuthorReviewSolution interface {
	SolutionForSubject(context.Context, redisstore.Credential, string, string) (problemreference.AuthorSolution, error)
	ConfirmSolutionForSubject(context.Context, redisstore.Credential, string, string, problemreference.SolutionBinding) (problemreference.SolutionConfirmation, error)
	ConfirmationForSubject(context.Context, redisstore.Credential, string, string) (problemreference.SolutionConfirmation, error)
}

func (h *handler) serveAuthorReviewSolution(w http.ResponseWriter, r *http.Request, key, action string) {
	expected, ok := expectedSubject(r)
	if !ok || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" ||
		(r.Method == http.MethodGet && (r.ContentLength != 0 || len(r.TransferEncoding) != 0)) {
		invalidAuthorSolution(w)
		return
	}
	credential, err := requestCredential(r)
	if err != nil {
		sessionProblemReview(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.draftTimeout)
	defer cancel()
	if action == "solution" {
		view, err := h.reviewSolution.SolutionForSubject(ctx, credential, expected, key)
		if err != nil {
			authorSolutionError(w, err)
			return
		}
		body, err := json.Marshal(view)
		if err != nil || !view.Valid() || view.StandardSolution.Source == "" ||
			len(body) > problemreference.MaxAuthorSolutionResponse || ctx.Err() != nil {
			authorSolutionError(w, problemreference.ErrStore)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
		return
	}
	var receipt problemreference.SolutionConfirmation
	if r.Method == http.MethodPost {
		var binding problemreference.SolutionBinding
		if !readReviewFields(w, r, 1024, map[string]any{"import_revision": &binding.ImportRevision,
			"formal_plan_sha256": &binding.FormalPlanSHA256}, invalidAuthorSolution) {
			return
		}
		if !binding.Valid() {
			invalidAuthorSolution(w)
			return
		}
		receipt, err = h.reviewSolution.ConfirmSolutionForSubject(ctx, credential, expected, key, binding)
		if err == nil && receipt.SolutionBinding != binding {
			err = problemreference.ErrStore
		}
	} else {
		receipt, err = h.reviewSolution.ConfirmationForSubject(ctx, credential, expected, key)
	}
	if err != nil {
		authorSolutionError(w, err)
		return
	}
	if err := ctx.Err(); err != nil {
		if r.Method == http.MethodPost {
			err = errors.Join(problemreference.ErrCommitUnknown, err)
		}
		authorSolutionError(w, err)
		return
	}
	if !receipt.Valid() || receipt.Owner != expected || receipt.IntakeKey != key {
		authorSolutionError(w, problemreference.ErrStore)
		return
	}
	writeReviewJSON(w, receipt)
}

func invalidAuthorSolution(w http.ResponseWriter) {
	problem(w, 400, "INVALID_SOLUTION_CONFIRMATION", "标程确认请求格式无效")
}

func authorSolutionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, problemreference.ErrCommitUnknown):
		problem(w, 503, "SOLUTION_CONFIRMATION_UNKNOWN", "确认结果未知，请使用原送审键查看回执")
	case errors.Is(err, problemreference.ErrReviewChanged):
		problem(w, 409, "SOLUTION_SELECTION_CHANGED", "标程或草稿修订已变化，请重新核对")
	default:
		reviewIntakeError(w, err)
	}
}
