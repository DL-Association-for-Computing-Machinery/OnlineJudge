//go:build integration && linux

package integration_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerapi"
)

func claimedMaterialFixture(t *testing.T) (expectedPlanFixture, problemreference.MaterialAuthorizer, submission.NodeIdentity, problemreference.MaterialInput) {
	t.Helper()
	f, a, node, input := claimFixture(t)
	prepareClaim(t, f, expectedPlanKey)
	claim, err := a.ClaimReference(f.ctx, node, input)
	must(t, err)
	return f, a, node, problemreference.MaterialInput{ClaimInput: input, Token: claim.Token}
}

func TestReferenceMaterialsReauthorizeEveryRead(t *testing.T) {
	for _, kind := range []string{"token", "request", "source", "digest", "node", "version", "expired", "superseded", "cancel", "regrant", "disabled author", "recovery", "disabled node", "rotated node", "profile", "plan"} {
		t.Run(kind, func(t *testing.T) {
			f, a, node, input := claimedMaterialFixture(t)
			switch kind {
			case "token":
				input.Token = strings.Repeat("a", 64)
			case "request":
				input.RequestID = strings.Repeat("a", 32)
			case "source":
				input.SourceID = "library-checker:unionfind"
			case "digest":
				input.PlanSHA256 = strings.Repeat("a", 64)
			case "node":
				node.ID = "other-node"
			case "version":
				node.CredentialVersion++
			case "expired":
				_, err := f.a.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()-interval '1 second'`)
				must(t, err)
			case "superseded":
				_, err := f.a.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_token=$1`, strings.Repeat("a", 64))
				must(t, err)
			case "cancel":
				_, err := f.store.Cancel(f.ctx, f.actor, expectedPlanKey)
				must(t, err)
			case "regrant":
				f.role(t, f.actor.SubjectID, 1, false)
				f.role(t, f.actor.SubjectID, 2, true)
			case "disabled author":
				_, err := f.identities.DisableCAS(f.ctx, f.actor.SubjectID, identity.Revision{CredentialVersion: 1, AuthGeneration: 1})
				must(t, err)
			case "recovery":
				epoch, err := f.identities.BeginRecovery(f.ctx)
				must(t, err)
				must(t, f.identities.FinishRecovery(f.ctx, epoch))
			case "disabled node":
				_, err := f.a.Exec(f.ctx, `UPDATE worker_nodes SET enabled=false WHERE id=$1`, node.ID)
				must(t, err)
			case "rotated node":
				_, err := f.a.Exec(f.ctx, `UPDATE worker_nodes SET credential_version=2 WHERE id=$1`, node.ID)
				must(t, err)
			case "profile":
				_, err := f.a.Exec(f.ctx, `UPDATE problem_reference_requests SET profile_encoding=$1 WHERE id=$2`, []byte(`{}`), input.RequestID)
				must(t, err)
			case "plan":
				_, err := f.a.Exec(f.ctx, `UPDATE problem_reference_expected_plans SET plan_encoding=$1,plan_sha256=encode(sha256($1),'hex') WHERE request_id=$2`, []byte(`{}`), input.RequestID)
				must(t, err)
			}
			got, err := a.AuthorizeReferenceMaterials(f.ctx, node, input)
			if got.Plan.Encoding != nil || !got.Deadline.IsZero() || err == nil {
				t.Fatal("stale reservation authorized material", err)
			}
		})
	}
}

type referenceObjectHook struct {
	reader problem.ReferenceObjectReader
	hook   func()
}

func (h referenceObjectHook) Open(ctx context.Context, o materialstore.Object) (*materialstore.Snapshot, error) {
	f, err := h.reader.Open(ctx, o)
	if err == nil {
		h.hook()
	}
	return f, err
}

func TestReferenceMaterialsRecheckAfterObjectIO(t *testing.T) {
	for _, kind := range []string{"cancel", "regrant", "expire", "rotate"} {
		t.Run(kind, func(t *testing.T) {
			f, a, node, input := claimedMaterialFixture(t)
			var plan problem.ReferencePlan
			must(t, json.Unmarshal(f.want.Encoding, &plan))
			store := referenceObjectHook{f.materials, func() {
				// These real writes must finish while the material snapshot is open:
				// the first authorization transaction cannot retain predecessor locks.
				ctx, cancel := context.WithTimeout(f.ctx, time.Second)
				defer cancel()
				switch kind {
				case "cancel":
					_, err := f.store.Cancel(ctx, f.actor, expectedPlanKey)
					must(t, err)
				case "regrant":
					f.role(t, f.actor.SubjectID, 1, false)
					f.role(t, f.actor.SubjectID, 2, true)
				case "expire":
					_, err := f.b.Exec(ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()-interval '1 second'`)
					must(t, err)
				case "rotate":
					_, err := f.b.Exec(ctx, `UPDATE worker_nodes SET credential_version=2 WHERE id=$1`, node.ID)
					must(t, err)
				}
			}}
			s, err := problemreference.NewReferenceMaterials(a, store)
			must(t, err)
			file, _, err := s.Download(f.ctx, node, input, plan.Source)
			if file != nil || err == nil {
				if file != nil {
					_ = file.Close()
				}
				t.Fatal("material escaped after I/O authorization changed")
			}
			// MaxInFlight=1 proves the discarded object did not retain storage quota.
			probe, err := f.materials.Open(f.ctx, plan.Source)
			must(t, err)
			must(t, probe.Close())
		})
	}
}

func TestReferenceMaterialsWaitCannotOutliveReservation(t *testing.T) {
	f, a, node, input := claimedMaterialFixture(t)
	tx, err := f.b.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(f.ctx, `UPDATE problem_reference_worker_claim SET lease_until=clock_timestamp()+interval '150 milliseconds'`)
	must(t, err)
	done := make(chan error, 1)
	go func() { _, err := a.AuthorizeReferenceMaterials(f.ctx, node, input); done <- err }()
	waitBacklogBlocked(t, f.backlogFixture, tx.Conn().PgConn().PID(), f.appA)
	time.Sleep(200 * time.Millisecond)
	must(t, tx.Commit(f.ctx))
	if err := <-done; !errors.Is(err, problemreference.ErrClaimUnavailable) {
		t.Fatal("lock wait bypassed database expiry", err)
	}
}

func TestReferenceMaterialsThroughRealMTLS(t *testing.T) {
	f, a, _, input := claimedMaterialFixture(t)
	var originalUntil time.Time
	must(t, f.a.QueryRow(f.ctx, `SELECT lease_until FROM problem_reference_worker_claim WHERE singleton`).Scan(&originalUntil))
	s, err := problemreference.NewReferenceMaterials(a, f.materials)
	must(t, err)
	ca := testpki.New(t)
	serverCert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Minute))
	server := httptest.NewUnstartedServer(workerapi.WithReferenceClaims(workerapi.NewHandler(f.first, ca.Roots), a.(problemreference.WorkerClaimer), ca.Roots, s))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots, Certificates: []tls.Certificate{serverCert}}
	server.StartTLS()
	defer server.Close()
	clientCert, _, _ := ca.Issue(t, "spiffe://onlinejudge/reference-worker/reference-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: ca.Roots, Certificates: []tls.Certificate{clientCert}}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	post := func(path string, body any) *http.Response {
		t.Helper()
		raw, err := json.Marshal(body)
		must(t, err)
		response, err := client.Post(server.URL+"/api/v1/worker/reference/"+path, "application/json", bytes.NewReader(raw))
		must(t, err)
		return response
	}
	resp := post("assignment", input)
	var descriptor problemreference.MaterialDescriptor
	must(t, json.NewDecoder(resp.Body).Decode(&descriptor))
	must(t, resp.Body.Close())
	if resp.StatusCode != 200 || !bytes.Equal(descriptor.Encoding, f.want.Encoding) || descriptor.RemainingMS <= 0 {
		t.Fatal("descriptor changed frozen plan")
	}
	resp = post("differential/assignment", input)
	must(t, resp.Body.Close())
	if resp.StatusCode != 404 {
		t.Fatal("claims-only mode exposed differential assignment", resp.StatusCode)
	}
	var plan problem.ReferencePlan
	must(t, json.Unmarshal(descriptor.Encoding, &plan))
	// Source lock is synthetic: independently freeze the actual mTLS object
	// stream, without claiming production source admission for the fixture.
	snapshot, err := plan.FreezeMaterials(f.ctx, func(ctx context.Context, o materialstore.Object) (io.ReadCloser, error) {
		resp := post("material", struct {
			problemreference.MaterialInput
			Object materialstore.Object `json:"object"`
		}{input, o})
		if resp.StatusCode != 200 {
			_ = resp.Body.Close()
			return nil, errors.New("material denied")
		}
		if resp.ContentLength != o.Size || resp.Header.Get("X-Content-SHA256") != o.SHA256 {
			_ = resp.Body.Close()
			return nil, errors.New("object identity changed")
		}
		return resp.Body, nil
	})
	must(t, err)
	got, err := snapshot.ReadSource(f.ctx)
	must(t, err)
	must(t, snapshot.Close())
	if string(got) != "synthetic expected plan source\n" {
		t.Fatal("network changed private source")
	}
	var currentUntil time.Time
	must(t, f.a.QueryRow(f.ctx, `SELECT lease_until FROM problem_reference_worker_claim WHERE singleton`).Scan(&currentUntil))
	if !currentUntil.Equal(originalUntil) {
		t.Fatal("material reads renewed the reservation")
	}
	f.counts(t, 1, 1)
	ordinary, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/reference-node/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Minute))
	ordinaryTransport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: ca.Roots, Certificates: []tls.Certificate{ordinary}}}
	defer ordinaryTransport.CloseIdleConnections()
	ordinaryClient := &http.Client{Transport: ordinaryTransport, Timeout: time.Second}
	body, err := json.Marshal(input)
	must(t, err)
	for _, path := range []string{"assignment", "material"} {
		response, err := ordinaryClient.Post(server.URL+"/api/v1/worker/reference/"+path, "application/json", bytes.NewReader(body))
		must(t, err)
		must(t, response.Body.Close())
		if response.StatusCode != 401 {
			t.Fatal("judge certificate crossed into reference materials", path, response.StatusCode)
		}
	}
	foreign := materialstore.Object{SHA256: strings.Repeat("a", 64), Size: 1}
	resp = post("material", struct {
		problemreference.MaterialInput
		Object materialstore.Object `json:"object"`
	}{input, foreign})
	must(t, resp.Body.Close())
	if resp.StatusCode != 409 {
		t.Fatal("foreign object was exposed")
	}
	_, err = f.store.Cancel(f.ctx, f.actor, expectedPlanKey)
	must(t, err)
	resp = post("assignment", input)
	must(t, resp.Body.Close())
	if resp.StatusCode != 409 {
		t.Fatal("cancelled descriptor was exposed")
	}
	f.counts(t, 1, 0)
}

func TestReferenceMaterialQueriesReadBoundedSingleton(t *testing.T) {
	f, _, _, _ := claimedMaterialFixture(t)
	for _, query := range []string{`SELECT * FROM problem_reference_worker_claim WHERE singleton FOR SHARE`, `SELECT floor(extract(epoch FROM (lease_until-clock_timestamp()))*1000)::bigint FROM problem_reference_worker_claim WHERE singleton`} {
		rows, err := f.a.Query(f.ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) "+query)
		must(t, err)
		for rows.Next() {
			var line string
			must(t, rows.Scan(&line))
			t.Log(line)
		}
		must(t, rows.Err())
		rows.Close()
	}
}
