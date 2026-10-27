package workerapi

import (
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// NewHandler exposes lease and optional material operations behind direct mTLS.
// The store must authorize node state/version inside each operation's transaction.
func NewHandler(store submission.LeaseStore, roots *x509.CertPool, materialService ...*materials.Service) http.Handler {
	return newHandler(store, roots, time.Now, materialService...)
}

func newHandler(store submission.LeaseStore, roots *x509.CertPool, now func() time.Time, materialService ...*materials.Service) http.Handler {
	var service *materials.Service
	if len(materialService) > 0 {
		service = materialService[0]
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		node, err := authenticate(r.TLS, roots, now())
		if err != nil {
			problem(w, 401, "UNAUTHENTICATED", "需要有效节点证书")
			return
		}
		if r.Header.Get("Origin") != "" {
			problem(w, 403, "WORKER_ONLY", "接口仅供受控节点使用")
			return
		}
		if r.URL.Path != "/api/v1/worker/claim" && r.URL.Path != "/api/v1/worker/renew" && r.URL.Path != "/api/v1/worker/report" && r.URL.Path != "/api/v1/worker/assignment" && r.URL.Path != "/api/v1/worker/material" {
			problem(w, 404, "NOT_FOUND", "接口不存在")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			problem(w, 405, "METHOD_NOT_ALLOWED", "请求方法不支持")
			return
		}
		if store == nil {
			problem(w, 503, "WORKER_UNAVAILABLE", "节点服务暂时不可用")
			return
		}
		switch r.URL.Path {
		case "/api/v1/worker/assignment", "/api/v1/worker/material":
			serveMaterials(w, r, service, node, roots, now)
		case "/api/v1/worker/claim":
			var req struct{}
			if !decode(w, r, &req) {
				return
			}
			lease, err := store.Claim(r.Context(), node)
			if errors.Is(err, submission.ErrNoTask) {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusNoContent)
			} else if err != nil {
				storeError(w, err)
			} else {
				writeJSON(w, struct {
					Token    string    `json:"lease_token"`
					ID       string    `json:"submission_id"`
					Judge    int64     `json:"judge_generation"`
					Recovery int64     `json:"recovery_generation"`
					Attempt  int32     `json:"attempt_number"`
					Until    time.Time `json:"lease_until"`
				}{lease.Token, lease.SubmissionID, lease.JudgeGeneration, lease.RecoveryGeneration, lease.AttemptNumber, lease.Until})
			}
		case "/api/v1/worker/renew":
			var req struct {
				Token string `json:"lease_token"`
			}
			if !decode(w, r, &req) {
				return
			}
			if !validToken(w, req.Token) {
				return
			}
			until, err := store.Renew(r.Context(), node, req.Token)
			if err != nil {
				storeError(w, err)
				return
			}
			writeJSON(w, struct {
				Until time.Time `json:"lease_until"`
			}{until})
		case "/api/v1/worker/report":
			var req struct {
				Token    string `json:"lease_token"`
				Verdict  string `json:"verdict"`
				Evidence string `json:"evidence_sha256"`
			}
			if !decode(w, r, &req) {
				return
			}
			if !validToken(w, req.Token) {
				return
			}
			validVerdict := false
			for _, v := range []string{"AC", "WA", "CE", "RE", "TLE", "MLE", "OLE", "SE"} {
				validVerdict = validVerdict || v == req.Verdict
			}
			if !validVerdict || !digestPattern.MatchString(req.Evidence) {
				problem(w, 400, "INVALID_REQUEST", "结果格式无效")
				return
			}
			if err := store.Report(r.Context(), node, req.Token, submission.Result{Verdict: req.Verdict, EvidenceSHA256: req.Evidence}); err != nil {
				storeError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	})
}

func validToken(w http.ResponseWriter, token string) bool {
	if digestPattern.MatchString(token) {
		return true
	}
	problem(w, 400, "INVALID_REQUEST", "租约格式无效")
	return false
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		problem(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 请求")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	// A pointer to the supplied DTO also detects JSON null instead of treating it as {}.
	err = decoder.Decode(&value)
	if err == nil && value == nil {
		err = submission.ErrInvalid
	}
	if err == nil {
		var trailing any
		if next := decoder.Decode(&trailing); next != io.EOF {
			err = next
			if err == nil {
				err = submission.ErrInvalid
			}
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			problem(w, 413, "BODY_TOO_LARGE", "请求过大")
		} else {
			problem(w, 400, "INVALID_REQUEST", "请求格式无效")
		}
		return false
	}
	return true
}

func storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, submission.ErrNode):
		problem(w, 403, "NODE_UNAUTHORIZED", "节点权限已失效")
	case errors.Is(err, submission.ErrLease):
		problem(w, 409, "LEASE_INVALID", "租约已失效或不属于当前节点")
	case errors.Is(err, submission.ErrResultConflict):
		problem(w, 409, "RESULT_CONFLICT", "结果与已接受回报冲突")
	case errors.Is(err, submission.ErrInvalid):
		problem(w, 400, "INVALID_REQUEST", "材料请求格式无效")
	case errors.Is(err, submission.ErrNotFound):
		problem(w, 404, "MATERIAL_NOT_FOUND", "材料不在当前任务清单中")
	default:
		problem(w, 503, "WORKER_UNAVAILABLE", "操作状态未知；停止执行并保留原租约和结果")
	}
}

func problem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "status": status, "code": code, "title": title})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
