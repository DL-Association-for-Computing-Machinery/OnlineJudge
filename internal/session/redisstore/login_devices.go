package redisstore

import (
	"context"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

// PendingDevices lists only the full set of ordinary slots for a valid pending
// login. Its handles are scoped to that particular candidate, not to a session.
func (l *Login) PendingDevices(ctx context.Context, c Continuation) ([]Device, error) {
	_, result, err := l.pendingDevices(ctx, c, "")
	return result, err
}

// ReplaceDevice exchanges exactly one selected slot for the original candidate.
// A lost reply is recovered through Complete, never by evicting a second slot.
func (l *Login) ReplaceDevice(ctx context.Context, c Continuation, selection string) (LoginGrant, error) {
	if len(selection) != 64 {
		return LoginGrant{}, ErrInvalid
	}
	a, _, err := l.pendingDevices(ctx, c, selection)
	if err != nil {
		return LoginGrant{}, err
	}
	return l.checkGrant(ctx, a)
}

func (l *Login) pendingDevices(ctx context.Context, c Continuation, selection string) (*Attempt, []Device, error) {
	a, err := l.open(c)
	if err != nil {
		return nil, nil, err
	}
	s := l.store
	if err := s.check(ctx); err != nil {
		return nil, nil, err
	}
	keys, args := s.keys(a), s.args(a)
	var result []Device
	err = s.authority.WithSessionCredentials(ctx, a.state.ID, a.state.Revision, a.state.RecoveryID, func(call context.Context, _ identity.SessionState) error {
		rows, err := s.eval(call, "pending-devices", keys, args)
		if err != nil {
			return err
		}
		devices, err := decodeDevices(a, rows, false)
		if err != nil || len(devices) != 3 {
			return ErrConflict
		}
		selected := false
		for _, device := range devices {
			f := device.fields
			device.ID = digest("pending-device-v1:" + a.namespace + ":" + digest(a.credential.token) + ":" + f[1])
			result = append(result, device.Device)
			target := "0"
			if device.ID == selection {
				target, selected = "1", true
			}
			args = append(args, f[1], deviceInvariant(f), target)
		}
		if selection == "" {
			return nil
		}
		if !selected {
			return ErrConflict
		}
		return s.runReplacement(call, keys, args)
	})
	if err != nil {
		return nil, nil, err
	}
	return a, result, nil
}

func (s *Store) runReplacement(ctx context.Context, keys []string, args []any) error {
	rows, err := s.eval(ctx, "replace-device", keys, args)
	if err == nil && len(rows) != 0 {
		return storageError{ErrConflict}
	}
	return err
}
