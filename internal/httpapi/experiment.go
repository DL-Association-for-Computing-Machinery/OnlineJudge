package httpapi

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/gin-gonic/gin"
)

// Experiment binds a controlled fixture identity to an opt-in local API.
// This is not a production login/session mechanism.
type Experiment struct {
	Store submission.Store
	Token string
}

func registerExperiment(router *gin.Engine, cfg Experiment) {
	expected := sha256.Sum256([]byte(cfg.Token))
	group := router.Group("/api/v1/experiments")
	group.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		auth := c.GetHeader("Authorization")
		provided := sha256.Sum256([]byte(strings.TrimPrefix(auth, "Bearer ")))
		if len(cfg.Token) < 32 || !strings.HasPrefix(auth, "Bearer ") || subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
			problem(c, http.StatusUnauthorized, "UNAUTHENTICATED", "需要实验凭据")
			return
		}
		// No browser cookies/CORS are accepted; a known token identifies only this fixture principal.
		if c.GetHeader("Origin") != "" {
			problem(c, http.StatusForbidden, "EXPERIMENT_ONLY", "实验接口仅供本机受控客户端使用")
			return
		}
		c.Next()
	})
	const subject, scope = "m0-operator", "m0-training"
	group.POST("/submissions", func(c *gin.Context) {
		mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || mediaType != "application/json" {
			problem(c, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 提交")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 6*submission.MaxSourceBytes+1024)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				problem(c, 413, "BODY_TOO_LARGE", "请求过大")
				return
			}
			problem(c, 400, "INVALID_SUBMISSION", "无法读取提交")
			return
		}
		if !utf8.Valid(body) {
			problem(c, 400, "INVALID_SUBMISSION", "提交必须使用 UTF-8")
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		var req submission.Request
		err = decoder.Decode(&req)
		if err == nil {
			var trailing any
			if decoder.Decode(&trailing) != io.EOF {
				err = submission.ErrInvalid
			}
		}
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				problem(c, 413, "BODY_TOO_LARGE", "请求过大")
				return
			}
			problem(c, 400, "INVALID_SUBMISSION", "提交格式无效")
			return
		}
		result, created, err := cfg.Store.Accept(c.Request.Context(), subject, scope, c.GetHeader("Idempotency-Key"), req)
		if err != nil {
			submissionError(c, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusAccepted
		}
		c.Header("Location", "/api/v1/experiments/submissions/"+result.ID)
		c.JSON(status, result)
	})
	group.GET("/submission-receipt", func(c *gin.Context) {
		result, err := cfg.Store.Receipt(c.Request.Context(), subject, scope, c.GetHeader("Idempotency-Key"))
		if err != nil {
			submissionError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	group.GET("/submissions/:id", func(c *gin.Context) {
		result, err := cfg.Store.Get(c.Request.Context(), subject, scope, c.Param("id"))
		if err != nil {
			submissionError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
}

func submissionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, submission.ErrInvalid):
		problem(c, 400, "INVALID_SUBMISSION", "提交字段或幂等键无效")
	case errors.Is(err, submission.ErrConflict):
		problem(c, 409, "IDEMPOTENCY_CONFLICT", "同一幂等键已用于不同内容")
	case errors.Is(err, submission.ErrNotFound):
		problem(c, 404, "NOT_FOUND", "尚未查到已接收提交，请保留原幂等键核验")
	case errors.Is(err, submission.ErrMaterial):
		problem(c, 503, "MATERIAL_UNAVAILABLE", "实验材料不可用")
	default:
		problem(c, 503, "ACCEPTANCE_UNKNOWN", "服务暂时不可用；请使用原幂等键核验接收状态")
	}
}
