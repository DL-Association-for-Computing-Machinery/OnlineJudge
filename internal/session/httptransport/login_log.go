package httptransport

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func loginOperation(path string) string {
	switch path {
	case "/api/v1/session/login/begin":
		return "begin"
	case "/api/v1/session/login/complete":
		return "complete"
	case "/api/v1/session/login/abandon":
		return "abandon"
	default:
		return "unknown"
	}
}

func recordLoginFailure(logger *slog.Logger, operation, stage string, elapsed time.Duration, err error) {
	if logger == nil {
		return
	}
	// Driver errors can contain private details. Report only bounded categories.
	logger.Warn("session login unavailable", "operation", operation, "stage", stage,
		"reason", loginFailureReason(err), "elapsed_ms", elapsed.Milliseconds())
}

func loginFailureReason(err error) string {
	switch {
	case errors.Is(err, redisstore.ErrUnknown):
		return "outcome_unknown"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, password.ErrBusy):
		return "password_busy"
	case errors.Is(err, password.ErrQueueTimeout):
		return "password_queue_timeout"
	case errors.Is(err, redisstore.ErrStore):
		return "redis"
	case errors.Is(err, identity.ErrStore):
		return "identity_store"
	case errors.Is(err, identity.ErrRecovering):
		return "recovery"
	default:
		return "other"
	}
}
