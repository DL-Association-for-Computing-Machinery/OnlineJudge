//go:build integration

package sessionprotocol_test

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type loginAuthorityProbe struct {
	protocol.Authority
	entries atomic.Int64
}

func (p *loginAuthorityProbe) RecoveryState(ctx context.Context) (identity.RecoveryState, error) {
	p.entries.Add(1)
	return p.Authority.RecoveryState(ctx)
}

func rateLimits() protocol.LoginLimits {
	b := protocol.LoginBucket{Count: 10, Window: time.Hour}
	return protocol.LoginLimits{BeginGlobal: b, BeginSource: b, BeginEmail: b, RecoveryGlobal: b, RecoverySource: b}
}

func limitedManager(t *testing.T, f *fixture, s *protocol.Store, secrets *protocol.LoginSecrets, limits protocol.LoginLimits, initialize bool) *protocol.Login {
	t.Helper()
	if initialize {
		must(t, protocol.InitializeLoginLimits(f.ctx, s, secrets, limits))
	}
	l, err := protocol.NewLimitedLogin(f.ctx, s, loginOrigin, time.Minute, secrets, limits)
	must(t, err)
	return l
}

type rateRecord struct {
	dump     string
	deadline time.Duration
}

func rateSnapshot(t *testing.T, f *fixture, ns string) map[string]rateRecord {
	return admissionSnapshot(t, f, "oj-login-limit:"+ns+":*")
}

func admissionSnapshot(t *testing.T, f *fixture, pattern string) map[string]rateRecord {
	t.Helper()
	keys, err := f.control.Keys(f.ctx, pattern).Result()
	must(t, err)
	result := make(map[string]rateRecord, len(keys))
	for _, k := range keys {
		dump, err := f.control.Dump(f.ctx, k).Result()
		must(t, err)
		deadline, err := f.control.PExpireTime(f.ctx, k).Result()
		must(t, err)
		result[k] = rateRecord{dump, deadline}
	}
	return result
}

func unchangedRate(t *testing.T, f *fixture, ns string, before map[string]rateRecord) {
	t.Helper()
	if !reflect.DeepEqual(before, rateSnapshot(t, f, ns)) {
		t.Fatal("denial changed a key, counter or absolute TTL")
	}
}

func rateCounterKeys(t *testing.T, f *fixture, ns string) []string {
	t.Helper()
	keys, err := f.control.Keys(f.ctx, "oj-login-limit:"+ns+":*").Result()
	must(t, err)
	var result []string
	for _, k := range keys {
		if !strings.HasSuffix(k, ":config") {
			result = append(result, k)
		}
	}
	return result
}
