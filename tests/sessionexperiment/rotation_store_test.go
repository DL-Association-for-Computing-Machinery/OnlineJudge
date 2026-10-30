package sessionexperiment

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/alexedwards/scs/v2"
)

var errRotationConflict = errors.New("rotation conflict")

// All validation precedes the first write. Atomic execution prevents another
// request from observing a half-transfer, but Redis script errors do not roll
// back writes. The declared script rejects already-OOM entry, not runtime OOM.
const rotateLua = `#!lua
local function integer(n)
  return n and n == n and n > 0 and n < math.huge and n == math.floor(n)
end
local t = redis.call('TIME')
local now = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
local deadline, expiry, maximum = tonumber(ARGV[4]), tonumber(ARGV[5]), tonumber(ARGV[6])
local indexMaximum = tonumber(ARGV[7])
if not integer(deadline) or not integer(expiry) or not integer(maximum) or not integer(indexMaximum) then return 3 end
if deadline <= now or expiry <= now then return 2 end
if expiry > deadline or deadline > now + maximum then return 3 end
if redis.call('TYPE', KEYS[1]).ok == 'none' then return 2 end
if redis.call('TYPE', KEYS[1]).ok ~= 'hash' then return 3 end
local data = redis.call('HGET', KEYS[1], 'data')
if not data then return 2 end
local oldExpiry = tonumber(redis.call('HGET', KEYS[1], 'expiry'))
if not integer(oldExpiry) or oldExpiry > deadline then return 3 end
if oldExpiry <= now then return 2 end
if data ~= ARGV[2] or redis.call('HGET', KEYS[1], 'subject') ~= ARGV[1]
  or redis.call('HGET', KEYS[1], 'deadline') ~= ARGV[4]
  or redis.call('PEXPIRETIME', KEYS[1]) ~= deadline
  or expiry < oldExpiry then return 3 end
if redis.call('EXISTS', KEYS[2]) ~= 0 then return 3 end
if redis.call('TYPE', KEYS[3]).ok ~= 'zset' then return 3 end
if tonumber(redis.call('ZSCORE', KEYS[3], KEYS[1])) ~= oldExpiry then return 3 end
local latest = redis.call('ZREVRANGE', KEYS[3], 0, 0, 'WITHSCORES')
local lastExpiry = tonumber(latest[2])
if not integer(lastExpiry) or lastExpiry > now + indexMaximum or redis.call('PEXPIRETIME', KEYS[3]) < lastExpiry
  or redis.call('ZCOUNT', KEYS[3], '(' .. now, '+inf') > 3 then return 3 end
local indexDeadline = math.max(lastExpiry, expiry)
redis.call('HSET', KEYS[2], 'subject', ARGV[1], 'data', ARGV[3], 'deadline', ARGV[4], 'expiry', ARGV[5])
redis.call('PEXPIREAT', KEYS[2], deadline)
redis.call('HDEL', KEYS[1], 'data')
redis.call('ZREM', KEYS[3], KEYS[1])
redis.call('ZADD', KEYS[3], expiry, KEYS[2])
redis.call('PEXPIREAT', KEYS[3], indexDeadline)
return 0
`

// One request owns this adapter. RenewToken's delete is staged until Commit
// can atomically exchange the credential and its one existing admission slot.
type rotationStore struct {
	*guardedStore
	old           string
	source        []byte
	staged, tried bool
}

func (s *rotationStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	if token == "" || token != s.old || len(s.source) != 0 {
		return nil, false, errRotationConflict
	}
	b, found, err := s.guardedStore.FindCtx(ctx, token)
	if err == nil && found {
		s.source = bytes.Clone(b)
	}
	return b, found, err
}

func (s *rotationStore) DeleteCtx(_ context.Context, token string) error {
	if token == "" || token != s.old || len(s.source) == 0 || s.staged || s.tried {
		return errRotationConflict
	}
	s.staged = true
	return nil
}

func (s *rotationStore) CommitCtx(ctx context.Context, token string, data []byte, expiry time.Time) error {
	if !s.staged || s.tried || token == "" || token == s.old {
		return errRotationConflict
	}
	s.tried = true
	deadline, values, err := (scs.GobCodec{}).Decode(s.source)
	if err != nil {
		return err
	}
	nextDeadline, nextValues, err := (scs.GobCodec{}).Decode(data)
	if err != nil {
		return err
	}
	// Gob map ordering may change. Compare decoded identity/policy instead of
	// rejecting equivalent encodings; the Lua CAS uses the original raw bytes.
	if !nextDeadline.Equal(deadline) || !reflect.DeepEqual(values, nextValues) {
		return errRotationConflict
	}
	err = s.withPayload(ctx, s.source, func(hash string, deadline time.Time, maximum time.Duration) error {
		// A personal index can also contain remembered siblings. Its bound is
		// the namespace maximum, not this source session's shorter policy.
		indexMaximum := 7 * 24 * time.Hour
		if s.admin {
			indexMaximum = 8 * time.Hour
		}
		result, err := s.client.Eval(ctx, rotateLua,
			[]string{s.key(s.old), s.key(token), s.prefix + "subject:" + hash},
			hash, s.source, data, deadline.UnixMilli(), expiry.UnixMilli(), maximum.Milliseconds(), indexMaximum.Milliseconds()).Int()
		if err != nil {
			return err
		}
		switch result {
		case 0:
			return nil
		case 2:
			return errStale
		default:
			return errRotationConflict
		}
	})
	if err == nil && s.rotationReceipt != nil {
		return s.rotationReceipt(newSaveAttempt(token, data, expiry))
	}
	return err
}
