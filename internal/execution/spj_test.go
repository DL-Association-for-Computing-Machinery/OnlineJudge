package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

type spjCacheFixture struct {
	*cacheFixture
	status string
	calls  int
}

func (f *spjCacheFixture) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path != "/run" {
		return f.cacheFixture.RoundTrip(r)
	}
	f.calls++
	var body struct {
		Cmd []struct {
			Args   []string
			CopyIn map[string]struct{ FileID string }
		}
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Cmd) != 1 {
		f.t.Fatal("invalid checker request")
	}
	cmd := body.Cmd[0]
	args := experiment.SPJArgs()
	if !reflect.DeepEqual(cmd.Args, args[:]) || len(cmd.CopyIn) != 4 {
		f.t.Fatal("unexpected checker arguments or materials")
	}
	for _, name := range args {
		if name == "./checker" {
			name = "checker"
		}
		if string(f.data[cmd.CopyIn[name].FileID]) != name {
			f.t.Fatalf("wrong private material for %s", name)
		}
	}
	raw, _ := json.Marshal([]any{map[string]any{"status": f.status, "exitStatus": 50,
		"time": 1, "memory": 1, "files": map[string]string{"stdout": "", "stderr": ""}}})
	return response(string(raw)), nil
}

func TestSPJCacheRefusesBadReadbackAndCannotPassOnExecutorOrCleanupFailure(t *testing.T) {
	for _, tc := range []struct {
		name, fail, status string
		ok                 bool
		calls              int
	}{
		{"valid", "", "Nonzero Exit Status", true, 1},
		{"readback mismatch", "hash", "Nonzero Exit Status", false, 0},
		{"cleanup failure", "delete", "Nonzero Exit Status", false, 1},
		{"timeout with pass exit", "", "Time Limit Exceeded", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &spjCacheFixture{cacheFixture: &cacheFixture{t: t, fail: tc.fail, data: map[string][]byte{}}, status: tc.status}
			verify := func(context.Context, experiment.ExecutionProfile) error { return nil }
			g := NewReferenceGoJudge(f, verify, f)
			g.runtime, g.limits = experiment.PythonTextBase(g.runtime), experiment.SPJFiles()
			ctx := t.Context()
			if err := g.Verify(ctx, g.runtime); err != nil {
				t.Fatal(err)
			}
			ok, err := g.checkSPJ(ctx, []byte("checker"), []byte("input"), []byte("output"), []byte("answer"))
			if ok != tc.ok || (err == nil) != tc.ok || f.calls != tc.calls {
				t.Fatalf("verdict=%v err=%v calls=%d", ok, err, f.calls)
			}
			if tc.fail != "delete" && len(f.data) != 0 {
				t.Fatal("checker objects leaked")
			}
		})
	}
}
