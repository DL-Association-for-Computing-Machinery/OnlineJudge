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
	problemdata "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

const referenceDescribePath = "/api/v1/worker/reference/assignment"
const referenceMaterialPath = "/api/v1/worker/reference/material"
const differentialDescribePath = "/api/v1/worker/reference/differential/assignment"
const differentialMaterialPath = "/api/v1/worker/reference/differential/material"

func serveReferenceMaterials(w http.ResponseWriter, r *http.Request, s *problemreference.ReferenceMaterials, node submission.NodeIdentity, roots *x509.CertPool) {
	input, object, digest, ok := decodeReferenceMaterial(w, r)
	if !ok {
		return
	}
	if !input.Valid() || (r.URL.Path == differentialMaterialPath && !problemdata.ValidImportRevision(digest)) {
		problem(w, 400, "INVALID_REQUEST", "参考材料请求无效")
		return
	}
	if r.URL.Path == referenceDescribePath || r.URL.Path == differentialDescribePath {
		start := time.Now()
		var body any
		var remaining int64
		var err error
		if r.URL.Path == differentialDescribePath {
			d, e := s.DescribeDifferential(r.Context(), node, input)
			body, remaining, err = d, d.RemainingMS, e
		} else {
			d, e := s.Describe(r.Context(), node, input)
			body, remaining, err = d, d.RemainingMS, e
		}
		if err != nil {
			referenceMaterialError(w, err)
			return
		}
		if _, err := authenticateURI(r.TLS, roots, time.Now(), referenceIdentityURI); err != nil {
			referenceMaterialError(w, submission.ErrNode)
			return
		}
		deadline := start.Add(time.Duration(remaining) * time.Millisecond)
		if !time.Now().Before(deadline) || http.NewResponseController(w).SetWriteDeadline(deadline) != nil {
			referenceMaterialError(w, problemreference.ErrClaimUnavailable)
			return
		}
		writeJSON(w, body)
		return
	}
	var file *problemreference.ReferenceFile
	var deadline time.Time
	var err error
	if r.URL.Path == differentialMaterialPath {
		file, deadline, err = s.DownloadDifferential(r.Context(), node, input, digest, object)
	} else {
		file, deadline, err = s.Download(r.Context(), node, input, object)
	}
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
	w.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	w.Header().Set("X-Content-SHA256", object.SHA256)
	// On failure leave a truncated body; never append JSON to private bytes.
	_, _ = io.Copy(w, referenceDeadlineReader{ctx, file})
}

func decodeReferenceMaterial(w http.ResponseWriter, r *http.Request) (problemreference.MaterialInput, materialstore.Object, string, bool) {
	switch r.URL.Path {
	case differentialDescribePath:
		var input problemreference.MaterialInput
		ok := decode(w, r, &input)
		return input, materialstore.Object{}, "", ok
	case differentialMaterialPath:
		var input struct {
			problemreference.MaterialInput
			Object materialstore.Object `json:"object"`
			Digest string               `json:"differential_sha256"`
		}
		ok := decode(w, r, &input)
		return input.MaterialInput, input.Object, input.Digest, ok
	default:
		var input struct {
			problemreference.MaterialInput
			Object materialstore.Object `json:"object"`
		}
		ok := decode(w, r, &input)
		return input.MaterialInput, input.Object, "", ok
	}
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
