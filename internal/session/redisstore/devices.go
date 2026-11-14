package redisstore

import (
	"context"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// Device describes one active browser session. ID is a selection handle, never
// a credential or evidence that the session belongs to a physical device.
type Device struct {
	ID               string    `json:"id"`
	Current          bool      `json:"current"`
	CreatedAt        time.Time `json:"created_at"`
	IdleExpiresAt    time.Time `json:"idle_expires_at"`
	AbsoluteDeadline time.Time `json:"absolute_deadline"`
}

// ListDevices returns an atomic read of the current session's active slots.
// The current cookie and account authority are rechecked; nothing is renewed.
func (s *Store) ListDevices(ctx context.Context, c Credential) ([]Device, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	located, err := s.locate(ctx, c)
	if err != nil {
		return nil, err
	}
	a := &located.attempt
	var devices []Device
	err = s.authority.WithSession(ctx, a.state.ID, identity.SessionVersion{AuthGeneration: a.state.AuthGeneration, RecoveryID: a.state.RecoveryID}, func(call context.Context, _ identity.SessionState) error {
		rows, err := s.eval(call, "devices", s.keys(a), s.args(a))
		if err != nil {
			return err
		}
		devices, err = decodeDevices(a, rows)
		return err
	})
	if err != nil {
		return nil, err
	}
	return devices, nil
}

func decodeDevices(owner *Attempt, rows []any) ([]Device, error) {
	if len(rows) < 1 || len(rows) > 3 {
		return nil, ErrConflict
	}
	devices := make([]Device, 0, len(rows))
	seenCurrent := false
	for _, value := range rows {
		row, ok := value.([]any)
		if !ok || len(row) != 12 {
			return nil, ErrConflict
		}
		var fields [12]string
		for i, v := range row {
			fields[i], ok = v.(string)
			if !ok {
				return nil, ErrConflict
			}
		}
		device, err := decodeDevice(owner, fields)
		if err != nil || (device.Current && seenCurrent) {
			return nil, ErrConflict
		}
		seenCurrent = seenCurrent || device.Current
		devices = append(devices, device)
	}
	if !seenCurrent {
		return nil, ErrConflict
	}
	return devices, nil
}

func decodeDevice(owner *Attempt, fields [12]string) (Device, error) {
	var n [5]int64
	for i, j := range []int{3, 6, 7, 8, 9} {
		x, err := strconv.ParseInt(fields[j], 10, 64)
		if err != nil || x <= 0 {
			return Device{}, ErrConflict
		}
		n[i] = x
	}
	record := *owner
	record.state.CredentialVersion, record.created, record.absolute, record.initial = n[0], n[1], n[2], n[3]
	if fields[11] != recordBinding(&record, fields[1]) {
		return Device{}, ErrConflict
	}
	return Device{ID: digest("ordinary-device-v1:" + owner.namespace + ":" + fields[1]),
		Current: fields[1] == digest(owner.credential.token), CreatedAt: time.UnixMilli(n[1]),
		AbsoluteDeadline: time.UnixMilli(n[2]), IdleExpiresAt: time.UnixMilli(n[4])}, nil
}
