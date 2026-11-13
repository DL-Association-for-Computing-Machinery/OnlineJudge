package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	draftproblem "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const draftCollection = "/api/v1/problem-drafts"

// Drafts accepts opaque ordinary credentials; the application owns fresh
// session, author, candidate and owner checks. No actor comes from HTTP input.
type Drafts interface {
	Import(context.Context, redisstore.Credential, string, string) (draftproblem.Draft, error)
	GetOwn(context.Context, redisstore.Credential, string) (draftproblem.Draft, error)
}

// WithPrivateDrafts creates an independent adapter retaining the exact existing
// session/proxy capabilities. The input is not mutated and can already be used.
func WithPrivateDrafts(base http.Handler, drafts Drafts, timeout time.Duration) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || drafts == nil || timeout <= 0 || timeout > time.Minute || h.drafts != nil {
		return nil, redisstore.ErrInvalid
	}
	return &handler{store: h.store, config: h.config, host: h.host, login: h.login, proxy: h.proxy,
		passwordChange: h.passwordChange, emailConfirmation: h.emailConfirmation, drafts: drafts, draftTimeout: timeout}, nil
}

// IsPrivateDraftPath recognizes route shapes only, without granting capability
// or authority. The outer HTTP router and this adapter share the same predicate.
func IsPrivateDraftPath(path string) bool {
	return path == ownDraftCollection || draftImportPath(path) || managementPath(path) || path == draftCollection || draftPreviewID(path) != "" || (strings.HasPrefix(path, draftCollection+"/") && identity.ValidID(strings.TrimPrefix(path, draftCollection+"/")))
}

func (h *handler) serveDraft(w http.ResponseWriter, r *http.Request) {
	if managementPath(r.URL.Path) || r.URL.Path == ownDraftCollection {
		if _, ok := expectedSubject(r); !ok {
			problem(w, http.StatusBadRequest, "INVALID_DRAFT_REQUEST", "草稿请求格式无效")
			return
		}
	}
	credential, err := requestCredential(r)
	if err != nil {
		problem(w, 401, "SESSION_REQUIRED", "会话无效或已过期")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.draftTimeout)
	defer cancel()
	if r.URL.Path == ownDraftCollection {
		h.serveOwnDrafts(w, r, ctx, credential)
		return
	}
	if draftImportPath(r.URL.Path) || managementPath(r.URL.Path) {
		h.serveDraftImport(w, r, ctx, credential)
		return
	}
	if id := draftPreviewID(r.URL.Path); id != "" {
		h.serveDraftPreview(w, ctx, credential, id)
		return
	}
	var draft draftproblem.Draft
	if r.Method == http.MethodPost {
		key, source, ok := draftBody(w, r)
		if !ok {
			return
		}
		draft, err = h.drafts.Import(ctx, credential, key, source)
	} else {
		draft, err = h.drafts.GetOwn(ctx, credential, strings.TrimPrefix(r.URL.Path, draftCollection+"/"))
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		draftError(w, err)
		return
	}
	// Encode before committing headers. []byte would otherwise become base64.
	if !json.Valid(draft.Document) {
		draftError(w, draftproblem.ErrStore)
		return
	}
	body, err := json.Marshal(struct {
		ID              string                   `json:"id"`
		RequestKey      string                   `json:"request_key"`
		SourceID        string                   `json:"source_id"`
		Version         string                   `json:"version"`
		State           string                   `json:"state"`
		ValidationState string                   `json:"validation_state"`
		Document        json.RawMessage          `json:"document"`
		References      []draftproblem.Reference `json:"references"`
		CreatedAt       time.Time                `json:"created_at"`
	}{draft.ID, draft.RequestKey, draft.SourceID, draft.Version, draft.State, draft.ValidationState, draft.Document, draft.References, draft.CreatedAt})
	if err != nil || ctx.Err() != nil {
		draftError(w, draftproblem.ErrStore)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func draftError(w http.ResponseWriter, err error) {
	// Unknown effects and infrastructure errors win over joined refusals. POST
	// callers retain the original key AND the exact configured candidate version.
	failed := errors.Is(err, draftproblem.ErrStore) || errors.Is(err, draftproblem.ErrCommitUnknown) || errors.Is(err, draftproblem.ErrInvalid) || errors.Is(err, draftproblem.ErrBusy) ||
		errors.Is(err, materialstore.ErrUnavailable) || errors.Is(err, materialstore.ErrCommitUnknown) ||
		errors.Is(err, materialstore.ErrIntegrity) || errors.Is(err, materialstore.ErrBusy) || errors.Is(err, materialstore.ErrClosed) || errors.Is(err, materialstore.ErrNotFound) || errors.Is(err, materialstore.ErrInvalid) ||
		errors.Is(err, identity.ErrStore) || errors.Is(err, identity.ErrRecovering) || (errors.Is(err, identity.ErrGuardedOperation) && !sessionDenied(err)) ||
		errors.Is(err, redisstore.ErrStore) || errors.Is(err, redisstore.ErrUnknown) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	if !failed {
		switch {
		case sessionDenied(err):
			problem(w, 401, "SESSION_REQUIRED", "会话无效或已过期")
			return
		case errors.Is(err, identity.ErrForbidden):
			problem(w, 403, "PROBLEM_AUTHOR_REQUIRED", "需要出题权限")
			return
		case errors.Is(err, draftproblem.ErrNotFound):
			problem(w, 404, "DRAFT_NOT_FOUND", "草稿不存在")
			return
		case errors.Is(err, draftproblem.ErrUnknownSource):
			problem(w, 400, "DRAFT_SOURCE_UNKNOWN", "候选题目未配置")
			return
		case errors.Is(err, draftproblem.ErrConflict):
			problem(w, 409, "DRAFT_REQUEST_CONFLICT", "请求键已用于不同内容")
			return
		case errors.Is(err, draftproblem.ErrSourceChanged):
			problem(w, 409, "DRAFT_SOURCE_CHANGED", "候选修订已变化；请保留原请求并查询回执")
			return
		case errors.Is(err, draftproblem.ErrSubjectChanged):
			problem(w, 409, "DRAFT_SUBJECT_CHANGED", "会话用户已变化")
			return
		case errors.Is(err, draftproblem.ErrInvalidCursor):
			problem(w, 400, "DRAFT_CURSOR_INVALID", "分页游标无效，请重新检查第一页")
			return
		}
	}
	problem(w, 503, "DRAFT_UNAVAILABLE", "草稿服务暂时不可用；导入请保留原请求键和候选版本核验")
}
