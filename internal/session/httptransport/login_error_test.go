package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func TestLoginUnavailableLogsOnlyFixedDiagnostics(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := &handler{config: Config{Timeout: time.Second, Logger: logger}}
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("private-cancellation-detail"))
	r := httptest.NewRequest(http.MethodPost, "/api/v1/session/login/begin?private-query", strings.NewReader("private-body")).WithContext(ctx)
	r.Header.Set("Cookie", "private-cookie")
	w := httptest.NewRecorder()
	h.serveLogin(w, r, netip.Addr{})
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "LOGIN_UNAVAILABLE") ||
		len(w.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("diagnostics changed the existing login failure contract")
	}
	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil || len(record) != 7 ||
		record["msg"] != "session login unavailable" || record["operation"] != "begin" ||
		record["stage"] != "request" || record["reason"] != "canceled" || strings.Contains(logs.String(), "private-") {
		t.Fatal("login failure diagnostics included unexpected fields or values")
	}
	if ms, ok := record["elapsed_ms"].(float64); !ok || ms < 0 {
		t.Fatal("login failure omitted a nonnegative elapsed time")
	}
	logs.Reset()
	r = httptest.NewRequest(http.MethodPost, "/api/v1/session/login/complete", nil)
	w = httptest.NewRecorder()
	h.serveLogin(w, r, netip.Addr{})
	if w.Code != http.StatusBadRequest || logs.Len() != 0 {
		t.Fatal("ordinary rejection should not emit a service failure event")
	}
}

func TestLoginFailureSeparatesPasswordPressureFromDeadlineAndStore(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{password.ErrBusy, "password_busy"},
		{password.ErrQueueTimeout, "password_queue_timeout"},
		{errors.Join(redisstore.ErrStore, context.DeadlineExceeded), "deadline"},
		{errors.Join(identity.ErrGuardedOperation, redisstore.ErrStore), "redis"},
		{identity.ErrStore, "identity_store"},
		{errors.Join(redisstore.ErrUnknown, context.DeadlineExceeded), "outcome_unknown"},
	} {
		if got := loginFailureReason(tc.err); got != tc.want {
			t.Errorf("reason = %q, want %q", got, tc.want)
		}
	}
}

func TestLoginUnknownErrorsWinBeforeCookiePublication(t *testing.T) {
	for _, unknown := range []error{identity.ErrStore, redisstore.ErrStore, identity.ErrRecovering, redisstore.ErrUnknown, context.Canceled, context.DeadlineExceeded} {
		for _, refused := range []error{redisstore.ErrStale, redisstore.ErrLimit, redisstore.ErrConflict, redisstore.ErrInvalid, redisstore.ErrLoginRate, password.ErrPassword, identity.ErrCredentials} {
			w := httptest.NewRecorder()
			loginError(w, errors.Join(unknown, identity.ErrGuardedOperation, refused))
			if w.Code != 503 || len(w.Header().Values("Set-Cookie")) != 0 {
				t.Fatal("uncertain login result published a cookie or deterministic refusal")
			}
		}
	}
}
