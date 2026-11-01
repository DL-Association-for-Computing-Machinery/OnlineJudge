//go:build integration

package sessionprotocol_test

import (
	"net/http"
	"testing"
	"time"

	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testLoginFences(t *testing.T, f *fixture) {
	for _, change := range []string{"rehash", "replace", "disable", "recovery"} {
		t.Run(change, func(t *testing.T) {
			ns := namespace(t)
			p := f.subject(t, false)
			s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
			x := loginServer(t, loginManager(t, s, time.Minute, loginKey(t)))
			pending := beginLogin(t, x, p)
			switch change {
			case "rehash":
				record, err := f.hasher.Create([]byte("synthetic protocol password"))
				must(t, err)
				_, err = f.identities[1].RehashCAS(f.ctx, p.ID, p.Revision, record)
				must(t, err)
			case "replace":
				record, err := f.hasher.Create([]byte("synthetic replacement password"))
				must(t, err)
				_, err = f.identities[1].ReplacePasswordCAS(f.ctx, p.ID, p.Revision, record)
				must(t, err)
			case "disable":
				_, err := f.identities[1].DisableCAS(f.ctx, p.ID, p.Revision)
				must(t, err)
			case "recovery":
				generation, err := f.identities[1].BeginRecovery(f.ctx)
				must(t, err)
				loginStatus(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 503)
				must(t, f.identities[1].FinishRecovery(f.ctx, generation))
			}
			loginStatus(t, loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{pending}, nil), 401)
			if len(namespaceRecords(t, f, ns)) != 0 {
				t.Fatal("old credential proof was relabeled and admitted")
			}
			if change == "rehash" {
				loginStatus(t, loginCall(x, "/api/v1/session/login/abandon", "", []*http.Cookie{pending}, nil), 204)
				keys := namespaceRecords(t, f, ns)
				if len(keys) != 1 {
					t.Fatal("rehash prevented candidate cleanup")
				}
				state, err := f.control.HGet(f.ctx, keys[0], "state").Result()
				must(t, err)
				if state != "dead" {
					t.Fatal("rehash cleanup did not tombstone")
				}
			}
		})
	}
}
