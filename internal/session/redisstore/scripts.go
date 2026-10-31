package redisstore

import (
	"context"
	_ "embed"
	"encoding/binary"
	"strconv"
	"time"
)

//go:embed lua/common.lua
var commonLua string

//go:embed lua/locate.lua
var locateLua string

//go:embed lua/retry.lua
var retryLua string

//go:embed lua/check.lua
var checkLua string

//go:embed lua/read.lua
var readLua string

//go:embed lua/touch.lua
var touchLua string

//go:embed lua/abandon.lua
var abandonLua string

func luaScript(op string) string {
	flags, body := "no-cluster", ""
	switch op {
	case "locate":
		flags, body = "no-writes,no-cluster", locateLua
	case "check":
		flags, body = "no-writes,no-cluster", checkLua
	case "read":
		flags, body = "no-writes,no-cluster", readLua
	case "retry":
		body = retryLua
	case "touch":
		body = touchLua
	case "abandon":
		body = abandonLua
	}
	return "#!lua flags=" + flags + "\n-- session-protocol:" + op + "\n" + commonLua + "\n" + body
}

func (s *Store) keys(a *Attempt) []string {
	group := a.state.ID + ":" + strconv.FormatInt(a.state.AuthGeneration, 10) + ":" + a.state.RecoveryID
	return []string{"oj-session:" + s.namespace + ":token:" + digest(a.credential.token), "oj-session:" + s.namespace + ":subject:" + digest(group)}
}

func (s *Store) args(a *Attempt) []any {
	return []any{digest(a.credential.token), a.state.ID, strconv.FormatInt(a.state.CredentialVersion, 10),
		strconv.FormatInt(a.state.AuthGeneration, 10), a.state.RecoveryID, strconv.FormatInt(a.created, 10),
		strconv.FormatInt(a.absolute, 10), strconv.FormatInt(a.initial, 10), s.policy.Idle.Milliseconds(), s.policy.Absolute.Milliseconds(), binding(a)}
}

// binding detects field drift, not malicious Redis writes or rollback. Fixed
// order plus standard-library length prefixes keeps the digest input unambiguous.
func binding(a *Attempt) string {
	var data []byte
	for _, field := range []string{"ordinary-v1", a.namespace, digest(a.credential.token), a.state.ID,
		strconv.FormatInt(a.state.CredentialVersion, 10), strconv.FormatInt(a.state.AuthGeneration, 10), a.state.RecoveryID,
		strconv.FormatInt(a.created, 10), strconv.FormatInt(a.absolute, 10), strconv.FormatInt(a.initial, 10)} {
		data = binary.AppendUvarint(data, uint64(len(field)))
		data = append(data, field...)
	}
	return digest(string(data))
}

func (s *Store) eval(ctx context.Context, op string, keys []string, args []any) ([]any, error) {
	values, err := s.client.Eval(ctx, luaScript(op), keys, args...).Slice()
	if err != nil {
		return nil, storageError{err}
	}
	if len(values) == 0 {
		return nil, storageError{ErrConflict}
	}
	code, ok := values[0].(int64)
	if !ok {
		return nil, storageError{ErrConflict}
	}
	switch code {
	case 1:
		return values[1:], nil
	case 0:
		if op == "check" {
			return nil, ErrUnknown
		}
		return nil, ErrNotFound
	case 2:
		return nil, ErrStale
	case 3:
		return nil, ErrConflict
	case 4:
		return nil, ErrLimit
	default:
		return nil, storageError{ErrConflict}
	}
}

func (s *Store) runAttempt(ctx context.Context, op string, a *Attempt) error {
	_, err := s.eval(ctx, op, s.keys(a), s.args(a))
	return err
}

type located struct{ attempt Attempt }

func (s *Store) locate(ctx context.Context, c Credential) (located, error) {
	if _, err := ParseCredential(c.token); err != nil {
		return located{}, err
	}
	key := "oj-session:" + s.namespace + ":token:" + digest(c.token)
	args := []any{digest(c.token), "", "", "", "", "", "", "", s.policy.Idle.Milliseconds(), s.policy.Absolute.Milliseconds(), ""}
	values, err := s.eval(ctx, "locate", []string{key}, args)
	if err != nil {
		return located{}, err
	}
	if len(values) != 12 {
		return located{}, storageError{ErrConflict}
	}
	fields := make([]string, len(values))
	for i, value := range values {
		var ok bool
		fields[i], ok = value.(string)
		if !ok {
			return located{}, storageError{ErrConflict}
		}
	}
	numbers := make([]int64, 6)
	for i, j := range []int{3, 4, 6, 7, 8, 9} {
		n, err := strconv.ParseInt(fields[j], 10, 64)
		if err != nil || n <= 0 {
			return located{}, storageError{ErrConflict}
		}
		numbers[i] = n
	}
	a := Attempt{namespace: s.namespace, credential: c, created: numbers[2], absolute: numbers[3], initial: numbers[4]}
	a.state.ID, a.state.Enabled = fields[2], true
	a.state.CredentialVersion, a.state.AuthGeneration, a.state.RecoveryID = numbers[0], numbers[1], fields[5]
	if !a.valid(s.namespace, s.policy) || fields[0] != "1" || fields[1] != digest(c.token) {
		return located{}, storageError{ErrConflict}
	}
	if fields[11] != binding(&a) {
		return located{}, ErrConflict
	}
	return located{attempt: a}, nil
}

func view(a *Attempt, idle int64) View {
	return View{SubjectID: a.state.ID, AuthGeneration: a.state.AuthGeneration, RecoveryID: a.state.RecoveryID,
		CreatedAt: time.UnixMilli(a.created), AbsoluteDeadline: time.UnixMilli(a.absolute), IdleExpiresAt: time.UnixMilli(idle)}
}
