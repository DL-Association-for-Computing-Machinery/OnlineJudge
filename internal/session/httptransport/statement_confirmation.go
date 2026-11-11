package httptransport

import (
	"context"
	"errors"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// StatementConfirmations exposes the explicit human declaration and its original-key readback.
type StatementConfirmations interface {
	ConfirmForSubject(context.Context, redisstore.Credential, string, string, problemreference.StatementConfirmationIntent) (problemreference.StatementConfirmationReceipt, error)
	ReceiptForSubject(context.Context, redisstore.Credential, string, string, string) (problemreference.StatementConfirmationReceipt, error)
}

const statementConfirmationAction = "statement-confirmation"

// IsStatementConfirmationPath recognizes the two reviewer confirmation routes.
func IsStatementConfirmationPath(path string) bool {
	_, _, method := reviewActionRoute(path, statementConfirmationAction)
	return method != ""
}

func (h *handler) serveStatementConfirmation(w http.ResponseWriter, r *http.Request, id, key string) {
	expected, ok := expectedSubject(r)
	if !ok || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" ||
		(key != "" && (r.ContentLength != 0 || len(r.TransferEncoding) != 0)) {
		invalidStatementConfirmation(w)
		return
	}
	credential, err := requestCredential(r)
	if err != nil {
		sessionProblemReview(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.draftTimeout)
	defer cancel()
	var receipt problemreference.StatementConfirmationReceipt
	if key == "" {
		var i problemreference.StatementConfirmationIntent
		fields := map[string]any{"request_key": &i.RequestKey, "assignment_revision": &i.AssignmentRevision,
			"import_revision": &i.ImportRevision, "evidence_sha256": &i.EvidenceSHA256, "checklist_version": &i.ChecklistVersion}
		if !readReviewFields(w, r, 4096, fields, invalidStatementConfirmation) {
			return
		}
		if !i.Valid() {
			invalidStatementConfirmation(w)
			return
		}
		key = i.RequestKey
		receipt, err = h.statementConfirmations.ConfirmForSubject(ctx, credential, expected, id, i)
		if err == nil && receipt.StatementConfirmationIntent != i {
			err = problemreference.ErrStore
		}
	} else {
		receipt, err = h.statementConfirmations.ReceiptForSubject(ctx, credential, expected, id, key)
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
		if r.Method == http.MethodPost {
			err = errors.Join(problemreference.ErrCommitUnknown, err)
		}
	}
	switch {
	case errors.Is(err, problemreference.ErrCommitUnknown):
		problem(w, 503, "STATEMENT_CONFIRMATION_UNKNOWN", "题面确认结果未知，请用原请求键查询")
	case errors.Is(err, problemreference.ErrNotFound):
		problem(w, 404, "STATEMENT_CONFIRMATION_NOT_FOUND", "审核项未分配或确认回执不存在")
	case errors.Is(err, problemreference.ErrConflict):
		problem(w, 409, "STATEMENT_CONFIRMATION_CONFLICT", "请求键或当前分配已有其他确认")
	case err != nil:
		reviewReturnError(w, err)
	case !receipt.Valid() || receipt.Reviewer != expected || receipt.ReviewID != id || receipt.RequestKey != key:
		reviewReturnError(w, problemreference.ErrStore)
	default:
		writeReviewJSON(w, receipt)
	}
}

func invalidStatementConfirmation(w http.ResponseWriter) {
	problem(w, 400, "INVALID_STATEMENT_CONFIRMATION", "题面确认请求格式无效")
}
