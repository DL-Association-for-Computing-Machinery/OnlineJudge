//go:build sandbox

package sandbox_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerclient"
)

// Two real TLS handlers share a disposable database. Only the execution adapter
// talks to the offline Linux container. Claim and materials use the node client;
// fixture submission and sandbox lifecycle remain test-driver responsibilities.
func TestExecutionReport(t *testing.T) {
	if os.Getenv("OJ_SANDBOX_REPORT") != "1" {
		t.Skip("run make report-test with the dedicated sandbox")
	}
	verifyEnvironment(t)
	dsn := os.Getenv("OJ_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("disposable test database required")
	}
	ctx := context.Background()
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var tables int
	if err := conn.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_schema='public'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("report database must be empty")
	}
	provider, err := db.Provider(conn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	version, manifest, _ := experiment.SelectFixture(experiment.PinnedVersion)
	store, err := postgres.New(pool, version, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterFixture(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES('report-node',1),('other-node',1)`); err != nil {
		t.Fatal(err)
	}
	ca := testpki.New(t)
	serverCert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	cert, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/report-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	otherCert, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/other-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	var loseReply atomic.Bool
	root := t.TempDir()
	if err := experiment.WriteMaterials(root); err != nil {
		t.Fatal(err)
	}
	reader, err := materials.OpenLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	newController := func() *httptest.Server {
		handler := workerapi.NewHandler(store, ca.Roots, materials.New(store, reader))
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/worker/report" && loseReply.CompareAndSwap(true, false) {
				recorded := httptest.NewRecorder()
				handler.ServeHTTP(recorded, r)
				if recorded.Code != 204 {
					t.Error("expected accepted report before dropping response")
				}
				conn, _, _ := w.(http.Hijacker).Hijack()
				conn.Close()
				return
			}
			handler.ServeHTTP(w, r)
		}))
		server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots}
		server.StartTLS()
		t.Cleanup(server.Close)
		return server
	}
	first, second := newController(), newController()
	acquirer, err := workerclient.NewAcquirer(first.URL, ca.Roots, cert)
	if err != nil {
		t.Fatal(err)
	}
	defer acquirer.Close()
	downloader, err := workerclient.NewAcquirer(second.URL, ca.Roots, cert)
	if err != nil {
		t.Fatal(err)
	}
	defer downloader.Close()
	renewer, err := workerclient.NewRenewer(first.URL, ca.Roots, cert)
	if err != nil {
		t.Fatal(err)
	}
	defer renewer.Close()
	reporter, err := workerclient.NewReporter(first.URL, ca.Roots, cert)
	if err != nil {
		t.Fatal(err)
	}
	defer reporter.Close()
	retry, err := workerclient.NewReporter(second.URL, ca.Roots, cert)
	if err != nil {
		t.Fatal(err)
	}
	defer retry.Close()
	wrong, err := workerclient.NewReporter(second.URL, ca.Roots, otherCert)
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	for index, tc := range []struct{ verdict, source string }{
		{"AC", "#include <cstdio>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);printf(\"%d\",a+b);}"},
		{"WA", "#include <cstdio>\nint main(){puts(\"wrong\");}"},
		{"CE", "this is not C++"},
		{"RE", "int main(){return 1;}"},
		{"TLE", "int main(){for(;;){asm volatile(\"\");}}"},
		{"MLE", "#include <cstdlib>\nint main(){volatile char*p=(char*)malloc(128*1024*1024);if(!p)return 2;for(long i=0;i<128*1024*1024;i+=4096)p[i]=1;for(;;){asm volatile(\"\");}}"},
		{"OLE", "#include <cstdio>\nint main(){for(;;)puts(\"unbounded output fixture\");}"},
		{"expired", "#include <cstdio>\nint main(){int a,b;scanf(\"%d%d\",&a,&b);printf(\"%d\",a+b);}"},
	} {
		t.Run(tc.verdict, func(t *testing.T) {
			row, _, err := store.Accept(ctx, "report-fixture", "m0-training", fmt.Sprintf("real-report-%08d", index), submission.Request{MaterialVersion: version, Language: "gnu++17", Source: tc.source})
			if err != nil {
				t.Fatal(err)
			}
			lease, err := acquirer.Claim(ctx)
			if err != nil || lease.SubmissionID != row.ID {
				t.Fatalf("claim: %v", err)
			}
			d, blobs, err := downloader.Fetch(ctx, lease)
			if err != nil {
				t.Fatal(err)
			}
			verifyEnvironment(t)
			sandbox := execution.NewGoJudge(&dockerTransport{}, func(context.Context, experiment.ExecutionProfile) error { return nil }) // verified above
			runner := execution.NewLeased(sandbox, renewer.Renew, func(context.Context) error { return errors.New("unexpected interruption requires fixture recovery") })
			frozen, err := runner.JudgeReport(ctx, lease, d, blobs)
			want := tc.verdict
			if want == "expired" {
				want = "AC"
			}
			if err != nil || frozen.Verdict != want {
				t.Fatalf("execution verdict=%s err=%v", frozen.Verdict, err)
			}
			if err := wrong.Report(ctx, lease.Token, frozen); err != submission.ErrLease {
				t.Fatalf("cross-node result: %v", err)
			}
			if tc.verdict == "expired" {
				if _, err := pool.Exec(ctx, `UPDATE judge_attempts SET lease_until=clock_timestamp()-interval '1 second' WHERE token=$1`, lease.Token); err != nil {
					t.Fatal(err)
				}
				if err := reporter.Report(ctx, lease.Token, frozen); err != submission.ErrLease {
					t.Fatalf("expired result: %v", err)
				}
				got, err := store.Get(ctx, "report-fixture", "m0-training", row.ID)
				if err != nil || got.Verdict != nil {
					t.Fatal("expired result became visible")
				}
				return
			}
			if tc.verdict == "AC" {
				if _, err := pool.Exec(ctx, `UPDATE worker_nodes SET enabled=false WHERE id='report-node'`); err != nil {
					t.Fatal(err)
				}
				if err := reporter.Report(ctx, lease.Token, frozen); err != submission.ErrNode {
					t.Fatalf("revoked result: %v", err)
				}
				if _, err := pool.Exec(ctx, `UPDATE worker_nodes SET enabled=true WHERE id='report-node'`); err != nil {
					t.Fatal(err)
				}
			}
			loseReply.Store(true)
			if err := reporter.Report(ctx, lease.Token, frozen); err != workerclient.ErrReportUnknown {
				t.Fatalf("lost acknowledgement: %v", err)
			}
			if err := retry.Report(ctx, lease.Token, frozen); err != nil {
				t.Fatalf("identical retry: %v", err)
			}
			changed := frozen
			changed.EvidenceSHA256 = hashBytes([]byte("different evidence"))
			if err := retry.Report(ctx, lease.Token, changed); err != submission.ErrResultConflict {
				t.Fatalf("conflicting retry: %v", err)
			}
			got, err := store.Get(ctx, "report-fixture", "m0-training", row.ID)
			if err != nil || got.State != "finished" || got.Verdict == nil || *got.Verdict != tc.verdict {
				t.Fatalf("durable result: %+v %v", got, err)
			}
			var stored submission.Result
			var payload []byte
			if err := pool.QueryRow(ctx, `SELECT result_payload FROM judge_attempts WHERE token=$1`, lease.Token).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(payload, &stored); err != nil || stored != frozen {
				t.Fatal("persisted evidence changed")
			}
			t.Logf("real %s persisted; lost reply retried across TLS controllers; conflict and wrong node rejected", tc.verdict)
		})
	}
}
