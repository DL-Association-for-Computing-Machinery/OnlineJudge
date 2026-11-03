package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

type referenceHost struct {
	testHost
	requests   int
	cancel     context.CancelFunc
	stopCancel context.CancelFunc
	fileIDs    int
	fileData   map[string][]byte
}

func (h *referenceHost) UploadFile(ctx context.Context, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if h.fileData == nil {
		h.fileData = map[string][]byte{}
	}
	h.fileIDs++
	id := "upload-" + strconv.Itoa(h.fileIDs)
	h.fileData[id] = append([]byte(nil), data...)
	return id, nil
}

func (h *referenceHost) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, ok := h.fileData[id]
	if !ok || int64(len(raw)) > maximum {
		return nil, errors.New("missing file")
	}
	return append([]byte(nil), raw...), nil
}

func (h *referenceHost) Stop(ctx context.Context) error {
	if h.stopCancel != nil {
		h.stopCancel()
	}
	return h.testHost.Stop(ctx)
}

func (h *referenceHost) Identity() (string, string) { return strings.Repeat("a", 64), "fixed-start" }
func (h *referenceHost) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path != "/run" && r.Method != "DELETE" {
		return h.testHost.RoundTrip(r)
	}
	body := `{}`
	if r.URL.Path == "/run" {
		h.requests++
		body = `[{"status":"Accepted","exitStatus":0,"time":1,"memory":1,"files":{"stdout":"","stderr":""},"fileIds":{"main":"compiled"}}]`
		if h.requests > 1 {
			id := "stdout-" + strconv.Itoa(h.requests)
			h.fileData[id] = []byte("3\n")
			body = `[{"status":"Accepted","exitStatus":0,"time":1,"memory":1,"files":{"stderr":""},"fileIds":{"stdout":"` + id + `"}}]`
			if h.cancel != nil {
				h.cancel()
			}
		}
	} else {
		delete(h.fileData, strings.TrimPrefix(r.URL.Path, "/file/"))
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func validationFixture(t *testing.T) *problem.Prepared {
	t.Helper()
	b := problemfixture.New(t)
	b.AddOriginal(t, "sample/aplusb/sol/correct.cpp", []byte("int main(){}"))
	b.BindReferenceCases(t)
	b.Write(t)
	p, err := problem.LoadBundle(b.Directory, "library-checker:aplusb", b.Lock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestReferenceOwnerStopAndQuarantine(t *testing.T) {
	for _, sourceID := range []string{"library-checker:aplusb", "library-checker:unionfind", "library-checker:zalgorithm", "library-checker:suffixarray", "library-checker:primality_test"} {
		t.Run(sourceID, func(t *testing.T) { testReferenceOwnerStopAndQuarantine(t, sourceID) })
	}
}

func testReferenceOwnerStopAndQuarantine(t *testing.T, sourceID string) {
	for _, scenario := range []string{"pass", "stop", "clean", "cancel", "cancel-stop", "open", "start-error"} {
		t.Run(scenario, func(t *testing.T) {
			p := validationFixture(t)
			if sourceID != "library-checker:aplusb" {
				b := problemfixture.NewUnionFind(t)
				if sourceID == "library-checker:zalgorithm" {
					b = problemfixture.NewZAlgorithm(t)
				}
				if sourceID == "library-checker:suffixarray" {
					b = problemfixture.NewSuffixArray(t)
				}
				if sourceID == "library-checker:primality_test" {
					b = problemfixture.NewPrimality(t)
				}
				sourcePath := b.Document["provenance"].(map[string]any)["source_path"].(string)
				b.AddOriginal(t, sourcePath+"/sol/correct.cpp", []byte("int main(){}"))
				b.BindReferenceCases(t)
				b.Write(t)
				var err error
				p, err = problem.LoadBundle(b.Directory, sourceID, b.Lock)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = p.Close() })
			}
			path := filepath.Join(t.TempDir(), "lock")
			lock, err := acquireLock(path)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			if scenario == "clean" {
				lock.file = &syncFailureFile{File: lock.file.(*os.File)}
			}
			h := &referenceHost{}
			if scenario == "stop" {
				h.stopErr = errors.New("synthetic-reference-secret")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancel" {
				h.cancel = cancel
			}
			if scenario == "cancel-stop" {
				h.stopCancel = cancel
			}
			r, err := runReferenceSourceLocked(ctx, p, lock, func(context.Context) (validationHost, error) {
				raw, e := os.ReadFile(path)
				if e != nil || len(raw) == 0 {
					t.Fatal("sandbox opened before persisted dirty marker")
				}
				if scenario == "open" {
					return nil, errors.New("preflight")
				}
				if scenario == "start-error" {
					return h, errors.New("start ack")
				}
				return h, nil
			}, sourceID)
			if (err == nil) != (scenario == "pass") {
				t.Fatalf("%s err=%v", scenario, err)
			}
			if (r.Status == "reference-validation-passed") != (scenario == "pass") {
				t.Fatal("pass escaped cleanup")
			}
			if scenario != "open" {
				if r.StopConfirmed != (scenario != "stop") || (r.StopDiagnostic != nil) != (scenario == "stop") {
					t.Fatal("stop and marker-clean evidence conflated")
				}
				if scenario == "stop" && (r.StopDiagnostic.Stage != "unknown" || r.StopDiagnostic.Code != "unknown") {
					t.Fatal("invented diagnosis for unknown stop error")
				}
				raw, e := json.Marshal(r)
				if e != nil || strings.Contains(string(raw), "synthetic-reference-secret") {
					t.Fatal("report leaked host error")
				}
			}
			if scenario != "open" && (h.stops != 1 || !h.independent) {
				t.Fatal("stop must happen once with live independent context")
			}
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			quarantined := scenario == "open" || scenario == "stop" || scenario == "clean"
			if (len(raw) != 0) != quarantined {
				t.Fatal("incorrect quarantine state")
			}
		})
	}
}

type dirtyFailure struct{ *os.File }

func (f *dirtyFailure) Sync() error { return errors.New("sync failed") }
func TestReferenceDirtyFailureNeverStarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	lock, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	lock.file = &dirtyFailure{File: lock.file.(*os.File)}
	called := false
	if _, err := runReferenceLocked(context.Background(), nil, lock, func(context.Context) (validationHost, error) { called = true; return nil, nil }); err == nil || called {
		t.Fatal("dirty failure dispatched sandbox")
	}
}

func TestReferenceReportAndCommandGuards(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	t.Setenv("OJ_SANDBOX_EXPERIMENT", "")
	if err := dispatch(context.Background(), []string{"validate-reference"}, logger, nil); err == nil {
		t.Fatal("missing opt-in accepted")
	}
	t.Setenv("OJ_SANDBOX_EXPERIMENT", "1")
	for _, args := range [][]string{{}, {"--bundle", "relative"}, {"--unknown"}, {"extra"}} {
		if err := validateReference(context.Background(), args, logger); err == nil {
			t.Fatal("bad args accepted")
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "INCOMPLETE"), []byte("incomplete"), 0600); err != nil {
		t.Fatal(err)
	}
	r := problemvalidation.Report{Schema: "oj.reference-validation.v1", Status: "reference-validation-failed", Publication: "blocked", Verdict: "SE"}
	if err := persistReferenceReport(dir, r); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "report.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("report permissions")
	}
	if _, err := os.Stat(filepath.Join(dir, "INCOMPLETE")); !os.IsNotExist(err) {
		t.Fatal("successful report retained incomplete marker")
	}
	if err := persistReferenceReport(dir, r); err == nil {
		t.Fatal("overwrote existing report")
	}
	lock := filepath.Join(t.TempDir(), "lock.json")
	if err := os.WriteFile(lock, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readValidationLock(lock); err == nil {
		t.Fatal("unreviewed lock accepted")
	}
}

func TestReferenceSourceAndSlotSelectionGuards(t *testing.T) {
	t.Setenv("OJ_SANDBOX_EXPERIMENT", "1")
	b := problemfixture.New(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, source := range []string{"library-checker:unionfind", "library-checker:zalgorithm", "library-checker:suffixarray", "library-checker:primality_test", "library-checker:other"} {
		t.Setenv("OJ_WORKER_M0_SLOT", "secondary")
		output := filepath.Join(t.TempDir(), "report")
		args := []string{"--source-id", source, "--bundle", b.Directory, "--source-lock", filepath.Join(t.TempDir(), "lock.json"), "--output", output}
		if err := validateReference(context.Background(), args, logger); err == nil {
			t.Fatal("source/slot override accepted")
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("invalid selection created output")
		}
	}
	if err := dispatch(context.Background(), []string{"recover-reference", strings.Repeat("a", 64)}, logger, nil); err == nil {
		t.Fatal("reference recovery accepted M0 override")
	}
}

func TestReferenceOutputNeverMutatesBundle(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.Mkdir(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	if referenceOutputOutside(bundle, filepath.Join(bundle, "report")) {
		t.Fatal("nested report admitted")
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(bundle, alias); err != nil {
		t.Fatal(err)
	}
	if referenceOutputOutside(bundle, filepath.Join(alias, "report")) {
		t.Fatal("aliased nested report admitted")
	}
	caseAlias := filepath.Join(root, "BUNDLE")
	if info, err := os.Stat(caseAlias); err == nil {
		source, err := os.Stat(bundle)
		if err != nil {
			t.Fatal(err)
		}
		if os.SameFile(source, info) && referenceOutputOutside(bundle, filepath.Join(caseAlias, "report")) {
			t.Fatal("case alias admitted a report inside the input bundle")
		}
	}
	if !referenceOutputOutside(bundle, filepath.Join(root, "report")) {
		t.Fatal("independent report rejected")
	}
}
