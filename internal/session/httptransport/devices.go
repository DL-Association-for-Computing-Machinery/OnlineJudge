package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const (
	// DevicesPath lists the caller's active ordinary sessions.
	DevicesPath = "/api/v1/session/devices"
	// RevokeDevicePath revokes one other ordinary session.
	RevokeDevicePath = DevicesPath + "/revoke"
	// RevokeOtherDevicesPath revokes every other active ordinary session.
	RevokeOtherDevicesPath = DevicesPath + "/revoke-others"
)

// IsDevicePath is true for the exact session device endpoints.
func IsDevicePath(path string) bool {
	return path == DevicesPath || path == RevokeDevicePath || path == RevokeOtherDevicesPath
}

// DeviceSessions provides read and revoke operations for the current owner.
type DeviceSessions interface {
	ListDevices(context.Context, redisstore.Credential) ([]redisstore.Device, error)
	RevokeDevice(context.Context, redisstore.Credential, string) error
	RevokeOtherDevices(context.Context, redisstore.Credential) error
}

// WithDevices enables device management with the ordinary HTTPS cookie policy.
func WithDevices(base http.Handler, devices DeviceSessions) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || devices == nil || h.devices != nil {
		return nil, errors.New("invalid session device list configuration")
	}
	next := *h
	next.devices = devices
	return &next, nil
}

func (h *handler) serveDevices(w http.ResponseWriter, r *http.Request, ctx context.Context, credential redisstore.Credential) {
	if r.Method == http.MethodPost {
		h.revokeDevices(w, r, ctx, credential)
		return
	}
	devices, err := h.devices.ListDevices(ctx, credential)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		sessionError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Devices []redisstore.Device `json:"devices"`
	}{devices})
}

func (h *handler) revokeDevices(w http.ResponseWriter, r *http.Request, ctx context.Context, credential redisstore.Credential) {
	var err error
	if r.URL.Path == RevokeDevicePath {
		body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, 64))
		if readErr != nil {
			problem(w, http.StatusBadRequest, "INVALID_DEVICE", "无效会话标识")
			return
		}
		err = h.devices.RevokeDevice(ctx, credential, string(body))
	} else {
		err = h.devices.RevokeOtherDevices(ctx, credential)
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if errors.Is(err, redisstore.ErrDeviceNotFound) && !errors.Is(err, redisstore.ErrStore) {
		problem(w, http.StatusNotFound, "DEVICE_NOT_FOUND", "其他有效会话不存在")
		return
	}
	sessionError(w, err)
}
