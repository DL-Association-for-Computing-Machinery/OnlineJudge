package workerclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
)

func acquisitionServer(t *testing.T, handler http.Handler) *Acquirer {
	t.Helper()
	ca := testpki.New(t)
	serverCert, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, time.Now().Add(time.Hour))
	cert, _, _ := ca.Issue(t, "spiffe://onlinejudge/worker/node-a/1", x509.ExtKeyUsageClientAuth, time.Now().Add(time.Hour))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || len(r.TLS.VerifiedChains) == 0 || r.Header.Get("Content-Type") != "application/json" || r.URL.RawQuery != "" {
			t.Error("invalid acquisition protocol")
		}
		handler.ServeHTTP(w, r)
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverCert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots}
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	a, err := NewAcquirer(server.URL, ca.Roots, cert)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

func acquisitionFixture() (submission.Lease, materials.Descriptor) {
	l := submission.Lease{Token: strings.Repeat("a", 64), SubmissionID: "12345678123412341234123456789012",
		JudgeGeneration: 1, RecoveryGeneration: 2, AttemptNumber: 3, Until: time.Now().Add(30 * time.Second).UTC()}
	version, manifest, _ := experiment.SelectFixture(experiment.PinnedVersion)
	source := "// 原样源码😀\n"
	d := materials.Descriptor{TaskMaterials: submission.TaskMaterials{SubmissionID: l.SubmissionID, Version: version,
		Manifest: manifest, ManifestSHA256: checksum(manifest), Language: "gnu++17", Source: source}, SourceSHA256: checksum([]byte(source))}
	return l, d
}

func TestClaimBoundaries(t *testing.T) {
	l, _ := acquisitionFixture()
	body := fmt.Sprintf(`{"lease_token":%q,"submission_id":%q,"judge_generation":1,"recovery_generation":2,"attempt_number":3,"lease_until":%q}`,
		l.Token, l.SubmissionID, l.Until.Format(time.RFC3339Nano))
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"success", body, 200, nil}, {"empty", "", 204, submission.ErrNoTask},
		{"revoked", l.Token, 403, submission.ErrNode}, {"unauthorized", "", 401, submission.ErrNode},
		{"unavailable", l.Token, 503, ErrClaimUnknown}, {"redirect", "", 307, ErrClaimUnknown},
		{"null", "null", 200, ErrClaimUnknown}, {"missing", `{}`, 200, ErrClaimUnknown},
		{"oversized", strings.Repeat(" ", 1025) + body, 200, ErrClaimUnknown},
		{"trailing", body + `{}`, 200, ErrClaimUnknown},
		{"token", strings.Replace(body, l.Token, "bad", 1), 200, ErrClaimUnknown},
		{"id", strings.Replace(body, l.SubmissionID, "bad", 1), 200, ErrClaimUnknown},
		{"generation", strings.Replace(body, `"judge_generation":1`, `"judge_generation":0`, 1), 200, ErrClaimUnknown},
		{"recovery", strings.Replace(body, `"recovery_generation":2`, `"recovery_generation":-1`, 1), 200, ErrClaimUnknown},
		{"attempt", strings.Replace(body, `"attempt_number":3`, `"attempt_number":2147483648`, 1), 200, ErrClaimUnknown},
		{"expiry", strings.Replace(body, l.Until.Format(time.RFC3339Nano), "bad", 1), 200, ErrClaimUnknown},
		{"media", body, 200, ErrClaimUnknown}, {"lost reply", "", 200, ErrClaimUnknown},
		{"cancelled", "", 200, ErrClaimUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			a := acquisitionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				data, _ := io.ReadAll(r.Body)
				if r.URL.Path != "/api/v1/worker/claim" || string(data) != "{}" {
					t.Error("claim replayed or wrong body")
				}
				if tc.name == "lost reply" {
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				}
				if tc.name == "cancelled" {
					<-r.Context().Done()
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.name == "media" {
					w.Header().Set("Content-Type", "text/plain")
				}
				w.Header().Set("Location", "/must-not-follow")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			got, err := a.Claim(ctx)
			if !errors.Is(err, tc.want) || calls.Load() != 1 {
				t.Fatalf("err=%v calls=%d", err, calls.Load())
			}
			if tc.want == nil && !reflect.DeepEqual(got, l) {
				t.Fatal("lease attribution changed")
			}
			if tc.want != nil && !reflect.DeepEqual(got, submission.Lease{}) {
				t.Fatal("partial lease exposed")
			}
		})
	}
}

func TestFetchBoundaries(t *testing.T) {
	for _, mode := range []string{"success", "source", "manifest", "manifest hash", "version", "id", "language", "null", "oversized descriptor", "descriptor media", "assignment revoked", "assignment expired", "blob", "digest header", "truncated", "oversized blob", "chunked", "blob media", "missing", "revoked", "expired", "redirect", "cancelled", "invalid lease"} {
		t.Run(mode, func(t *testing.T) {
			lease, descriptor := acquisitionFixture()
			var calls atomic.Int32
			var downloads atomic.Int32
			a := acquisitionServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var req struct {
					Token string `json:"lease_token"`
					ID    string `json:"test_id"`
					Kind  string `json:"kind"`
				}
				if json.NewDecoder(r.Body).Decode(&req) != nil || req.Token != lease.Token {
					t.Error("wrong acquisition body")
				}
				if r.URL.Path == "/api/v1/worker/assignment" {
					d := descriptor
					w.Header().Set("Content-Type", "application/json")
					switch mode {
					case "source":
						d.Source += "changed"
					case "manifest":
						d.Manifest = []byte(`{}`)
						d.ManifestSHA256 = checksum(d.Manifest)
					case "manifest hash":
						d.ManifestSHA256 = strings.Repeat("f", 64)
					case "version":
						d.Version = experiment.Version
					case "id":
						d.SubmissionID = "another-task"
					case "language":
						d.Language = "python"
					case "null":
						fmt.Fprint(w, "null")
						return
					case "oversized descriptor":
						fmt.Fprint(w, strings.Repeat(" ", 6*submission.MaxSourceBytes+32769))
						return
					case "descriptor media":
						w.Header().Set("Content-Type", "text/plain")
					case "assignment revoked":
						w.WriteHeader(403)
						return
					case "assignment expired":
						w.WriteHeader(409)
						return
					}
					_ = json.NewEncoder(w).Encode(d)
					return
				}
				if r.URL.Path != "/api/v1/worker/material" {
					t.Error("redirect followed")
					return
				}
				downloads.Add(1)
				data := map[string]string{"01/input": "1 2\n", "01/answer": "3\n", "02/input": "-7 4\n", "02/answer": "-3\n"}[req.ID+"/"+req.Kind]
				if data == "" {
					t.Error("unscoped material request")
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Length", fmt.Sprint(len(data)))
				w.Header().Set("X-Content-SHA256", checksum([]byte(data)))
				// Fail the last blob to prove partial sets are never exposed.
				if downloads.Load() == 4 {
					switch mode {
					case "blob":
						data = strings.Repeat("x", len(data))
					case "digest header":
						w.Header().Set("X-Content-SHA256", strings.Repeat("f", 64))
					case "truncated":
						data = data[:1]
					case "oversized blob":
						w.Header().Set("Content-Length", fmt.Sprint(judging.TextBudget+1))
					case "chunked":
						w.Header().Del("Content-Length")
						w.(http.Flusher).Flush()
					case "blob media":
						w.Header().Set("Content-Type", "text/plain")
					case "missing":
						w.WriteHeader(404)
						return
					case "revoked":
						w.WriteHeader(403)
						return
					case "expired":
						w.WriteHeader(409)
						return
					case "redirect":
						w.Header().Set("Location", "/must-not-follow")
						w.WriteHeader(307)
						return
					case "cancelled":
						<-r.Context().Done()
						return
					}
				}
				fmt.Fprint(w, data)
			}))
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			if mode == "invalid lease" {
				lease.Token = "bad"
			}
			d, blobs, err := a.Fetch(ctx, lease)
			if mode == "success" {
				if err != nil || !reflect.DeepEqual(d, descriptor) || len(blobs) != 4 || calls.Load() != 5 {
					t.Fatalf("valid materials failed: %v", err)
				}
				for _, data := range []string{"1 2\n", "3\n", "-7 4\n", "-3\n"} {
					if string(blobs[checksum([]byte(data))]) != data {
						t.Fatal("material bytes changed")
					}
				}
				return
			}
			want := ErrMaterials
			if strings.Contains(mode, "revoked") {
				want = submission.ErrNode
			}
			if strings.Contains(mode, "expired") || mode == "invalid lease" {
				want = submission.ErrLease
			}
			if !errors.Is(err, want) || blobs != nil || !reflect.DeepEqual(d, materials.Descriptor{}) {
				t.Fatalf("partial or unexpected failure: %v", err)
			}
			if strings.Contains(err.Error(), lease.Token) || calls.Load() > 5 {
				t.Fatal("credential leak or retry")
			}
			if mode == "invalid lease" && calls.Load() != 0 {
				t.Fatal("invalid lease sent")
			}
		})
	}
}
