//go:build integration

package integration_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Two real controller processes and two TLS clients; reports remain synthetic.
func testWorkerAPI(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *postgres.Store) {
	ca := testpki.New(t)
	_, certFile, keyFile := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	binary := filepath.Join(t.TempDir(), "worker-api")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/worker-api")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build worker controller: %v\n%s", err, output)
	}
	nodes := []string{"tls-node-a", "tls-node-b"}
	clients := make([]*http.Client, 2)
	for i, id := range nodes {
		_, err := pool.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,1)`, id)
		must(t, err)
		pair, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/"+id+"/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
		clients[i] = workerClient(t, ca.Roots, pair)
	}
	materialRoot := t.TempDir()
	must(t, experiment.WriteMaterials(materialRoot))
	urls := []string{startWorkerController(t, binary, ca.CAFile, certFile, keyFile, clients[0], materialRoot), startWorkerController(t, binary, ca.CAFile, certFile, keyFile, clients[1], materialRoot)}
	seq := 0
	accept := func() submission.Receipt {
		seq++
		row, _, err := store.Accept(ctx, "tls-fixture", "m0-training", fmt.Sprintf("tls-fixture-%08d", seq), submission.Request{MaterialVersion: experiment.Version, Language: "gnu++17", Source: "// synthetic protocol fixture; 原样源码😀; never executed\n"})
		must(t, err)
		return row
	}
	call := func(client *http.Client, base, action, body string, want int) []byte {
		t.Helper()
		status, data, _, err := workerRequest(client, base, action, body)
		must(t, err)
		if status != want {
			t.Fatalf("%s status=%d want=%d body=%s", action, status, want, data)
		}
		return data
	}
	type lease struct {
		Token string `json:"lease_token"`
		ID    string `json:"submission_id"`
	}
	claim := func(client *http.Client, base string) lease {
		var l lease
		must(t, json.Unmarshal(call(client, base, "claim", "{}", 200), &l))
		if len(l.Token) != 64 || l.ID == "" {
			t.Fatal("invalid lease response")
		}
		return l
	}
	renew := func(l lease) string { return `{"lease_token":"` + l.Token + `"}` }
	report := func(l lease, verdict string) string {
		return `{"lease_token":"` + l.Token + `","verdict":"` + verdict + `","evidence_sha256":"` + strings.Repeat("a", 64) + `"}`
	}

	t.Run("two controllers and nodes compete", func(t *testing.T) {
		expected := accept()
		type response struct {
			status int
			data   []byte
			err    error
			index  int
		}
		responses := make(chan response, 2)
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s, b, _, e := workerRequest(clients[i], urls[i], "claim", "{}")
				responses <- response{s, b, e, i}
			}()
		}
		wg.Wait()
		close(responses)
		winner := -1
		var l lease
		for r := range responses {
			must(t, r.err)
			if r.status == 200 {
				if winner != -1 {
					t.Fatal("duplicate claim")
				}
				winner = r.index
				must(t, json.Unmarshal(r.data, &l))
			} else if r.status != 204 {
				t.Fatalf("claim status=%d", r.status)
			}
		}
		if winner < 0 || l.ID != expected.ID {
			t.Fatal("task not uniquely claimed")
		}
		call(clients[winner], urls[1-winner], "claim", "{}", 204)
		call(clients[1-winner], urls[winner], "renew", renew(l), 409)
		call(clients[1-winner], urls[winner], "report", report(l, "AC"), 409)
		call(clients[winner], urls[1-winner], "renew", renew(l), 200)
		testWorkerMaterials(t, clients[winner], clients[1-winner], urls, materialRoot, l.ID, l.Token)
		call(clients[winner], urls[0], "report", report(l, "AC"), 204)
		call(clients[winner], urls[1], "assignment", renew(l), 409)
		call(clients[winner], urls[0], "material", materialBody(l.Token), 409)
		call(clients[winner], urls[1], "report", report(l, "AC"), 204)
		call(clients[winner], urls[1], "report", report(l, "WA"), 409)
		row, err := store.Get(ctx, "tls-fixture", "m0-training", expected.ID)
		must(t, err)
		if row.State != "finished" || row.Verdict == nil || *row.Verdict != "AC" {
			t.Fatal("synthetic report not durable")
		}
	})
	t.Run("revocation and rotation on existing connections", func(t *testing.T) {
		accept()
		old := claim(clients[0], urls[0])
		call(clients[0], urls[0], "renew", renew(old), 200)
		_, err := pool.Exec(ctx, `UPDATE worker_nodes SET enabled=false WHERE id=$1`, nodes[0])
		must(t, err)
		status, _, reused, err := workerRequest(clients[0], urls[0], "renew", renew(old))
		must(t, err)
		if status != 403 || !reused {
			t.Fatalf("revocation status=%d connection reused=%v", status, reused)
		}
		call(clients[0], urls[0], "claim", "{}", 403)
		call(clients[0], urls[0], "report", report(old, "AC"), 403)
		call(clients[0], urls[0], "assignment", renew(old), 403)
		call(clients[0], urls[1], "material", materialBody(old.Token), 403)
		_, err = pool.Exec(ctx, `UPDATE worker_nodes SET enabled=true,credential_version=2 WHERE id=$1`, nodes[0])
		must(t, err)
		call(clients[0], urls[1], "claim", "{}", 403)
		pair, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/"+nodes[0]+"/2", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
		fresh := workerClient(t, ca.Roots, pair)
		call(fresh, urls[0], "renew", renew(old), 409)
		call(fresh, urls[0], "report", report(old, "AC"), 409)
		call(fresh, urls[0], "assignment", renew(old), 409)
		call(fresh, urls[1], "material", materialBody(old.Token), 409)
		_, err = store.Recover(ctx)
		must(t, err)
		current := claim(fresh, urls[1])
		if current.ID != old.ID || current.Token == old.Token {
			t.Fatal("revoked task recovery failed")
		}
		call(fresh, urls[0], "report", report(current, "AC"), 204)
	})
	t.Run("expiry and stale attempt before and after recovery", func(t *testing.T) {
		accept()
		old := claim(clients[1], urls[0])
		_, err := pool.Exec(ctx, `UPDATE judge_attempts SET lease_until=clock_timestamp()-interval '1 second' WHERE token=$1`, old.Token)
		must(t, err)
		call(clients[1], urls[1], "renew", renew(old), 409)
		call(clients[1], urls[1], "report", report(old, "AC"), 409)
		call(clients[1], urls[1], "assignment", renew(old), 409)
		call(clients[1], urls[0], "material", materialBody(old.Token), 409)
		_, err = store.Recover(ctx)
		must(t, err)
		current := claim(clients[1], urls[1])
		call(clients[1], urls[0], "assignment", renew(old), 409)
		call(clients[1], urls[0], "material", materialBody(old.Token), 409)
		call(clients[1], urls[0], "report", report(old, "AC"), 409)
		call(clients[1], urls[0], "report", report(current, "AC"), 204)
	})
	t.Run("unregistered certificate identity denied", func(t *testing.T) {
		pair, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/unregistered/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
		call(workerClient(t, ca.Roots, pair), urls[0], "claim", "{}", 403)
	})
	t.Run("material lease locks and generations", func(t *testing.T) {
		testMaterialLocks(t, ctx, pool, store, submission.NodeIdentity{ID: nodes[1], CredentialVersion: 1})
	})
	t.Run("unconfigured materials fail closed", func(t *testing.T) {
		base := startWorkerController(t, binary, ca.CAFile, certFile, keyFile, clients[1])
		call(clients[1], base, "assignment", `{"lease_token":"`+strings.Repeat("a", 64)+`"}`, 503)
	})
}

func workerClient(t *testing.T, roots *x509.CertPool, cert tls.Certificate) *http.Client {
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{cert}}}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}

func workerRequest(client *http.Client, base, action, body string) (int, []byte, bool, error) {
	req, err := http.NewRequest("POST", base+"/api/v1/worker/"+action, strings.NewReader(body))
	if err != nil {
		return 0, nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	reused := false
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}))
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, reused, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if resp.Header.Get("Cache-Control") != "no-store" {
		return 0, nil, reused, fmt.Errorf("cacheable worker response")
	}
	return resp.StatusCode, data, reused, err
}

func startWorkerController(t *testing.T, binary, ca, cert, key string, client *http.Client, materialRoot ...string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	addr := listener.Addr().String()
	must(t, listener.Close())
	cmd := exec.Command(binary)
	// Do not inherit credentials or configuration from the invoking user's environment.
	cmd.Env = []string{"OJ_WORKER_API_ADDR=" + addr, "OJ_WORKER_CA_FILE=" + ca, "OJ_WORKER_CERT_FILE=" + cert, "OJ_WORKER_KEY_FILE=" + key, "OJ_DATABASE_URL=" + os.Getenv("OJ_TEST_DATABASE_URL")}
	if len(materialRoot) > 0 {
		cmd.Env = append(cmd.Env, "OJ_MATERIAL_ROOT="+materialRoot[0])
	}
	logFile, err := os.Create(filepath.Join(t.TempDir(), "controller.log"))
	must(t, err)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	must(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("controller exit: %v", err)
			}
		case <-time.After(12 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Error("controller failed graceful shutdown")
		}
		logFile.Close()
	})
	base := "https://" + addr
	for range 50 {
		// Unknown route verifies listener/TLS readiness without claiming work.
		status, _, _, err := workerRequest(client, base, "not-a-route", "{}")
		if err == nil && status == 404 {
			return base
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("worker controller did not become ready")
	return ""
}
