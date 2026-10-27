package workerapi

import (
	"crypto/x509"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func serveMaterials(w http.ResponseWriter, r *http.Request, service *materials.Service, node submission.NodeIdentity, roots *x509.CertPool, now func() time.Time) {
	// A disabled material service never silently exposes database source alone.
	if service == nil {
		storeError(w, materials.ErrUnavailable)
		return
	}
	if r.URL.Path == "/api/v1/worker/assignment" {
		var req struct {
			Token string `json:"lease_token"`
		}
		if !decode(w, r, &req) || !validToken(w, req.Token) {
			return
		}
		value, err := service.Describe(r.Context(), node, req.Token)
		if err != nil {
			storeError(w, err)
			return
		}
		if _, err := authenticate(r.TLS, roots, now()); err != nil {
			problem(w, 401, "UNAUTHENTICATED", "需要有效节点证书")
			return
		}
		writeJSON(w, value)
		return
	}
	var req struct {
		Token  string `json:"lease_token"`
		TestID string `json:"test_id"`
		Kind   string `json:"kind"`
	}
	if !decode(w, r, &req) || !validToken(w, req.Token) {
		return
	}
	file, err := service.Download(r.Context(), node, req.Token, req.TestID, req.Kind)
	if err != nil {
		storeError(w, err)
		return
	}
	defer file.Close()
	if _, err := authenticate(r.TLS, roots, now()); err != nil {
		problem(w, 401, "UNAUTHENTICATED", "需要有效节点证书")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	w.Header().Set("X-Content-SHA256", file.SHA256)
	// A failed transfer stays truncated; never append a JSON error to material bytes.
	_, _ = io.Copy(w, file)
}
