package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	draftproblem "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const draftSources = "/api/v1/problem-draft-sources"
const draftImports = "/api/v1/problem-draft-imports"
const managementSources = "/api/v1/problem-draft-management/sources"
const managementImports = "/api/v1/problem-draft-management/imports"

// DraftManagement is the subject-pinned management capability.
type DraftManagement interface {
	SourcesForSubject(context.Context, redisstore.Credential, string) ([]draftproblem.ImportSource, error)
	ImportPinnedForSubject(context.Context, redisstore.Credential, string, string, string, string) (draftproblem.ImportReceipt, error)
	ReceiptForSubject(context.Context, redisstore.Credential, string, string) (draftproblem.ImportReceipt, error)
}

// DraftImportProtocol is an additive capability for pinned imports and read-only
// recovery. Existing two-field draft imports retain their original contract.
type DraftImportProtocol interface {
	Sources(context.Context, redisstore.Credential) ([]draftproblem.ImportSource, error)
	ImportPinned(context.Context, redisstore.Credential, string, string, string) (draftproblem.ImportReceipt, error)
	Receipt(context.Context, redisstore.Credential, string) (draftproblem.ImportReceipt, error)
}

func draftImportKey(path string) string {
	return importKey(path, draftImports)
}

func importKey(path, prefix string) string {
	if !strings.HasPrefix(path, prefix+"/") {
		return ""
	}
	key := strings.TrimPrefix(path, prefix+"/")
	if !draftproblem.ValidKey(key) {
		return ""
	}
	return key
}

func managementPath(path string) bool {
	return path == managementSources || path == managementImports || importKey(path, managementImports) != ""
}

func expectedSubject(r *http.Request) (string, bool) {
	values := r.Header.Values("X-OJ-Expected-Subject")
	if len(values) != 1 || !identity.ValidID(values[0]) {
		return "", false
	}
	return values[0], true
}

func draftImportPath(path string) bool {
	return path == draftSources || path == draftImports || draftImportKey(path) != ""
}

func (h *handler) serveDraftImport(w http.ResponseWriter, r *http.Request, ctx context.Context, credential redisstore.Credential) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		problem(w, http.StatusBadRequest, "INVALID_DRAFT_REQUEST", "草稿请求格式无效")
		return
	}
	management := managementPath(r.URL.Path)
	expected := ""
	if management {
		var ok bool
		expected, ok = expectedSubject(r)
		if !ok {
			problem(w, http.StatusBadRequest, "INVALID_DRAFT_REQUEST", "草稿请求格式无效")
			return
		}
	}
	var result any
	var err error
	switch {
	case r.URL.Path == draftSources || r.URL.Path == managementSources:
		var sources []draftproblem.ImportSource
		if management {
			sources, err = h.drafts.(DraftManagement).SourcesForSubject(ctx, credential, expected)
		} else {
			sources, err = h.drafts.(DraftImportProtocol).Sources(ctx, credential)
		}
		result = struct {
			Sources []draftproblem.ImportSource `json:"sources"`
		}{sources}
	case r.Method == http.MethodPost:
		key, source, revision, ok := pinnedDraftBody(w, r)
		if !ok {
			return
		}
		if management {
			result, err = h.drafts.(DraftManagement).ImportPinnedForSubject(ctx, credential, key, source, revision, expected)
		} else {
			result, err = h.drafts.(DraftImportProtocol).ImportPinned(ctx, credential, key, source, revision)
		}
	default:
		if management {
			result, err = h.drafts.(DraftManagement).ReceiptForSubject(ctx, credential, importKey(r.URL.Path, managementImports), expected)
		} else {
			result, err = h.drafts.(DraftImportProtocol).Receipt(ctx, credential, draftImportKey(r.URL.Path))
		}
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
