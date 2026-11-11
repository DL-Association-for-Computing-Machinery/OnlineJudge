//go:build integration && linux && live_reference

package integration_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/testpki"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerapi"
)

// TestLiveReferenceReports is driven by a real Darwin Worker, never by a
// synthetic report. Run only in the documented disposable Linux fixture.
func TestLiveReferenceReports(t *testing.T) {
	if os.Getenv("OJ_LIVE_REFERENCE_FIXTURE") != "1" {
		t.Skip("requires the explicit live reference fixture")
	}
	bundle, out := os.Getenv("OJ_PRIVATE_DRAFT_BUNDLE_DIR"), os.Getenv("OJ_LIVE_REFERENCE_OUTPUT")
	if !filepath.IsAbs(bundle) || !filepath.IsAbs(out) {
		t.Fatal("requires absolute bundle and output paths")
	}
	info, err := os.Lstat(out)
	must(t, err)
	entries, err := os.ReadDir(out)
	must(t, err)
	if !info.IsDir() || info.Mode().Perm() != 0700 || len(entries) != 0 {
		t.Fatal("fixture output must be a new, private directory")
	}
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	other := f.actor
	other.SubjectID = identityID(1101)
	f.role(t, other.SubjectID, 0, true)
	ctx, cancel := context.WithTimeout(t.Context(), 14*time.Minute)
	defer cancel()
	lock, err := os.ReadFile("../../config/problem-sources/library-checker.json")
	must(t, err)
	access, err := problempg.NewReferenceAccess(f.access)
	must(t, err)
	store, err := referencepg.New(f.a, access)
	must(t, err)
	// The next source can only enqueue if the last terminal report freed quota.
	must(t, referencepg.Configure(ctx, f.a, problemreference.Limits{Global: 1, Owner: 1}))
	root := filepath.Join(t.TempDir(), "objects")
	must(t, os.Mkdir(root, 0700))
	materials, err := materialstore.OpenLocal(materialstore.Config{Root: root, MaxObjectBytes: 128 << 20, MaxInFlight: 1})
	must(t, err)
	t.Cleanup(func() { must(t, materials.Close()) })
	sessions := fixedDraftSession{view: redisstore.View{SubjectID: f.actor.SubjectID, AuthGeneration: f.actor.AuthGeneration, RecoveryID: f.actor.RecoveryID}}
	credential, err := redisstore.ParseCredential(base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	must(t, err)
	drafts, err := problem.New(sessions, f.repository, materials)
	must(t, err)
	plans, err := problemreference.NewPlanService(sessions, store, f.repository, materials, lock)
	must(t, err)
	serverDone := serveLiveReferences(t, ctx, out, f, store, materials, lock)
	type fixtureSource struct {
		id    string
		cases int
	}
	sources := []fixtureSource{
		{"aplusb", 12}, {"unionfind", 18}, {"zalgorithm", 29}, {"suffixarray", 50}, {"primality_test", 13},
		{"static_range_sum", 11}, {"associative_array", 20}, {"point_add_range_sum", 21}, {"staticrmq", 27}, {"convolution_mod", 53},
	}
	var versionPin string
	var suffixSizes []int64
	switch os.Getenv("OJ_LIVE_REFERENCE_SUPPLEMENT") {
	case "":
	case "associative_array":
		sources = []fixtureSource{{"associative_array", 22}}
		versionPin = "4d6dd8e3fe97a0e1ec6585ff538219d811f660bdfff7cc2172bf6092cf858928"
		suffixSizes = []int64{24, 66}
	case "suffixarray":
		sources = []fixtureSource{{"suffixarray", 52}}
		versionPin = "7ddd631ac3f2f76e4cf56f85b23ee71fbdc0d5f54c40bdc968d27e933297b777"
		suffixSizes = []int64{17, 19}
	case "zalgorithm":
		sources = []fixtureSource{{"zalgorithm", 31}}
		versionPin = "965239246cbdbd63f1641cfcf551d9c8f3a65cd558e194668ad845cac1e39711"
		suffixSizes = []int64{2, 500001}
	case "aplusb":
		sources = []fixtureSource{{"aplusb", 17}}
		versionPin = "74c7797f5484daf9df01b1be2e5a537eba7b05b1ef20efc08110c61e3c1490fd"
		suffixSizes = []int64{4, 4, 4, 21, 21}
	case "unionfind":
		sources = []fixtureSource{{"unionfind", 20}}
		versionPin = "42e62825925bf727c394f3435c2b50f850758576c392d082fb628f7219e4f07d"
		suffixSizes = []int64{10, 10}
	case "static_range_sum":
		sources = []fixtureSource{{"static_range_sum", 13}}
		versionPin = "a93676964a96fb5dd0320441307eb0bb11ed41b0a1e1d4441c47d124a644e375"
		suffixSizes = []int64{10, 19}
	default:
		t.Fatal("unknown supplement fixture")
	}
	for _, source := range sources {
		if !t.Run(source.id, func(t *testing.T) {
			sourceID := "library-checker:" + source.id
			prepared, err := problem.LoadBundle(bundle, sourceID, lock)
			must(t, err)
			t.Cleanup(func() { must(t, prepared.Close()) })
			identity, err := prepared.ImportIdentity()
			must(t, err)
			catalog, err := problemreference.NewCatalog(map[string]*problem.Prepared{sourceID: prepared})
			must(t, err)
			author, err := problemreference.New(sessions, store, catalog)
			must(t, err)
			draft, err := drafts.Create(ctx, credential, "live-draft-"+source.id, prepared)
			must(t, err)
			key := "live-reference-" + source.id
			request, err := author.EnqueueForSubject(ctx, credential, f.actor.SubjectID, key, draft.ID)
			must(t, err)
			plan, err := plans.PrepareForSubject(ctx, credential, f.actor.SubjectID, key)
			must(t, err)
			var formal problem.ReferencePlan
			must(t, json.Unmarshal(plan.Encoding, &formal))
			if request.State != problemreference.Queued || formal.SourceID != sourceID ||
				formal.Version != identity.Version || formal.ImportRevision != identity.ImportRevision ||
				len(formal.Cases) != source.cases || (plan.Differential != nil) != (source.id == "convolution_mod") {
				t.Fatal("persisted request does not match the frozen draft and fixed source")
			}
			if versionPin != "" && formal.Version != versionPin {
				t.Fatal("persisted plan lacks the pinned local version")
			}
			if source.id == "static_range_sum" && versionPin != "" {
				if formal.Cases[11].Answer.SHA256 != liveReferenceDigest([]byte("0\n")) ||
					formal.Cases[12].Answer.SHA256 != liveReferenceDigest([]byte("1000000000\n")) {
					t.Fatal("persisted static range sum plan lost its boundary answers")
				}
			}
			for i, size := range suffixSizes {
				pos := len(formal.Cases) - len(suffixSizes) + i
				if c := formal.Cases[pos]; c.ID != fmt.Sprintf("case-%04d", pos+1) || c.Input.Size != size {
					t.Fatal("persisted plan lacks the pinned local boundary points")
				}
			}
			if source.id == "suffixarray" && versionPin != "" {
				if formal.Cases[50].Answer.SHA256 != liveReferenceDigest([]byte("8 0 12 4 10 2 14 6 9 1 13 5 11 3 15 7\n")) ||
					formal.Cases[51].Answer.SHA256 != liveReferenceDigest([]byte("16 14 12 10 8 6 4 2 0 17 15 13 11 9 7 5 3 1\n")) {
					t.Fatal("persisted suffix array plan lost its structure answers")
				}
			}
			if source.id == "unionfind" && versionPin != "" {
				if formal.Cases[18].Answer.Size != 2 || formal.Cases[19].Answer.Size != 0 ||
					formal.Cases[19].Answer.SHA256 != liveReferenceDigest(nil) {
					t.Fatal("persisted unionfind plan lost its singleton answers")
				}
			}
			if source.id == "associative_array" && versionPin != "" {
				if formal.Cases[20].Answer.SHA256 != liveReferenceDigest([]byte("0\n")) ||
					formal.Cases[21].Answer.SHA256 != liveReferenceDigest([]byte("1000000000000000000\n")) {
					t.Fatal("persisted associative array plan lost its boundary answers")
				}
			}
			writeLiveReferenceJSON(t, out, "ready-"+source.id, problemreference.ClaimInput{RequestID: request.ID, SourceID: sourceID, PlanSHA256: plan.SHA256})
			report := readLiveReferenceReport(t, ctx, serverDone, f, request.ID)
			if report.formalSHA != plan.SHA256 || report.evidenceSHA != liveReferenceDigest(report.raw) {
				t.Fatal("stored report digest does not match the original plan and bytes")
			}
			var evidence problemreference.WorkerEvidence
			must(t, json.Unmarshal(report.raw, &evidence))
			if evidence.Verdict != "AC" || !evidence.StopConfirmed || len(evidence.Points) != len(formal.Cases) ||
				evidence.ProfileSHA256 != request.Specification.ProfileSHA256 {
				t.Fatal("accepted evidence lacks complete AC or stop confirmation")
			}
			for i, point := range evidence.Points {
				if point.ID != formal.Cases[i].ID || point.Verdict != "AC" {
					t.Fatal("accepted formal points do not match the persisted order")
				}
			}
			view, err := author.ReportForSubject(ctx, credential, f.actor.SubjectID, key)
			must(t, err)
			if view.RequestID != request.ID || view.DraftID != draft.ID || view.Version != identity.Version ||
				view.Formal.Verdict != "AC" || view.Formal.Passed != source.cases || view.Formal.Total != source.cases {
				t.Fatal("author projection does not match the terminal report")
			}
			if _, err := store.AuthorReport(ctx, other, key); !errors.Is(err, problemreference.ErrNotFound) {
				t.Fatal("another authorized author could observe the private report")
			}
			differential, matched := "", 0
			if plan.Differential != nil {
				if evidence.Comparison == nil || evidence.Comparison.PlanSHA256 != plan.Differential.SHA256 ||
					view.Comparison == nil || view.Comparison.Status != "passed" || view.Comparison.Matched != 24 ||
					view.Comparison.Total != 24 || view.Comparison.Counterexample != nil {
					t.Fatal("convolution did not accept the planned independent comparison")
				}
				differential, matched = plan.Differential.SHA256, view.Comparison.Matched
			} else if evidence.Comparison != nil || view.Comparison != nil {
				t.Fatal("formal-only request obtained comparison evidence")
			}
			var version, revision, draftState, validation string
			var active bool
			must(t, f.b.QueryRow(ctx, `SELECT content_version, request_sha256, state, validation_state,
				EXISTS (SELECT 1 FROM problem_reference_worker_claim WHERE lease_until > clock_timestamp())
				FROM problem_drafts WHERE id=$1`, draft.ID).Scan(&version, &revision, &draftState, &validation, &active))
			if version != identity.Version || revision != identity.ImportRevision || draftState != "private-draft" ||
				validation != "not-run" || active {
				t.Fatal("report changed draft state or left the reservation active")
			}
			if source.id == "aplusb" {
				verifyLiveCoverageReview(t, ctx, f, store, credential, other, out, versionPin != "", problemreference.ReviewIntent{
					RequestKey: "live-coverage-review", DraftID: draft.ID, ImportRevision: revision, ReferenceRequestID: request.ID,
				})
			}
			writeLiveReferenceJSON(t, out, "accepted-"+source.id, map[string]any{
				"source_id": sourceID, "content_version": version, "import_revision": revision,
				"formal_plan_sha256": plan.SHA256, "profile_sha256": evidence.ProfileSHA256, "evidence_sha256": report.evidenceSHA,
				"formal_ac": source.cases, "comparison_plan_sha256": differential, "comparison_matched": matched,
				"stop_confirmed": true, "active_reservation": active, "author_summary_matches": true,
				"other_author_denied": true, "draft_state": draftState, "draft_validation_state": validation,
			})
			t.Logf("accepted %d formal points, %d independent matches; reservation released", source.cases, matched)
		}) {
			return
		}
	}
	var draftsCount, requestsCount, reportsCount int
	must(t, f.b.QueryRow(ctx, `SELECT (SELECT count(*) FROM problem_drafts),
		(SELECT count(*) FROM problem_reference_requests WHERE state='reported'),
		(SELECT count(*) FROM problem_reference_worker_reports)`).Scan(&draftsCount, &requestsCount, &reportsCount))
	if draftsCount != len(sources) || requestsCount != len(sources) || reportsCount != len(sources) {
		t.Fatal("incomplete or duplicate live reference history")
	}
}

func serveLiveReferences(t *testing.T, ctx context.Context, out string, f draftFixture, store *referencepg.Store, local *materialstore.Local, lock []byte) <-chan error {
	t.Helper()
	const node = "live-reference-node"
	_, err := f.a.Exec(ctx, `INSERT INTO worker_nodes(id,credential_version) VALUES($1,$2)`, node, 1)
	must(t, err)
	claims, err := referencepg.NewReferenceClaimer(store, submissionpg.LockReferenceNode, node, liveReferenceDigest(lock))
	must(t, err)
	materials, err := problemreference.NewReferenceMaterials(claims, local)
	must(t, err)
	ca := testpki.New(t)
	expires := time.Now().Add(16 * time.Minute)
	server, _, _ := ca.Issue(t, "", x509.ExtKeyUsageServerAuth, expires)
	_, cert, key := ca.Issue(t, "spiffe://onlinejudge/reference-worker/"+node+"/1", x509.ExtKeyUsageClientAuth, expires)
	for name, path := range map[string]string{"ca.pem": ca.CAFile, "cert.pem": cert, "key.pem": key} {
		raw, err := os.ReadFile(path)
		must(t, err)
		must(t, os.WriteFile(filepath.Join(out, name), raw, 0600))
	}
	// Only the disposable container listens here; the documented host binding is
	// 127.0.0.1. Database sockets and material storage stay inside the container.
	listener, err := net.Listen("tcp4", "0.0.0.0:18443")
	must(t, err)
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.Roots, Certificates: []tls.Certificate{server}}
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, TLSConfig: cfg,
		Handler: workerapi.WithReferenceExecution(http.NotFoundHandler(), claims, claims, ca.Roots, materials, claims)}
	t.Cleanup(func() {
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		must(t, srv.Shutdown(shutdown))
	})
	done := make(chan error, 1)
	go func() { done <- srv.Serve(tls.NewListener(listener, cfg)) }()
	return done
}

type liveReferenceReport struct {
	formalSHA, evidenceSHA string
	raw                    []byte
}

func readLiveReferenceReport(t *testing.T, ctx context.Context, serverDone <-chan error, f draftFixture, id string) liveReferenceReport {
	t.Helper()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-serverDone:
			t.Fatalf("live mTLS fixture stopped: %v", err)
		case <-ctx.Done():
			t.Fatal("no terminal report before the fixture deadline")
		case <-ticker.C:
			var state string
			var plan, sha *string
			var raw []byte
			must(t, f.b.QueryRow(ctx, `SELECT r.state,p.plan_sha256,p.evidence_sha256,p.evidence
				FROM problem_reference_requests r LEFT JOIN problem_reference_worker_reports p ON p.request_id=r.id
				WHERE r.id=$1`, id).Scan(&state, &plan, &sha, &raw))
			if state == problemreference.Reported && plan != nil && sha != nil {
				return liveReferenceReport{*plan, *sha, raw}
			}
			if state != problemreference.Queued || plan != nil || sha != nil || raw != nil {
				t.Fatal("request and evidence did not transition atomically")
			}
		}
	}
}

func writeLiveReferenceJSON(t *testing.T, dir, name string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	must(t, err)
	path := filepath.Join(dir, name)
	must(t, os.WriteFile(path+".tmp", raw, 0600))
	must(t, os.Rename(path+".tmp", path+".json"))
}

func liveReferenceDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
