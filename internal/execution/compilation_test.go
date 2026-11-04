package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func compilationFiles() []SourceFile {
	return []SourceFile{{"sol/correct.cpp", []byte("source")}, {"lib/prime.hpp", []byte{0, 255, '\n'}}, {"lib/montgomery.hpp", []byte("mont")}, {"lib/util.hpp", []byte("util")}, {"common/random.h", []byte("UTF-8 原始字节")}}
}

type compilationFixture struct {
	*cacheFixture
	g                                           *CachedGoJudge
	want                                        []SourceFile
	uploadCalls, readCalls, failUpload, badRead int
	cleanupFailure                              bool
	maxObjects                                  int
	onFirstUpload                               func()
	onDelete                                    func(*http.Request)
}

func (f *compilationFixture) UploadFile(ctx context.Context, data []byte) (string, error) {
	f.uploadCalls++
	if f.uploadCalls == 1 && f.onFirstUpload != nil {
		f.onFirstUpload()
	}
	id, err := f.cacheFixture.UploadFile(ctx, data)
	if f.uploadCalls == f.failUpload {
		return "", errors.New("PRIVATE-unknown-upload")
	}
	return id, err
}
func (f *compilationFixture) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	f.readCalls++
	raw, err := f.cacheFixture.ReadFile(ctx, id, maximum)
	if f.readCalls == f.badRead && len(raw) != 0 {
		raw[0] ^= 255
	}
	return raw, err
}
func (f *compilationFixture) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == "DELETE" && f.onDelete != nil {
		f.onDelete(r)
	}
	if r.Method == "DELETE" && f.cleanupFailure {
		return nil, context.DeadlineExceeded
	}
	if r.URL.Path == "/run" {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var envelope struct {
			Cmd []map[string]any `json:"cmd"`
		}
		if json.Unmarshal(raw, &envelope) != nil || len(envelope.Cmd) != 1 {
			f.t.Fatal("bad compilation envelope")
		}
		c := envelope.Cmd[0]
		if len(c["args"].([]any)) > 1 {
			copyIn := c["copyIn"].(map[string]any)
			if len(copyIn) != len(f.want) {
				f.t.Fatal("incomplete compilation files")
			}
			for _, file := range f.want {
				entry, ok := copyIn[file.Name].(map[string]any)
				if !ok || len(entry) != 1 {
					f.t.Fatal("file not represented by one owned cache ID")
				}
				id, ok := entry["fileId"].(string)
				if !ok || !bytes.Equal(f.data[id], file.Data) {
					f.t.Fatal("compilation bytes changed or foreign cache used")
				}
			}
			f.maxObjects = f.g.liveObjects
		}
	}
	return f.cacheFixture.RoundTrip(r)
}

func compilationAdapter(t *testing.T, files []SourceFile) (*CachedGoJudge, *compilationFixture, experiment.ExecutionProfile) {
	t.Helper()
	f := &compilationFixture{cacheFixture: &cacheFixture{t: t, data: map[string][]byte{}, reference: true, referenceOutput: 512 << 10, compilationEntry: "sol/correct.cpp", stdout: []byte("YES\n")}}
	for _, file := range files {
		f.want = append(f.want, SourceFile{Name: file.Name, Data: append([]byte(nil), file.Data...)})
	}
	g, err := NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f, experiment.ReferencePrimality)
	if err != nil {
		t.Fatal(err)
	}
	f.g = g
	if err := g.Verify(context.Background(), g.runtime); err != nil {
		t.Fatal(err)
	}
	return g, f, g.runtime
}

func TestCompilationPreflightsAllPathsAndBytesBeforeIO(t *testing.T) {
	for _, invalid := range []string{"/root/x", "../outside", "sol/./correct.cpp", "lib//x", "lib/../x", `lib\x`, "lib/", "", ".", "lib/..", "main", "stdout/x", "stdin", "stderr", "头.h", strings.Repeat("a", 129)} {
		t.Run(invalid, func(t *testing.T) {
			files := compilationFiles()
			files[1].Name = invalid
			g, f, p := compilationAdapter(t, files)
			if _, err := g.CompileFiles(context.Background(), p, files); err == nil || f.uploadCalls != 0 || f.compiles != 0 {
				t.Fatal("bad target reached I/O")
			}
		})
	}
	for _, mode := range []string{"duplicate", "case-alias", "prefix", "missing-entry", "too-many", "too-large"} {
		t.Run(mode, func(t *testing.T) {
			files := compilationFiles()
			switch mode {
			case "duplicate":
				files[1].Name = files[2].Name
			case "case-alias":
				files[1].Name = strings.ToUpper(files[2].Name)
			case "prefix":
				files[1].Name = "lib"
			case "missing-entry":
				files[0].Name = "sol/other.cpp"
			case "too-many":
				files = append(files, SourceFile{Name: "extra.h"})
			case "too-large":
				files[1].Data = make([]byte, 64<<10)
			}
			g, f, p := compilationAdapter(t, files)
			if _, err := g.CompileFiles(context.Background(), p, files); err == nil || f.uploadCalls != 0 {
				t.Fatal("invalid bundle partially uploaded")
			}
		})
	}
}

func TestDependencyCompilationAndThirteenPointLedger(t *testing.T) {
	files := compilationFiles()
	g, f, p := compilationAdapter(t, files)
	f.onFirstUpload = func() { files[1].Data[0] = 1; files[2].Name = "changed.h" }
	o, err := g.CompileFiles(context.Background(), p, files)
	if err != nil || o.Artifact == "" || f.maxObjects != 6 || g.liveObjects != 1 || len(f.deletes) != 5 {
		t.Fatal("compilation peak/cleanup", err)
	}
	for i := 0; i < 13; i++ {
		if _, err := g.Run(context.Background(), p, o.Artifact, []byte("1\n2\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.Remove(context.Background(), o.Artifact); err != nil {
		t.Fatal(err)
	}
	if g.operations != 97 || g.liveObjects != 0 || g.liveBytes != 0 || len(g.objects) != 0 || len(f.data) != 0 {
		t.Fatal("dependency operation/ownership ledger leaked")
	}
	if _, err := g.CompileFiles(context.Background(), p, compilationFiles()); err == nil {
		t.Fatal("compiler reused")
	}
}

func TestCompilationExactAggregateAndEntryIndependentOfOrder(t *testing.T) {
	files := compilationFiles()
	other := 0
	for _, file := range files[1:] {
		other += len(file.Data)
	}
	files[0].Data = bytes.Repeat([]byte{'s'}, (64<<10)-other)
	files = append(files[1:], files[0])
	g, f, p := compilationAdapter(t, files)
	o, err := g.CompileFiles(context.Background(), p, files)
	if err != nil || o.Artifact == "" || f.maxObjects != 6 || g.totalBytes != 128<<10 {
		t.Fatal("exact source aggregate or fixed entry rejected", err)
	}
	if err := g.Remove(context.Background(), o.Artifact); err != nil {
		t.Fatal(err)
	}
}

func TestCompilationFailuresCleanEveryKnownFile(t *testing.T) {
	for n := 1; n <= 5; n++ {
		files := compilationFiles()
		g, f, p := compilationAdapter(t, files)
		f.failUpload = n
		_, err := g.CompileFiles(context.Background(), p, files)
		if err == nil || f.compiles != 0 || len(f.deletes) != n-1 || g.liveObjects != 1 || !g.failed {
			t.Fatalf("unknown upload %d not fenced/cleaned", n)
		}
		d := FailureDetails(err)
		role := "dependency"
		if n == 1 {
			role = "source"
		}
		if d.Entries[0].Phase != "upload" || d.Entries[0].Role != role || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("unsafe failure projection")
		}
	}
	files := compilationFiles()
	g, f, p := compilationAdapter(t, files)
	f.badRead = 5
	_, err := g.CompileFiles(context.Background(), p, files)
	if err == nil || f.compiles != 0 || len(f.deletes) != 5 || g.liveObjects != 0 || FailureDetails(err).Entries[0] != (FailureEntry{Phase: "readback", Role: "dependency", Code: "integrity"}) {
		t.Fatal("last dependency integrity/cleanup")
	}
}

func TestDependencyCleanupFailureRetainsExecutableAndBoundedEvidence(t *testing.T) {
	files := compilationFiles()
	g, f, p := compilationAdapter(t, files)
	f.cleanupFailure = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var cleanupContext context.Context
	deletes := 0
	f.onDelete = func(r *http.Request) {
		deletes++
		if deletes == 1 {
			cleanupContext = r.Context()
			cancel()
		}
		deadline, ok := r.Context().Deadline()
		remaining := time.Until(deadline)
		if !ok || r.Context() != cleanupContext || r.Context() == ctx || r.Context().Err() != nil || remaining <= 0 || remaining > 5*time.Second {
			t.Fatal("compilation cleanup did not share one independent five-second context")
		}
	}
	o, err := g.CompileFiles(ctx, p, files)
	if deletes != 5 {
		t.Fatal("cleanup failure skipped known compilation objects")
	}
	f.onDelete = nil
	if err == nil || o.Artifact == "" || o.Artifact != g.executable || g.liveObjects != 6 || f.maxObjects != 6 {
		t.Fatal("successful artifact lost after dependency cleanup failure")
	}
	d := FailureDetails(err)
	if len(d.Entries) != 6 || d.Truncated || d.Entries[0].Role != "source" || d.Entries[4].Role != "dependency" || d.Entries[5].Code != "canceled" {
		t.Fatal("joined compilation cleanup evidence lost")
	}
	cleanup := g.Remove(context.Background(), o.Artifact)
	joined := errors.Join(err, context.Canceled, cleanup, context.Canceled, context.DeadlineExceeded)
	d = FailureDetails(joined)
	if len(d.Entries) != 8 || !d.Truncated || d.Entries[7].Role != "executable" {
		t.Fatal("larger cleanup widened or lost diagnostic bounds")
	}
}

func TestOldTiersCannotBorrowCompilationFiles(t *testing.T) {
	for _, tier := range []experiment.ReferenceTier{experiment.ReferenceSmall, experiment.ReferenceMedium, experiment.ReferenceStaticRangeSum, experiment.ReferenceAssociativeArray} {
		g, f := evidenceAdapter(t)
		c, _ := experiment.ResolveReferenceTier(tier)
		g.runtime, g.limits, g.reference = c.Runtime, c.Files, true
		if err := g.Verify(context.Background(), g.runtime); err != nil {
			t.Fatal(err)
		}
		if _, err := g.CompileFiles(context.Background(), g.runtime, compilationFiles()); err == nil || f.uploads != 0 {
			t.Fatal("old tier borrowed files")
		}
	}
	g, f := evidenceAdapter(t)
	if err := g.Verify(context.Background(), g.runtime); err != nil {
		t.Fatal(err)
	}
	if _, err := g.CompileFiles(context.Background(), g.runtime, compilationFiles()); err == nil || f.uploads != 0 {
		t.Fatal("M0 borrowed files")
	}
	files := compilationFiles()
	dependent, fixture, p := compilationAdapter(t, files)
	if _, err := dependent.Compile(context.Background(), p, "source"); err == nil || fixture.uploadCalls != 0 {
		t.Fatal("dependency tier accepted legacy source")
	}
	if !reflect.DeepEqual(experiment.PinnedEnvironment().CompileArgs, []string{"/usr/local/bin/g++", "-std=gnu++17", "-O2", "-pipe", "main.cpp", "-o", "main"}) {
		t.Fatal("old entry changed")
	}
}
