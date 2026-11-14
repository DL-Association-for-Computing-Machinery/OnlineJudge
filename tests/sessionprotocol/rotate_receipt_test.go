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

func testRotationLostReply(t *testing.T, f *fixture) {
	ns := namespace(t)
	p := f.subject(t, false)
	peer := f.store(t, ns, 1, protocol.OrdinaryPolicy())
	attempt, old := f.create(t, peer, p)
	var successorKey string
	store := f.faultStore(t, ns, &hook{op: "rotate", after: func(_ context.Context, cmd redis.Cmder) error {
		successorKey, _ = cmd.Args()[5].(string)
		return errors.New("synthetic rotation response loss")
	}}, time.Second)
	result, err := store.Rotate(f.ctx, old)
	wantError(t, err, protocol.ErrStore)
	if result != (protocol.LoginGrant{}) || successorKey == "" {
		t.Fatal("missing injected lost reply or unexpected cookie grant")
	}
	state, err := f.control.HGet(f.ctx, successorKey, "state").Result()
	must(t, err)
	if state != "live" {
		t.Fatal("lost reply fixture did not commit the successor")
	}
	_, err = peer.Rotate(f.ctx, old)
	wantError(t, err, protocol.ErrStale)
	_, err = peer.Touch(f.ctx, old)
	wantError(t, err, protocol.ErrStale)
	wantError(t, peer.Retry(f.ctx, attempt), protocol.ErrStale)
	if len(namespaceRecords(t, f, ns)) != 2 {
		t.Fatal("old token created more than one successor")
	}
}
