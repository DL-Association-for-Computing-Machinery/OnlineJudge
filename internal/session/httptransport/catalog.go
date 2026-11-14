package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	problemdomain "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

const catalogPath = "/api/v1/problems"

// CatalogEntry is the allowlisted public list item.
type CatalogEntry struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Title   string `json:"title"`
}

// CatalogReader reads public projections without consulting caller identity.
type CatalogReader struct {
	List   func(context.Context, string, string) ([]CatalogEntry, string, error)
	Detail func(context.Context, string) (problemdomain.PublicationStatement, error)
}

// IsCatalogPath identifies the public read route family.
func IsCatalogPath(path string) bool {
	return path == catalogPath || strings.HasPrefix(path, catalogPath+"/")
}

// WithCatalog enables only the public catalog read capability.
func WithCatalog(base http.Handler, catalog CatalogReader) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || h.catalog != nil || catalog.List == nil || catalog.Detail == nil {
		return nil, problemreference.ErrInvalid
	}
	next := *h
	next.catalog = &catalog
	return &next, nil
}

func (h *handler) serveCatalog(w http.ResponseWriter, r *http.Request) {
	if h.catalog == nil {
		problemResponseCatalog(w, http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		problemResponseCatalog(w, http.StatusMethodNotAllowed)
		return
	}
	if r.URL.RawPath != "" || len(r.URL.RawQuery) > 1024 || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problemResponseCatalog(w, http.StatusBadRequest)
		return
	}
	var result any
	var err error
	if r.URL.Path == catalogPath {
		values, queryErr := url.ParseQuery(r.URL.RawQuery)
		for key, parts := range values {
			if (key != "q" && key != "after") || len(parts) != 1 {
				queryErr = problemreference.ErrInvalid
			}
		}
		if queryErr != nil {
			problemResponseCatalog(w, http.StatusBadRequest)
			return
		}
		entries, next, listErr := h.catalog.List(r.Context(), values.Get("after"), values.Get("q"))
		result, err = struct {
			Problems []CatalogEntry `json:"problems"`
			Next     string         `json:"next"`
		}{entries, next}, listErr
	} else {
		id := strings.TrimPrefix(r.URL.Path, catalogPath+"/")
		if !identity.ValidID(id) {
			problemResponseCatalog(w, http.StatusNotFound)
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			problemResponseCatalog(w, http.StatusBadRequest)
			return
		}
		result, err = h.catalog.Detail(r.Context(), id)
	}
	if err != nil {
		switch {
		case errors.Is(err, problemreference.ErrStore):
			problemResponseCatalog(w, http.StatusServiceUnavailable)
		case errors.Is(err, problemreference.ErrInvalid):
			problemResponseCatalog(w, http.StatusBadRequest)
		case errors.Is(err, problemreference.ErrNotFound):
			problemResponseCatalog(w, http.StatusNotFound)
		default:
			problemResponseCatalog(w, http.StatusServiceUnavailable)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if h.trainingSubmit != nil && r.URL.Path != catalogPath {
		w.Header().Set("X-OJ-Training-Intake", "enabled")
	}
	_ = json.NewEncoder(w).Encode(result)
}

func problemResponseCatalog(w http.ResponseWriter, status int) {
	problem(w, status, "CATALOG_UNAVAILABLE", "题库请求暂不可用")
}
