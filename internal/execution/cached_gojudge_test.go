package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

type cacheFixture struct {
	t                             *testing.T
	data                          map[string][]byte
	next, uploads, compiles, runs int
	fail                          string
	deletes                       []string
	block, entered                chan struct{}
	cancelCleanup                 context.CancelFunc
	reference                     bool
	stdout                        []byte
}

func (f *cacheFixture) add(data []byte) string {
	f.next++
	id := "object-" + strconv.Itoa(f.next)
	f.data[id] = append([]byte(nil), data...)
	return id
}
func (f *cacheFixture) UploadFile(ctx context.Context, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f.uploads++
	id := f.add(data)
	if f.fail == "upload" {
		return id, errors.New("synthetic-private-receipt")
	}
	return id, nil
}
func (f *cacheFixture) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, ok := f.data[id]
	if !ok {
		return nil, errors.New("missing cache")
	}
	if f.fail == "hash" {
		return []byte("bad"), nil
	}
	return append([]byte(nil), raw...), nil
}
func (f *cacheFixture) RoundTrip(r *http.Request) (*http.Response, error) {
	switch r.URL.Path {
	case "/version":
		return response(`{"buildVersion":"v1.13.3","os":"linux","platform":"arm64"}`), nil
	case "/config":
		return response(`{"runnerConfig":{"cgroupType":2,"cgroupControllers":["memory","pids","cpu"],"seccomp":true}}`), nil
	}
	if r.Method == "DELETE" {
		if f.cancelCleanup != nil {
			f.cancelCleanup()
		}
		if f.fail == "delete" {
			return nil, errors.New("synthetic-private-delete")
		}
		id := strings.TrimPrefix(r.URL.Path, "/file/")
		f.deletes = append(f.deletes, id)
		delete(f.data, id)
		return response(`{}`), nil
	}
	var request struct {
		Cmd []map[string]any `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Cmd) != 1 {
		return nil, errors.New("bad request")
	}
	c := request.Cmd[0]
	files := c["files"].([]any)
	compile := len(c["args"].([]any)) > 1
	if f.reference {
		stdoutMax := 256 << 10
		if compile {
			stdoutMax = 4096
		}
		if files[1].(map[string]any)["max"] != float64(stdoutMax) || files[2].(map[string]any)["max"] != float64(4096) {
			f.t.Fatal("reference diagnostic/run output budgets mixed")
		}
	}
	copyIn := c["copyIn"].(map[string]any)
	result := map[string]any{"status": "Accepted", "exitStatus": 0, "time": 1, "memory": 1}
	if compile {
		f.compiles++
		if _, ok := copyIn["main.cpp"].(map[string]any)["fileId"]; !ok || copyIn["main.cpp"].(map[string]any)["content"] != nil {
			f.t.Error("source was inlined")
		}
		result["files"] = map[string]string{"stdout": "", "stderr": ""}
		if f.fail == "compile-stderr" || f.fail == "compile-stdout" {
			role := strings.TrimPrefix(f.fail, "compile-")
			result["files"].(map[string]string)[role] = strings.Repeat("x", 3*(4096+1)+1)
		}
		result["fileIds"] = map[string]string{"main": f.add([]byte("executable"))}
	} else {
		f.runs++
		if f.block != nil {
			close(f.entered)
			select {
			case <-f.block:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		inputID, _ := files[0].(map[string]any)["fileId"].(string)
		if inputID == "" || files[0].(map[string]any)["content"] != nil {
			f.t.Error("stdin was inlined")
		}
		raw := f.data[inputID]
		if f.stdout != nil {
			raw = f.stdout
		}
		ids := map[string]string{"stdout": f.add(raw)}
		inline := map[string]string{"stderr": ""}
		switch f.fail {
		case "dual":
			inline["stdout"] = "forbidden"
		case "role":
			ids["main"] = ids["stdout"]
			delete(ids, "stdout")
		case "reuse":
			ids["stdout"] = copyIn["main"].(map[string]any)["fileId"].(string)
		case "over":
			f.data[ids["stdout"]] = bytes.Repeat([]byte{'x'}, int(files[1].(map[string]any)["max"].(float64))+1)
		case "OLE", "collector-OLE":
			f.data[ids["stdout"]] = bytes.Repeat([]byte{'x'}, int(files[1].(map[string]any)["max"].(float64))+1)
			result["status"] = "Output Limit Exceeded"
			result["fileError"] = []any{map[string]string{"name": "stdout", "type": "CollectSizeExceeded"}}
			if f.fail == "collector-OLE" {
				result["error"] = "Output Limit Exceeded"
			}
		case "sigxfsz":
			result["status"] = "Output Limit Exceeded"
			result["exitStatus"] = 25
		case "negative":
			result["time"] = -1
		case "run-stderr":
			inline["stderr"] = strings.Repeat("x", 3*(4096+1)+1)
		}
		result["files"], result["fileIds"] = inline, ids
	}
	raw, err := json.Marshal([]any{result})
	if err != nil {
		return nil, err
	}
	return response(string(raw)), nil
}

func cachedFixture(t *testing.T) (*CachedGoJudge, *cacheFixture, string) {
	t.Helper()
	f := &cacheFixture{t: t, data: map[string][]byte{}}
	g := NewCachedGoJudge(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f)
	if err := g.Verify(context.Background(), experiment.PinnedEnvironment()); err != nil {
		t.Fatal(err)
	}
	o, err := g.Compile(context.Background(), experiment.PinnedEnvironment(), "source")
	if err != nil || o.Artifact == "" || len(f.data) != 1 {
		t.Fatal("cached compile", err)
	}
	return g, f, o.Artifact
}

func TestCachedGoJudgeRawBytesAndCleanup(t *testing.T) {
	g, f, id := cachedFixture(t)
	var raw []byte
	for i := 0; i < 256; i++ {
		raw = append(raw, byte(i))
	}
	o, err := g.Run(context.Background(), experiment.PinnedEnvironment(), id, raw)
	if err != nil || !bytes.Equal(o.Stdout, raw) || o.Status != "Accepted" || len(f.data) != 1 || g.liveObjects != 1 {
		t.Fatal("raw cached run failed", err)
	}
	if err := g.Remove(context.Background(), id); err != nil || len(f.data) != 0 || g.liveBytes != 0 {
		t.Fatal("cached artifact leaked", err)
	}
	if f.compiles != 1 || f.runs != 1 || f.uploads != 2 || len(f.deletes) != 4 {
		t.Fatal("cache lifecycle incomplete")
	}
}

func TestCachedGoJudgeFailureFencesAndOutputEvidence(t *testing.T) {
	for _, failure := range []string{"dual", "role", "reuse", "over", "negative", "OLE", "sigxfsz", "delete"} {
		t.Run(failure, func(t *testing.T) {
			g, f, id := cachedFixture(t)
			f.fail = failure
			o, err := g.Run(context.Background(), experiment.PinnedEnvironment(), id, []byte("bytes"))
			validOLE := failure == "OLE" || failure == "sigxfsz"
			if (err == nil) != validOLE {
				t.Fatalf("%s result=%+v err=%v", failure, o, err)
			}
			if validOLE && (o.Status != "Output Limit Exceeded" || o.OutputTruncated != (failure == "OLE")) {
				t.Fatal("OLE evidence changed")
			}
			if !validOLE {
				before := f.runs
				if _, err := g.Run(context.Background(), experiment.PinnedEnvironment(), id, nil); err == nil || f.runs != before {
					t.Fatal("failed adapter reused")
				}
			}
			f.fail = ""
			if err := g.Remove(context.Background(), id); err != nil {
				t.Fatal("owned cleanup refused after failure", err)
			}
		})
	}
}

func TestCachedUploadIntegrityAndUnknownReceipt(t *testing.T) {
	for _, failure := range []string{"hash", "upload"} {
		t.Run(failure, func(t *testing.T) {
			f := &cacheFixture{t: t, data: map[string][]byte{}, fail: failure}
			g := NewCachedGoJudge(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, f)
			if err := g.Verify(context.Background(), experiment.PinnedEnvironment()); err != nil {
				t.Fatal(err)
			}
			if _, err := g.Compile(context.Background(), experiment.PinnedEnvironment(), "source"); err == nil || strings.Contains(err.Error(), "synthetic-private") {
				t.Fatal("bad upload admitted or leaked")
			}
			if f.compiles != 0 || f.uploads != 1 {
				t.Fatal("compile/retry after bad source")
			}
			if failure == "upload" && (len(f.deletes) != 0 || len(f.data) != 1) {
				t.Fatal("unknown upload was guessed clean")
			}
			if failure == "hash" && len(f.data) != 0 {
				t.Fatal("known corrupt upload not cleaned")
			}
		})
	}
}

func TestCachedBudgetsReserveCleanupAndRejectForeignIDs(t *testing.T) {
	g, f, id := cachedFixture(t)
	for i := 0; i < 100; i++ {
		if _, err := g.Run(context.Background(), experiment.PinnedEnvironment(), id, nil); err != nil {
			break
		}
	}
	if !g.failed || g.operations > g.limits.Operations || len(g.seen) > g.limits.Operations {
		t.Fatal("empty operations escaped bounds")
	}
	if err := g.Remove(context.Background(), id); err != nil || len(f.data) != 0 {
		t.Fatal("quota prevented reserved cleanup", err)
	}
	g, f, _ = cachedFixture(t)
	before := len(f.deletes)
	if err := g.Remove(context.Background(), "foreign"); err == nil || len(f.deletes) != before {
		t.Fatal("foreign artifact deleted")
	}
}

func TestCachedGoJudgeRejectsConcurrentWork(t *testing.T) {
	g, f, id := cachedFixture(t)
	f.block = make(chan struct{})
	f.entered = make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(f.block) }) }
	defer unblock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := g.Run(ctx, experiment.PinnedEnvironment(), id, []byte("bytes")); finished <- err }()
	select {
	case <-f.entered:
	case <-ctx.Done():
		t.Fatal("run did not enter")
	}
	if _, err := g.Run(context.Background(), experiment.PinnedEnvironment(), id, nil); !errors.Is(err, ErrBusy) {
		t.Fatal("concurrent cached operation admitted")
	}
	unblock()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestCachedCancellationDuringIndependentCleanup(t *testing.T) {
	g, f, id := cachedFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.cancelCleanup = cancel
	if _, err := g.Run(ctx, experiment.PinnedEnvironment(), id, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cleanup cancellation escaped", err)
	}
	if len(f.data) != 1 {
		t.Fatal("independent point cleanup did not complete")
	}
	if _, err := g.Run(context.Background(), experiment.PinnedEnvironment(), id, nil); err == nil {
		t.Fatal("canceled cached adapter reused")
	}
	if err := g.Remove(context.Background(), id); err != nil {
		t.Fatal("owned artifact cleanup refused", err)
	}
}
