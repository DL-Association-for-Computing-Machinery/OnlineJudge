// Package httpapi owns the external HTTP transport, without executing user code.
package httpapi

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// NewHandler builds the bootstrap API. Health reports process liveness only.
func NewHandler(experiments ...Experiment) http.Handler {
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	router.HandleMethodNotAllowed = true
	router.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) {
		problem(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用")
	}))
	router.GET("/api/v1/healthz", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "api", "scope": "process"})
	})
	if len(experiments) == 1 && experiments[0].Store != nil {
		registerExperiment(router, experiments[0])
	}
	router.NoRoute(func(c *gin.Context) {
		problem(c, http.StatusNotFound, "NOT_FOUND", "接口不存在")
	})
	router.NoMethod(func(c *gin.Context) {
		problem(c, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "请求方法不支持")
	})
	return router
}

func problem(c *gin.Context, status int, code, title string) {
	c.Header("Content-Type", "application/problem+json")
	c.Header("Cache-Control", "no-store")
	c.AbortWithStatusJSON(status, gin.H{
		"type": "about:blank", "title": title, "status": status, "code": code,
	})
}
