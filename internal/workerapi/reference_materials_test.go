//go:build linux

package workerapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	problemdef "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

type shortReferenceGrant struct {
	plan     problemreference.ExpectedPlan
	deadline time.Time
}

func (s *shortReferenceGrant) AuthorizeReferenceMaterials(context.Context, submission.NodeIdentity, problemreference.MaterialInput) (problemreference.MaterialAuthorization, error) {
	return problemreference.MaterialAuthorization{Plan: s.plan, Deadline: s.deadline}, nil
}
func (s *shortReferenceGrant) ClaimReference(context.Context, submission.NodeIdentity, problemreference.ClaimInput) (problemreference.WorkerClaim, error) {
	return problemreference.WorkerClaim{}, problemreference.ErrClaimUnavailable
}

func TestReferenceMaterialWriteDeadlineStopsSlowPeer(t *testing.T) {
	// A small TCP send buffer forces the real TLS writer to block once the
	// client deliberately stops reading. No sandbox or source program runs.
	raw := bytes.Repeat([]byte{'x'}, 4<<20)
	object := materialstore.Object{SHA256: problemfixture.Hash(raw), Size: int64(len(raw))}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 4 << 20, MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := store.Put(t.Context(), object, bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	plan := problemdef.ReferencePlan{SourceID: "library-checker:unionfind", Version: digest, ImportRevision: digest, SourceLockSHA256: digest, Source: materialstore.Object{SHA256: digest, Size: 1}, Definition: materialstore.Object{SHA256: digest, Size: 1}, Hashes: materialstore.Object{SHA256: digest, Size: 1}, Cases: []problemdef.ReferenceCase{{ID: "case-0001", Input: object, Answer: materialstore.Object{SHA256: digest, Size: 1}}}}
	encoding, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	authority := &shortReferenceGrant{plan: problemreference.ExpectedPlan{Encoding: encoding, SHA256: plan.Digest()}, deadline: time.Now().Add(time.Second)}
	service, err := problemreference.NewReferenceMaterials(authority, store)
	if err != nil {
		t.Fatal(err)
	}
	ca := testpki.New(t)
	cert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Minute))
	done := make(chan struct{})
	handler := WithReferenceClaims(http.NotFoundHandler(), authority, ca.Roots, service)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { defer close(done); handler.ServeHTTP(w, r) }))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots, Certificates: []tls.Certificate{cert}}
	server.Config.ConnState = func(c net.Conn, state http.ConnState) {
		if state == http.StateNew {
			if tcp, ok := c.(*tls.Conn).NetConn().(*net.TCPConn); ok {
				_ = tcp.SetWriteBuffer(1024)
			}
		}
	}
	server.StartTLS()
	defer server.Close()
	worker, _, _ := ca.Issue(t, "spiffe://onlinejudge/reference-worker/test-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: ca.Roots, Certificates: []tls.Certificate{worker}}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	input := struct {
		problemreference.MaterialInput
		Object materialstore.Object `json:"object"`
	}{problemreference.MaterialInput{ClaimInput: problemreference.ClaimInput{RequestID: strings.Repeat("1", 32), SourceID: plan.SourceID, PlanSHA256: plan.Digest()}, Token: digest}, object}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Post(server.URL+referenceMaterialPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 200 {
		t.Fatal("material response rejected before streaming", response.StatusCode)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("slow response write outlived reservation")
	}
	if time.Now().Before(authority.deadline) {
		t.Fatal("response did not actually block through its deadline")
	}
	probe, err := store.Open(t.Context(), object)
	if err != nil {
		t.Fatal("streaming failure retained object quota", err)
	}
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
}
