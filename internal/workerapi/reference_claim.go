package workerapi

import (
	"crypto/x509"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

const referenceClaimPath = "/api/v1/worker/reference/claim"

var referenceIdentityURI = regexp.MustCompile(`^spiffe://onlinejudge/reference-worker/([A-Za-z0-9_-]{1,128})/([1-9][0-9]{0,18})$`)

// WithReferenceClaims adds the opt-in route without accepting reference
// credentials on submission routes or regular credentials on reference claims.
func WithReferenceClaims(next http.Handler, claims problemreference.WorkerClaimer, roots *x509.CertPool, materials ...*problemreference.ReferenceMaterials) http.Handler {
	if claims == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		materialPath := r.URL.Path == referenceDescribePath || r.URL.Path == referenceMaterialPath
		if r.URL.Path != referenceClaimPath && !materialPath {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		node, err := authenticateURI(r.TLS, roots, time.Now(), referenceIdentityURI)
		if err != nil {
			problem(w, 401, "UNAUTHENTICATED", "需要有效参考验证节点证书")
			return
		}
		if len(r.Header.Values("Origin")) != 0 {
			problem(w, 403, "WORKER_ONLY", "接口仅供受控节点使用")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			problem(w, 405, "METHOD_NOT_ALLOWED", "请求方法不支持")
			return
		}
		if materialPath {
			if len(materials) == 0 || materials[0] == nil {
				problem(w, 404, "NOT_FOUND", "接口不存在")
				return
			}
			serveReferenceMaterials(w, r, materials[0], node, roots)
			return
		}
		var input problemreference.ClaimInput
		if !decode(w, r, &input) {
			return
		}
		if !input.Valid() {
			problem(w, 400, "INVALID_REQUEST", "参考领取格式无效")
			return
		}
		claim, err := claims.ClaimReference(r.Context(), node, input)
		switch {
		case errors.Is(err, problemreference.ErrClaimUnavailable):
			problem(w, 409, "REFERENCE_UNAVAILABLE", "参考验证申请暂不可领取")
		case err != nil:
			storeError(w, err)
		case !claim.ValidFor(input):
			problem(w, 503, "WORKER_UNAVAILABLE", "领取状态未知；不得执行或自动重试")
		default:
			writeJSON(w, claim)
		}
	})
}
