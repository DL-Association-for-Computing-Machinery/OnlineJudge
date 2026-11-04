package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

func staticRangeSumExpectedStderrFailure(err error) bool {
	d := execution.FailureDetails(err)
	return d != nil && !d.Truncated && len(d.Entries) == 1 && d.Entries[0].Phase == "validate" && d.Entries[0].Role == "stdout" && d.Entries[0].Code == "invalid_evidence"
}

type staticRangeSumProbeFixture struct {
	host    referenceHost
	cleanup string
}

func (f *staticRangeSumProbeFixture) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == "DELETE" && f.host.requests > 1 {
		stdout := strings.HasPrefix(req.URL.Path, "/file/stdout-")
		input := strings.HasPrefix(req.URL.Path, "/file/upload-")
		if f.cleanup == "stdout" && stdout || f.cleanup == "input" && input {
			return nil, errors.New("synthetic probe cleanup failure")
		}
	}
	resp, err := f.host.RoundTrip(req)
	if err != nil || req.URL.Path != "/run" || f.host.requests < 2 {
		return resp, err
	}
	if err := resp.Body.Close(); err != nil {
		return nil, err
	}
	row := map[string]any{"status": "Output Limit Exceeded", "error": "Output Limit Exceeded", "exitStatus": 0, "time": 1, "memory": 1,
		"files":     map[string]string{"stderr": strings.Repeat("e", 4097)},
		"fileIds":   map[string]string{"stdout": "stdout-" + strconv.Itoa(f.host.requests)},
		"fileError": []map[string]string{{"name": "stderr", "type": "CollectSizeExceeded"}}}
	raw, err := json.Marshal([]any{row})
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw))}, nil
}

func TestStaticRangeSumExpectedRejectionCannotHideCleanupFailure(t *testing.T) {
	for _, cleanup := range []string{"none", "stdout", "input"} {
		t.Run(cleanup, func(t *testing.T) {
			ctx := context.Background()
			f := &staticRangeSumProbeFixture{cleanup: cleanup}
			c, err := experiment.ResolveReferenceTier(experiment.ReferenceStaticRangeSum)
			if err != nil {
				t.Fatal(err)
			}
			g, err := execution.NewReferenceGoJudgeForTier(f, func(context.Context, experiment.ExecutionProfile) error { return nil }, &f.host, experiment.ReferenceStaticRangeSum)
			if err != nil {
				t.Fatal(err)
			}
			if err := g.Verify(ctx, c.Runtime); err != nil {
				t.Fatal(err)
			}
			o, err := g.Compile(ctx, c.Runtime, "source")
			if err != nil {
				t.Fatal(err)
			}
			_, runErr := g.Run(ctx, c.Runtime, o.Artifact, []byte("input"))
			if runErr == nil || staticRangeSumExpectedStderrFailure(runErr) != (cleanup == "none") {
				t.Fatal("expected rejection hid a point cleanup failure")
			}
			d := execution.FailureDetails(runErr)
			if cleanup != "none" && (d == nil || len(d.Entries) != 2 || d.Entries[1].Phase != "delete" || d.Entries[1].Role != cleanup) {
				t.Fatal("joined point cleanup evidence lost")
			}
			if staticRangeSumExpectedStderrFailure(errors.Join(runErr, context.Canceled)) {
				t.Fatal("expected rejection hid independent cancellation")
			}
			if cleanup == "none" {
				many := make([]error, 9)
				for i := range many {
					many[i] = runErr
				}
				joined := errors.Join(many...)
				if d := execution.FailureDetails(joined); d == nil || !d.Truncated || staticRangeSumExpectedStderrFailure(joined) {
					t.Fatal("truncated rejection evidence admitted")
				}
			}
			if err := g.Remove(ctx, o.Artifact); err != nil {
				t.Fatal(err)
			}
		})
	}
	if staticRangeSumExpectedStderrFailure(nil) || staticRangeSumExpectedStderrFailure(errors.New("synthetic unknown")) {
		t.Fatal("missing expected rejection admitted")
	}
}
