package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// DevicesPath is the opt-in read-only ordinary session list.
const DevicesPath = "/api/v1/session/devices"

type deviceList func(context.Context, redisstore.Credential) ([]redisstore.Device, error)

// WithDevices enables listing with the same HTTPS and cookie policy as Read.
func WithDevices(base http.Handler, list func(context.Context, redisstore.Credential) ([]redisstore.Device, error)) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || list == nil || h.devices != nil {
		return nil, errors.New("invalid session device list configuration")
	}
	next := *h
	next.devices = list
	return &next, nil
}

func (h *handler) serveDevices(w http.ResponseWriter, ctx context.Context, credential redisstore.Credential) {
	devices, err := h.devices(ctx, credential)
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
