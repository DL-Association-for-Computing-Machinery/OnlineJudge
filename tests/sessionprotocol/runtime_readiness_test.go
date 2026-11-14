//go:build integration

package sessionprotocol_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
)

func testRuntimeReadiness(t *testing.T, f *fixture) {
	c := runtimeConfig(t, f, namespace(t), true)
	runtime := openRuntime(t, c)
	server := loginHandlerServer(t, httpapi.NewSessionHandler(runtime))
	read := func(path string) loginResponse {
		return loginCall(server, path, "", nil, func(r *http.Request) { r.Method = http.MethodGet })
	}
	wait := func(want int) {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for {
			got := read("/api/v1/readyz")
			if got.err == nil && got.status == want {
				if got.headers.Get("Cache-Control") != "no-store" || len(got.cookies) != 0 {
					t.Fatal("readiness response changed cache or session state")
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("readiness did not become %d; status=%d err=%v", want, got.status, got.err)
			}
			time.Sleep(150 * time.Millisecond)
		}
	}
	wait(200)
	_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_session_recovery SET enabled=false WHERE singleton`)
	must(t, err)
	defer func() {
		_, err := f.pools[0].Exec(f.ctx, `UPDATE identity_session_recovery SET enabled=true WHERE singleton`)
		must(t, err)
	}()
	wait(503)
	_, err = f.pools[0].Exec(f.ctx, `UPDATE identity_session_recovery SET enabled=true WHERE singleton`)
	must(t, err)
	wait(200)
	_, err = f.pools[0].Exec(f.ctx, `INSERT INTO goose_db_version (version_id,is_applied) VALUES (999,true)`)
	must(t, err)
	defer func() {
		_, err := f.pools[0].Exec(f.ctx, `DELETE FROM goose_db_version WHERE version_id=999`)
		must(t, err)
	}()
	wait(503)
	loginStatus(t, read("/api/v1/healthz"), 200)
	_, err = f.pools[0].Exec(f.ctx, `DELETE FROM goose_db_version WHERE version_id=999`)
	must(t, err)
	wait(200)
	must(t, f.control.Do(f.ctx, "CLIENT", "PAUSE", 3000, "ALL").Err())
	wait(503)
	loginStatus(t, read("/api/v1/healthz"), 200)
	wait(200)
	must(t, runtime.Close(t.Context()))
	loginStatus(t, read("/api/v1/readyz"), 503)
	loginStatus(t, read("/api/v1/healthz"), 200)
}
