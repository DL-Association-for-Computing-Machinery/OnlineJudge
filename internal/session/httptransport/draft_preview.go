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

// DraftPreview is an optional private-draft capability. It returns only fully
// verified and finally authorized statement/sample data, not rendered HTML.
type DraftPreview interface {
	Preview(context.Context, redisstore.Credential, string) (draftproblem.Preview, error)
}

func draftPreviewID(path string) string {
	if !strings.HasPrefix(path, draftCollection+"/") || !strings.HasSuffix(path, "/preview") {
		return ""
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, draftCollection+"/"), "/preview")
	if !identity.ValidID(id) {
		return ""
	}
	return id
}

func (h *handler) serveDraftPreview(w http.ResponseWriter, ctx context.Context, credential redisstore.Credential, id string) {
	preview, err := h.drafts.(DraftPreview).Preview(ctx, credential, id)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		draftError(w, err)
		return
	}
	body, err := json.Marshal(preview)
	if err != nil || ctx.Err() != nil {
		draftError(w, draftproblem.ErrStore)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
