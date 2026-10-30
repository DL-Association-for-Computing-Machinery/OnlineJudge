package sessionexperiment

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Test-only standalone Redis observation. One read-only command avoids keys
// expiring between enumeration, binary payload reads and absolute TTL reads.
// Nested RESP retains Gob bytes without a JSON/text encoding round trip.
const snapshotLua = `#!lua flags=no-writes
local t = redis.call('TIME')
local now = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
local snapshot = {}
for _, key in ipairs(redis.call('KEYS', ARGV[1] .. '*')) do
  local expiry = redis.call('PEXPIRETIME', key)
  if expiry <= 0 then return redis.error_reply('session snapshot requires TTL') end
  if expiry > now then
    local kind = redis.call('TYPE', key).ok
    local values
    if string.find(key, ':token:', 1, true) and kind == 'hash' then
      values = redis.call('HGETALL', key)
    elseif not string.find(key, ':token:', 1, true) and kind == 'zset' then
      values = redis.call('ZRANGE', key, 0, -1, 'WITHSCORES')
    else
      return redis.error_reply('unexpected session snapshot type')
    end
    table.insert(snapshot, {key, expiry, kind, values})
  end
end
return snapshot
`

func readMemorySnapshot(ctx context.Context, client *redis.Client, prefix string) (map[string]memoryRecord, error) {
	rows, err := client.EvalRO(ctx, snapshotLua, nil, prefix).Slice()
	if err != nil {
		return nil, err
	}
	state := make(map[string]memoryRecord, len(rows))
	for _, item := range rows {
		row, ok := item.([]interface{})
		if !ok || len(row) != 4 {
			return nil, errors.New("invalid snapshot row")
		}
		key, keyOK := row[0].(string)
		expiry, expiryOK := row[1].(int64)
		kind, kindOK := row[2].(string)
		values, valuesOK := row[3].([]interface{})
		if !keyOK || !expiryOK || expiry <= 0 || !kindOK || !valuesOK || len(values)%2 != 0 {
			return nil, errors.New("invalid snapshot fields or absolute expiry")
		}
		entry := memoryRecord{expires: time.Duration(expiry) * time.Millisecond}
		if kind == "hash" {
			entry.fields = make(map[string]string, len(values)/2)
		} else if kind != "zset" {
			return nil, errors.New("invalid snapshot type")
		}
		for i := 0; i < len(values); i += 2 {
			name, nameOK := values[i].(string)
			value, valueOK := values[i+1].(string)
			if !nameOK || !valueOK {
				return nil, errors.New("invalid snapshot member")
			}
			if kind == "hash" {
				entry.fields[name] = value
			} else {
				score, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid snapshot score: %w", err)
				}
				entry.slots = append(entry.slots, redis.Z{Score: score, Member: name})
			}
		}
		state[key] = entry
	}
	return state, nil
}

func TestMemorySnapshotAtNaturalExpiry(t *testing.T) {
	f := setup(t)
	client := f.stores[0].client
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	key, slot := f.stores[0].key("snapshot-fixture"), f.stores[0].prefix+"subject:fixture"
	deadline := redisTime(t, f).Add(time.Second).UnixMilli()
	payload := string([]byte{0, 255, '\n', '\r', 1})
	if _, err := client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, key, "data", payload)
		pipe.PExpireAt(ctx, key, time.UnixMilli(deadline))
		pipe.ZAdd(ctx, slot, redis.Z{Score: float64(deadline), Member: key})
		pipe.PExpireAt(ctx, slot, time.UnixMilli(deadline))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	live := false
	for {
		state, err := readMemorySnapshot(ctx, client, f.stores[0].prefix)
		if err != nil {
			t.Fatal(err)
		}
		if len(state) == 0 {
			if !live {
				t.Fatal("did not observe live snapshot before natural expiry")
			}
			break
		}
		if len(state) != 2 || state[key].fields["data"] != payload || state[key].expires.Milliseconds() != deadline ||
			state[slot].expires.Milliseconds() != deadline || len(state[slot].slots) != 1 || state[slot].slots[0].Score != float64(deadline) || state[slot].slots[0].Member != key {
			t.Fatal("snapshot mixed expiry states or changed binary payload/index")
		}
		live = true
		time.Sleep(time.Millisecond)
	}
}

func TestMemorySnapshotRejectsInvalidRecords(t *testing.T) {
	for _, fault := range []string{"missing-ttl", "wrong-type"} {
		t.Run(fault, func(t *testing.T) {
			f := setup(t)
			client := f.stores[0].client
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			key := f.stores[0].key("invalid-snapshot-fixture")
			var err error
			if fault == "missing-ttl" {
				err = client.HSet(ctx, key, "data", "fixture").Err()
			} else {
				err = client.Set(ctx, key, "fixture", time.Hour).Err()
			}
			if err != nil {
				t.Fatal(err)
			}
			expires, err := client.PExpireTime(ctx, key).Result()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readMemorySnapshot(ctx, client, f.stores[0].prefix); err == nil {
				t.Fatal("invalid record was silently omitted from snapshot")
			}
			if after, err := client.PExpireTime(ctx, key).Result(); err != nil || after != expires {
				t.Fatal("rejected snapshot modified record expiry")
			}
			if fault == "missing-ttl" {
				if data, err := client.HGet(ctx, key, "data").Result(); err != nil || data != "fixture" {
					t.Fatal("rejected snapshot changed hash payload")
				}
			} else if data, err := client.Get(ctx, key).Result(); err != nil || data != "fixture" {
				t.Fatal("rejected snapshot changed string payload")
			}
		})
	}
}
