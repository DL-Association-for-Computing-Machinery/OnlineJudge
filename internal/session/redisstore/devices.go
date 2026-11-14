package redisstore

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// ErrDeviceNotFound does not disclose whether the handle belongs to another
// account, the current cookie, or a session which is no longer active.
var ErrDeviceNotFound = errors.New("other active session not found")

// MatchesDeviceCookie binds a browser action to the session displayed when it
// listed devices. The handle is only a precondition; normal auth still applies.
func (s *Store) MatchesDeviceCookie(c Credential, id string) bool {
	return len(id) == 64 && deviceID(s.namespace, digest(c.token)) == id
}

func deviceID(namespace, tokenDigest string) string {
	return digest("ordinary-device-v1:" + namespace + ":" + tokenDigest)
}

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
func (s *Store) ListDevices(ctx context.Context, c Credential) (DeviceList, error) {
	records, err := s.withDevices(ctx, c, nil)
	if err != nil {
		return DeviceList{}, err
	}
	devices := make([]Device, 0, len(records))
	for _, record := range records {
		devices = append(devices, record.Device)
	}
	return DeviceList{SubjectID: records[0].fields[2], Devices: devices}, nil
}

// DeviceList identifies the account observed in the same authenticated read.
type DeviceList struct {
	SubjectID string   `json:"subject_id"`
	Devices   []Device `json:"devices"`
}

// RevokeDevice revokes one other active session by its selection handle.
func (s *Store) RevokeDevice(ctx context.Context, c Credential, id string) error {
	if len(id) != 64 {
		return ErrDeviceNotFound
	}
	raw, err := hex.DecodeString(id)
	if err != nil || hex.EncodeToString(raw) != id {
		return ErrDeviceNotFound
	}
	return s.revokeDevices(ctx, c, id)
}

// RevokeOtherDevices revokes the other active sessions and retains the caller.
func (s *Store) RevokeOtherDevices(ctx context.Context, c Credential) error {
	return s.revokeDevices(ctx, c, "")
}

func (s *Store) revokeDevices(ctx context.Context, c Credential, id string) error {
	_, err := s.withDevices(ctx, c, func(call context.Context, a *Attempt, records []deviceRecord) error {
		args, selected := s.args(a), false
		for _, device := range records {
			target := "0"
			if !device.Current && (id == "" || device.ID == id) {
				target, selected = "1", true
			}
			f := device.fields
			args = append(args, f[1], deviceInvariant(f), target)
		}
		if !selected {
			if id != "" {
				return ErrDeviceNotFound
			}
			return nil
		}
		result, err := s.eval(call, "revoke-devices", s.keys(a), args)
		if err == nil && len(result) != 0 {
			return storageError{ErrConflict}
		}
		return err
	})
	return err
}

type deviceRecord struct {
	Device
	fields [12]string
}

func deviceInvariant(f [12]string) string {
	// The protocol's validated decimal and hex fields cannot contain a colon.
	return f[3] + ":" + f[6] + ":" + f[7] + ":" + f[8] + ":" + f[11]
}

func (s *Store) withDevices(ctx context.Context, c Credential, action func(context.Context, *Attempt, []deviceRecord) error) ([]deviceRecord, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	located, err := s.locate(ctx, c)
	if err != nil {
		return nil, err
	}
	a := &located.attempt
	var devices []deviceRecord
	err = s.authority.WithSession(ctx, a.state.ID, identity.SessionVersion{AuthGeneration: a.state.AuthGeneration, RecoveryID: a.state.RecoveryID}, func(call context.Context, _ identity.SessionState) error {
		rows, err := s.eval(call, "devices", s.keys(a), s.args(a))
		if err != nil {
			return err
		}
		devices, err = decodeDevices(a, rows)
		if err == nil && action != nil {
			return action(call, a, devices)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return devices, nil
}

func decodeDevices(owner *Attempt, rows []any) ([]deviceRecord, error) {
	if len(rows) < 1 || len(rows) > 3 {
		return nil, ErrConflict
	}
	devices := make([]deviceRecord, 0, len(rows))
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
		devices = append(devices, deviceRecord{Device: device, fields: fields})
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
	return Device{ID: deviceID(owner.namespace, fields[1]),
		Current: fields[1] == digest(owner.credential.token), CreatedAt: time.UnixMilli(n[1]),
		AbsoluteDeadline: time.UnixMilli(n[2]), IdleExpiresAt: time.UnixMilli(n[4])}, nil
}
