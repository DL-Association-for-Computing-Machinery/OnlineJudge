package sessionexperiment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/redis/go-redis/v9"
)

var (
	errLimit = errors.New("session limit")
	errStale = errors.New("session expired or revoked")
)

// This adapter exists only in the experiment, with optional fixture authority.
// Retain expired/revoked records until their absolute deadline: deleting them
// early would let a request that loaded old data recreate the same token.
// Redis 7 declared-script defaults reject an already-OOM write before running
// ZREMRANGEBYSCORE; legacy eval lets this first non-allocating write bypass OOM
// checks for the later HSET/ZADD. This is not a hard cap during script execution.
const commitLua = `#!lua
local t = redis.call('TIME')
local now = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
local deadline, expiry = tonumber(ARGV[3]), tonumber(ARGV[4])
if expiry <= now or deadline <= now or expiry > deadline then return 2 end
if deadline > now + 43200000 then return 2 end
if redis.call('EXISTS', KEYS[1]) == 1 then
  if redis.call('HGET', KEYS[1], 'subject') ~= ARGV[1]
    or redis.call('HGET', KEYS[1], 'deadline') ~= ARGV[3]
    or redis.call('HEXISTS', KEYS[1], 'data') == 0
    or tonumber(redis.call('HGET', KEYS[1], 'expiry')) <= now then return 2 end
else
  redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now)
  if redis.call('ZCARD', KEYS[2]) >= 3 then return 1 end
end
redis.call('HSET', KEYS[1], 'subject', ARGV[1], 'data', ARGV[2],
  'deadline', ARGV[3], 'expiry', ARGV[4])
redis.call('PEXPIREAT', KEYS[1], deadline)
redis.call('ZADD', KEYS[2], expiry, KEYS[1])
local latest = redis.call('ZREVRANGE', KEYS[2], 0, 0, 'WITHSCORES')
redis.call('PEXPIREAT', KEYS[2], latest[2])
return 0
`

const findLua = `
local t = redis.call('TIME')
local now = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
local expiry = redis.call('HGET', KEYS[1], 'expiry')
if not expiry or tonumber(expiry) <= now then return false end
return redis.call('HGET', KEYS[1], 'data')
`

const deleteLua = `
local subject = redis.call('HGET', KEYS[1], 'subject')
if subject then
  redis.call('HDEL', KEYS[1], 'data')
  redis.call('ZREM', ARGV[1] .. subject, KEYS[1])
end
return 0
`

type guardedStore struct {
	client *redis.Client
	prefix string
	auth   *authAuthority
}

var _ scs.CtxStore = (*guardedStore)(nil)

func digest(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

func (s *guardedStore) key(token string) string { return s.prefix + "token:" + digest(token) }

func (s *guardedStore) CommitCtx(ctx context.Context, token string, b []byte, expiry time.Time) error {
	deadline, values, err := (scs.GobCodec{}).Decode(b)
	if err != nil {
		return err
	}
	subject, ok := values["subject"].(string)
	if !ok || subject == "" {
		return errors.New("missing authenticated fixture subject")
	}
	hash := digest(subject)
	if s.auth != nil {
		generation, ok := values["auth_generation"].(int64)
		if !ok || generation <= 0 {
			return errStale
		}
		recovery, ok := values["recovery_generation"].(int64)
		if !ok || recovery <= 0 {
			return errStale
		}
		// A new generation gets new slots; old records remain until expiry.
		hash = digest(subject + ":" + strconv.FormatInt(generation, 10) + ":" + strconv.FormatInt(recovery, 10))
		return s.auth.withValid(ctx, subject, authVersion{generation, recovery}, func() error {
			return s.commit(ctx, token, b, hash, deadline, expiry)
		})
	}
	return s.commit(ctx, token, b, hash, deadline, expiry)
}

func (s *guardedStore) commit(ctx context.Context, token string, b []byte, hash string, deadline, expiry time.Time) error {
	result, err := s.client.Eval(ctx, commitLua, []string{s.key(token), s.prefix + "subject:" + hash},
		hash, b, deadline.UnixMilli(), expiry.UnixMilli()).Int()
	if err != nil {
		return err
	}
	switch result {
	case 0:
		return nil
	case 1:
		return errLimit
	default:
		return errStale
	}
}

func (s *guardedStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	b, err := s.client.Eval(ctx, findLua, []string{s.key(token)}).Text()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err == nil && s.auth != nil {
		_, values, decodeErr := (scs.GobCodec{}).Decode([]byte(b))
		if decodeErr != nil {
			return nil, false, decodeErr
		}
		subject, _ := values["subject"].(string)
		generation, _ := values["auth_generation"].(int64)
		recovery, _ := values["recovery_generation"].(int64)
		err = s.auth.withValid(ctx, subject, authVersion{generation, recovery}, func() error { return nil })
	}
	return []byte(b), err == nil, err
}

func (s *guardedStore) DeleteCtx(ctx context.Context, token string) error {
	return s.client.Eval(ctx, deleteLua, []string{s.key(token)}, s.prefix+"subject:").Err()
}

func (s *guardedStore) Commit(token string, b []byte, expiry time.Time) error {
	return s.CommitCtx(context.Background(), token, b, expiry)
}
func (s *guardedStore) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}
func (s *guardedStore) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}
