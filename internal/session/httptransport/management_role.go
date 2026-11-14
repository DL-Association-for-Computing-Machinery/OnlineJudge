package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/jsoninput"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// ManagementRolePath applies one sensitive platform role change.
const ManagementRolePath = "/api/v1/platform-admin/roles/change"

// ManagementRoleChanges performs fresh password verification and one role write.
type ManagementRoleChanges interface {
	Change(context.Context, redisstore.Credential, netip.Addr, []byte, identity.PlatformAdminRoleRequest) (identity.PlatformAdminRole, error)
}

// WithManagementRoleChanges enables the separately configured write endpoint.
func WithManagementRoleChanges(base http.Handler, roles ManagementRoleChanges) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || h.management == nil || roles == nil || h.managementRoles != nil {
		return nil, errors.New("invalid management role configuration")
	}
	next := *h
	next.managementRoles = roles
	return &next, nil
}

func (h *handler) serveManagementRole(w http.ResponseWriter, r *http.Request, source netip.Addr) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	if h.managementRoles == nil {
		problem(w, 404, "NOT_FOUND", "接口不存在")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		problem(w, 405, "METHOD_NOT_ALLOWED", "请求方法不支持")
		return
	}
	if !h.sameOrigin(r) {
		problem(w, 403, "ORIGIN_REJECTED", "请求来源不允许")
		return
	}
	value, present, err := cookieValue(r, ManagementCookieName)
	credential, parseErr := redisstore.ParseCredential(value)
	if err != nil || !present || parseErr != nil {
		managementError(w, identity.ErrPlatformAdminRequired)
		return
	}
	if !source.IsValid() {
		source, err = loginSource(r.RemoteAddr)
		if err != nil {
			problem(w, 403, "ORIGIN_REJECTED", "请求来源不允许")
			return
		}
	}
	req, secret, ok := managementRoleBody(w, r)
	defer clear(secret)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	role, err := h.managementRoles.Change(ctx, credential, source, secret, req)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		managementRoleError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(role)
}

func managementRoleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrStore), errors.Is(err, redisstore.ErrStore), errors.Is(err, identity.ErrGuardedOperation),
		errors.Is(err, identity.ErrRecovering), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		problem(w, 503, "MANAGEMENT_CHANGE_UNCONFIRMED", "角色变更结果未确认，请核对当前修订")
	case errors.Is(err, identity.ErrConflict):
		problem(w, 409, "MANAGEMENT_ROLE_CONFLICT", "目标角色修订已变化")
	case errors.Is(err, identity.ErrNotFound):
		problem(w, 404, "MANAGEMENT_TARGET_NOT_FOUND", "目标账号不存在")
	case errors.Is(err, identity.ErrInvalid), errors.Is(err, password.ErrPassword):
		problem(w, 400, "INVALID_MANAGEMENT_CHANGE", "管理变更请求无效")
	case errors.Is(err, redisstore.ErrLoginRate):
		problem(w, 429, "LOGIN_RATE_LIMITED", "验证请求过于频繁，请稍后重试")
	default:
		managementError(w, err)
	}
}

func managementRoleBody(w http.ResponseWriter, r *http.Request) (identity.PlatformAdminRoleRequest, []byte, bool) {
	var zero identity.PlatformAdminRoleRequest
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		problem(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 提交")
		return zero, nil, false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	defer clear(body)
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		problem(w, 413, "BODY_TOO_LARGE", "请求过大")
		return zero, nil, false
	}
	invalid := func() (identity.PlatformAdminRoleRequest, []byte, bool) {
		problem(w, 400, "INVALID_MANAGEMENT_CHANGE", "管理变更请求格式无效")
		return zero, nil, false
	}
	if err != nil || !jsoninput.ValidUnicode(body) {
		return invalid()
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if start, err := d.Token(); err != nil || start != json.Delim('{') {
		return invalid()
	}
	seen := make(map[string]bool, 5)
	var req identity.PlatformAdminRoleRequest
	var secret string
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[name] {
			return invalid()
		}
		seen[name] = true
		var dest any
		switch name {
		case "subject_id":
			dest = &req.SubjectID
		case "expected_revision":
			dest = &req.ExpectedRevision
		case "enabled":
			dest = &req.Enabled
		case "reason":
			dest = &req.Reason
		case "password":
			dest = &secret
		default:
			return invalid()
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, dest) != nil {
			clear(raw)
			return invalid()
		}
		clear(raw)
	}
	if end, err := d.Token(); err != nil || end != json.Delim('}') || len(seen) != 5 || !req.Valid() || secret == "" {
		return invalid()
	}
	if _, err := d.Token(); err != io.EOF {
		return invalid()
	}
	return req, []byte(secret), true
}
