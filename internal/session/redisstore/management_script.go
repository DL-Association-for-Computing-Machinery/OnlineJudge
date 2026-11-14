package redisstore

import (
	"context"
	_ "embed"
)

//go:embed lua/management.lua
var managementLua string

func (s *Management) run(ctx context.Context, op string, credential Credential, expected string) (managementRecord, error) {
	var zero managementRecord
	flags := "no-cluster"
	if op == "locate" || op == "read" {
		flags = "no-writes,no-cluster"
	}
	values, err := s.store.client.Eval(ctx, "#!lua flags="+flags+"\n-- session-protocol:management-"+op+"\n"+managementLua, []string{s.tokenKey(credential)},
		op, expected, s.policy.Idle.Milliseconds(), s.policy.Absolute.Milliseconds(), managementMaxRecord).Slice()
	if err != nil {
		return zero, storageError{err}
	}
	if len(values) == 1 {
		switch values[0] {
		case int64(0):
			return zero, ErrStale
		case int64(1):
			if op == "revoke" {
				return zero, nil
			}
		case int64(2):
			return zero, ErrConflict
		}
	}
	if len(values) == 2 && values[0] == int64(1) && op != "revoke" {
		if raw, ok := values[1].(string); ok {
			return s.decode(raw, credential)
		}
	}
	return zero, storageError{ErrConflict}
}
