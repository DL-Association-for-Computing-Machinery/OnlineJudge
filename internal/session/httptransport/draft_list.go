package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	draftproblem "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const ownDraftCollection = "/api/v1/problem-draft-management/drafts"

// OwnDrafts is an additive subject-pinned owner-list capability.
type OwnDrafts interface {
	ListOwnForSubject(context.Context, redisstore.Credential, string, string) (draftproblem.OwnDraftPage, error)
}

func listCursor(r *http.Request) (string, bool) {
	if r.URL.RawPath != "" || r.URL.ForceQuery {
		return "", false
	}
	if r.URL.RawQuery == "" {
		return "", true
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	cursors := values["cursor"]
	if err != nil || len(values) != 1 || len(cursors) != 1 || cursors[0] == "" || len(cursors[0]) > 192 || r.URL.RawQuery != "cursor="+cursors[0] {
		return "", false
	}
	return cursors[0], true
}

func (h *handler) serveOwnDrafts(w http.ResponseWriter, r *http.Request, ctx context.Context, credential redisstore.Credential) {
	cursor, ok := listCursor(r)
	if !ok {
		problem(w, 400, "DRAFT_CURSOR_INVALID", "分页游标无效，请重新检查第一页")
		return
	}
	expected, _ := expectedSubject(r) // Validated before reading the credential.
	page, err := h.drafts.(OwnDrafts).ListOwnForSubject(ctx, credential, cursor, expected)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		draftError(w, err)
		return
	}
	body, err := json.Marshal(page)
	if err != nil || ctx.Err() != nil {
		draftError(w, draftproblem.ErrStore)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
