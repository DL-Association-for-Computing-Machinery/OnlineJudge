package workerapi

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

const referenceDescribePath = "/api/v1/worker/reference/assignment"
const referenceMaterialPath = "/api/v1/worker/reference/material"

func serveReferenceMaterials(w http.ResponseWriter, r *http.Request, s *problemreference.ReferenceMaterials, node submission.NodeIdentity, roots *x509.CertPool) {
	var input struct {
		problemreference.MaterialInput
		Object materialstore.Object `json:"object"`
	}
	if !decode(w, r, &input) {
		return
	}
	if !input.Valid() {
		problem(w, 400, "INVALID_REQUEST", "参考材料请求无效")
		return
	}
	if r.URL.Path == referenceDescribePath {
		start := time.Now()
		d, err := s.Describe(r.Context(), node, input.MaterialInput)
		if err != nil {
			referenceMaterialError(w, err)
			return
		}
		if _, err := authenticateURI(r.TLS, roots, time.Now(), referenceIdentityURI); err != nil {
			referenceMaterialError(w, submission.ErrNode)
			return
		}
		if http.NewResponseController(w).SetWriteDeadline(start.Add(time.Duration(d.RemainingMS)*time.Millisecond)) != nil {
			referenceMaterialError(w, problemreference.ErrClaimUnavailable)
			return
		}
		writeJSON(w, d)
		return
	}
	file, deadline, err := s.Download(r.Context(), node, input.MaterialInput, input.Object)
	if err != nil {
		referenceMaterialError(w, err)
		return
	}
	defer func() { _ = file.Close() }()
	if _, err := authenticateURI(r.TLS, roots, time.Now(), referenceIdentityURI); err != nil {
		referenceMaterialError(w, submission.ErrNode)
		return
	}
	// A context cannot interrupt a blocked network Write. Require the actual
	// connection deadline as well, before releasing any byte to a slow peer.
	if !time.Now().Before(deadline) || http.NewResponseController(w).SetWriteDeadline(deadline) != nil {
		referenceMaterialError(w, problemreference.ErrClaimUnavailable)
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(input.Object.Size, 10))
	w.Header().Set("X-Content-SHA256", input.Object.SHA256)
	// On failure leave a truncated body; never append JSON to private bytes.
	_, _ = io.Copy(w, referenceDeadlineReader{ctx, file})
}

type referenceDeadlineReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r referenceDeadlineReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(b)
}

func referenceMaterialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, submission.ErrNode):
		problem(w, 401, "UNAUTHENTICATED", "需要有效参考验证节点证书")
	case errors.Is(err, problemreference.ErrClaimUnavailable):
		problem(w, 409, "REFERENCE_UNAVAILABLE", "参考材料暂不可领取")
	default:
		problem(w, 503, "WORKER_UNAVAILABLE", "参考材料暂不可用")
	}
}
