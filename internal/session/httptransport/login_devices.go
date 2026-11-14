package httptransport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

const (
	// PendingDevicesPath reads available choices for a pending, full login.
	PendingDevicesPath = "/api/v1/session/login/devices"
	// ReplaceDevicePath confirms one selected exchange for that pending login.
	ReplaceDevicePath = "/api/v1/session/login/replace-device"
)

// IsPendingDevicePath reports the exact pending-login device routes.
func IsPendingDevicePath(path string) bool {
	return path == PendingDevicesPath || path == ReplaceDevicePath
}

func (h *handler) servePendingDevices(w http.ResponseWriter, r *http.Request, ctx context.Context, c redisstore.Continuation, fail func(string, error)) {
	if r.URL.Path == PendingDevicesPath {
		devices, err := h.login.PendingDevices(ctx, c)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		if err != nil {
			fail("devices", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Devices []redisstore.Device `json:"devices"`
		}{devices})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64))
	if err != nil {
		loginError(w, redisstore.ErrInvalid)
		return
	}
	credential, err := h.login.ReplaceDevice(ctx, c, string(body))
	finishLogin(w, ctx, credential, err, func(err error) { fail("replace_device", err) })
}
