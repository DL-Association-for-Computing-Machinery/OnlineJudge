package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	draftproblem "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const draftSources = "/api/v1/problem-draft-sources"
const draftImports = "/api/v1/problem-draft-imports"

// DraftImportProtocol is an additive capability for pinned imports and read-only
// recovery. Existing two-field draft imports retain their original contract.
type DraftImportProtocol interface {
	Sources(context.Context, redisstore.Credential) ([]draftproblem.ImportSource, error)
	ImportPinned(context.Context, redisstore.Credential, string, string, string) (draftproblem.ImportReceipt, error)
	Receipt(context.Context, redisstore.Credential, string) (draftproblem.ImportReceipt, error)
}

func draftImportKey(path string) string {
	if !strings.HasPrefix(path, draftImports+"/") {
		return ""
	}
	key := strings.TrimPrefix(path, draftImports+"/")
	if !draftproblem.ValidKey(key) {
		return ""
	}
	return key
}

func draftImportPath(path string) bool {
	return path == draftSources || path == draftImports || draftImportKey(path) != ""
}

func (h *handler) serveDraftImport(w http.ResponseWriter, r *http.Request, ctx context.Context, credential redisstore.Credential) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		problem(w, http.StatusBadRequest, "INVALID_DRAFT_REQUEST", "草稿请求格式无效")
		return
	}
	imports := h.drafts.(DraftImportProtocol)
	var result any
	var err error
	switch {
	case r.URL.Path == draftSources:
		var sources []draftproblem.ImportSource
		sources, err = imports.Sources(ctx, credential)
		result = struct {
			Sources []draftproblem.ImportSource `json:"sources"`
		}{sources}
	case r.Method == http.MethodPost:
		key, source, revision, ok := pinnedDraftBody(w, r)
		if !ok {
			return
		}
		result, err = imports.ImportPinned(ctx, credential, key, source, revision)
	default:
		result, err = imports.Receipt(ctx, credential, draftImportKey(r.URL.Path))
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		draftError(w, err)
		return
	}
	body, err := json.Marshal(result)
	if err != nil || ctx.Err() != nil {
		draftError(w, draftproblem.ErrStore)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
