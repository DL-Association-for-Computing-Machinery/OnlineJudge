package sessionexperiment

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"time"
)

var (
	errOutcomeUnknown    = errors.New("admission outcome unknown")
	errAdmissionConflict = errors.New("admission record conflict")
)

// A missing receipt cannot prove an in-flight save failed. Only a complete,
// currently authorized record is positive evidence, including its slot index.
const checkAdmissionLua = `#!lua flags=no-writes
local t = redis.call('TIME')
local now = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
local deadline, expiry = tonumber(ARGV[3]), tonumber(ARGV[4])
if deadline <= now or expiry <= now then return 2 end
if expiry > deadline or deadline > now + tonumber(ARGV[5]) then return 3 end
if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
if redis.call('HGET', KEYS[1], 'subject') ~= ARGV[1]
  or redis.call('HGET', KEYS[1], 'deadline') ~= ARGV[3] then return 3 end
local data = redis.call('HGET', KEYS[1], 'data')
if not data then return 2 end
if data ~= ARGV[2] or redis.call('HGET', KEYS[1], 'expiry') ~= ARGV[4]
  or redis.call('PEXPIRETIME', KEYS[1]) ~= deadline
  or tonumber(redis.call('ZSCORE', KEYS[2], KEYS[1])) ~= expiry then return 3 end
local latest = redis.call('ZREVRANGE', KEYS[2], 0, 0, 'WITHSCORES')
if #latest == 0 or redis.call('PEXPIRETIME', KEYS[2]) < tonumber(latest[2]) then return 3 end
if redis.call('ZCOUNT', KEYS[2], '(' .. now, '+inf') > 3 then return 3 end
return 1
`

// A tombstone is required even if the original save has not arrived. Merely
// deleting an absent token would let that delayed save create a new session.
const abandonAdmissionLua = `#!lua
local t = redis.call('TIME')
local now = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
local deadline, expiry = tonumber(ARGV[2]), tonumber(ARGV[3])
if deadline <= now or expiry <= now then return 2 end
if expiry > deadline or deadline > now + tonumber(ARGV[4]) then return 3 end
if redis.call('EXISTS', KEYS[1]) == 1 then
  if redis.call('HGET', KEYS[1], 'subject') ~= ARGV[1]
    or redis.call('HGET', KEYS[1], 'deadline') ~= ARGV[2] then return 3 end
else
  redis.call('HSET', KEYS[1], 'subject', ARGV[1], 'deadline', ARGV[2], 'expiry', ARGV[3])
end
redis.call('HDEL', KEYS[1], 'data')
redis.call('PEXPIREAT', KEYS[1], deadline)
redis.call('ZREM', KEYS[2], KEYS[1])
return 1
`

// Test-only, server-owned attempt. Never accept these fields from a browser.
// Bytes and both deadlines are frozen, so retry cannot silently create a new
// credential, alter payload, or extend the original idle/absolute lifetime.
type saveAttempt struct {
	token  string
	data   []byte
	expiry time.Time
}

func newSaveAttempt(token string, data []byte, expiry time.Time) saveAttempt {
	return saveAttempt{token: token, data: bytes.Clone(data), expiry: expiry}
}

func (a saveAttempt) retry(ctx context.Context, store *guardedStore) error {
	return store.CommitCtx(ctx, a.token, a.data, a.expiry)
}

func (a saveAttempt) check(ctx context.Context, store *guardedStore) error {
	return store.withPayload(ctx, a.data, func(hash string, deadline time.Time, maxLifetime time.Duration) error {
		result, err := store.client.Eval(ctx, checkAdmissionLua,
			[]string{store.key(a.token), store.prefix + "subject:" + hash},
			hash, a.data, deadline.UnixMilli(), a.expiry.UnixMilli(), maxLifetime.Milliseconds()).Int()
		return admissionResult(result, err)
	})
}

func (a saveAttempt) abandon(ctx context.Context, store *guardedStore) error {
	return store.withPayload(ctx, a.data, func(hash string, deadline time.Time, maxLifetime time.Duration) error {
		result, err := store.client.Eval(ctx, abandonAdmissionLua,
			[]string{store.key(a.token), store.prefix + "subject:" + hash},
			hash, deadline.UnixMilli(), a.expiry.UnixMilli(), maxLifetime.Milliseconds()).Int()
		return admissionResult(result, err)
	})
}

func admissionResult(result int, err error) error {
	if err != nil {
		return err
	}
	switch result {
	case 0:
		return errOutcomeUnknown
	case 1:
		return nil
	case 2:
		return errStale
	default:
		return errAdmissionConflict
	}
}

// Only the first CommitCtx is faulted. The optional real Redis write finishes
// before its application acknowledgement is discarded. This does not simulate
// network packet loss or process death; all subsequent calls use the real store.
type uncertainStore struct {
	*guardedStore
	write   bool
	used    atomic.Bool
	pending chan saveAttempt
}

func (s *uncertainStore) CommitCtx(ctx context.Context, token string, b []byte, expiry time.Time) error {
	if !s.used.CompareAndSwap(false, true) {
		return s.guardedStore.CommitCtx(ctx, token, b, expiry)
	}
	attempt := newSaveAttempt(token, b, expiry)
	if s.write {
		if err := s.guardedStore.CommitCtx(ctx, token, b, expiry); err != nil {
			return err
		}
	}
	s.pending <- attempt
	return errOutcomeUnknown
}
