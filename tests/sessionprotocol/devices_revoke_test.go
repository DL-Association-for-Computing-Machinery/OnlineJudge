//go:build integration

package sessionprotocol_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testRevokeDevices(t *testing.T, f *fixture) {
	for _, change := range []string{"touch", "membership", "invariant", "lost reply"} {
		t.Run(change, func(t *testing.T) {
			ns := namespace(t)
			p := f.subject(t, false)
			other := f.store(t, ns, 1, protocol.OrdinaryPolicy())
			_, current := f.create(t, other, p)
			targetAttempt, target := f.create(t, other, p)
			_, sibling := f.create(t, other, p)
			targetKey := tokenKey(ns, target)
			start, err := other.Read(f.ctx, target)
			must(t, err)
			if change == "touch" {
				f.waitTime(t, start.CreatedAt.Add(10*time.Millisecond))
			}
			h := &hook{op: "revoke-devices"}
			h.before = func(ctx context.Context, _ redis.Cmder) error {
				switch change {
				case "touch":
					view, err := other.Touch(ctx, target)
					if err == nil && !view.IdleExpiresAt.After(start.IdleExpiresAt) {
						return errors.New("fixture touch did not extend idle expiry")
					}
					return err
				case "membership":
					return other.Revoke(ctx, sibling)
				case "invariant":
					return f.control.HSet(ctx, targetKey, "credential", "3").Err()
				}
				return nil
			}
			if change == "lost reply" {
				h.after = func(context.Context, redis.Cmder) error { return errors.New("synthetic lost response") }
			}
			store := f.faultStore(t, ns, h, time.Second)
			before, err := f.control.Do(f.ctx, "PEXPIRETIME", targetKey).Int64()
			must(t, err)
			err = store.RevokeOtherDevices(f.ctx, current)
			_, readErr := other.Read(f.ctx, target)
			switch change {
			case "membership", "invariant":
				wantError(t, err, protocol.ErrConflict)
				if change == "invariant" {
					wantError(t, readErr, protocol.ErrConflict)
					state, err := f.control.HGet(f.ctx, targetKey, "state").Result()
					must(t, err)
					if state != "live" {
						t.Fatal("a failed operation partially revoked a device")
					}
					_, err = other.Read(f.ctx, sibling)
					must(t, err)
				} else {
					must(t, readErr)
				}
			default:
				if change == "lost reply" {
					wantError(t, err, protocol.ErrStore)
				} else {
					must(t, err)
				}
				wantError(t, readErr, protocol.ErrStale)
				_, err = other.Touch(f.ctx, target)
				wantError(t, err, protocol.ErrStale)
				wantError(t, other.Retry(f.ctx, targetAttempt), protocol.ErrStale)
				after, err := f.control.Do(f.ctx, "PEXPIRETIME", targetKey).Int64()
				must(t, err)
				if after != before {
					t.Fatal("device tombstone did not preserve absolute expiry")
				}
			}
			_, err = other.Read(f.ctx, current)
			must(t, err)
		})
	}
}
