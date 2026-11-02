package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestGoJudgeFileSizeEvidence(t *testing.T) {
	p := experiment.PinnedEnvironment()
	for _, tc := range []struct {
		name     string
		mutate   func(map[string]any)
		compile  bool
		ok, trim bool
	}{
		{name: "SIGXFSZ", ok: true},
		{name: "empty file errors", ok: true, mutate: func(r map[string]any) { r["fileError"] = []any{} }},
		{name: "UTF8 replacement bound", ok: true, mutate: func(r map[string]any) {
			r["files"].(map[string]any)["stdout"] = strings.Repeat("\uFFFD", p.OutputBytes)
		}},
		{name: "compile unsupported", compile: true},
		{name: "exit zero", mutate: func(r map[string]any) { r["exitStatus"] = 0 }},
		{name: "different signal", mutate: func(r map[string]any) { r["exitStatus"] = 9 }},
		{name: "shell signal convention", mutate: func(r map[string]any) { r["exitStatus"] = 153 }},
		{name: "missing exit", mutate: func(r map[string]any) { delete(r, "exitStatus") }},
		{name: "missing CPU", mutate: func(r map[string]any) { delete(r, "time") }},
		{name: "missing memory", mutate: func(r map[string]any) { delete(r, "memory") }},
		{name: "negative CPU", mutate: func(r map[string]any) { r["time"] = -1 }},
		{name: "negative memory", mutate: func(r map[string]any) { r["memory"] = -1 }},
		{name: "missing stdout", mutate: func(r map[string]any) { delete(r["files"].(map[string]any), "stdout") }},
		{name: "missing stderr", mutate: func(r map[string]any) { delete(r["files"].(map[string]any), "stderr") }},
		{name: "null stdout", mutate: func(r map[string]any) { r["files"].(map[string]any)["stdout"] = nil }},
		{name: "null stderr", mutate: func(r map[string]any) { r["files"].(map[string]any)["stderr"] = nil }},
		{name: "no streams", mutate: func(r map[string]any) { delete(r, "files") }},
		{name: "error text is not evidence", mutate: func(r map[string]any) { r["error"] = "SIGXFSZ: file size limit" }},
		{name: "main artifact", mutate: func(r map[string]any) { r["fileIds"] = map[string]string{"main": "artifact"} }},
		{name: "unknown artifact", mutate: func(r map[string]any) { r["fileIds"] = map[string]string{"unexpected": "artifact"} }},
		{name: "empty unknown artifact", mutate: func(r map[string]any) { r["fileIds"] = map[string]string{"unexpected": ""} }},
		{name: "stdout too long", mutate: func(r map[string]any) {
			r["files"].(map[string]any)["stdout"] = strings.Repeat("x", 3*p.OutputBytes+1)
		}},
		{name: "stderr too long", mutate: func(r map[string]any) {
			r["files"].(map[string]any)["stderr"] = strings.Repeat("x", 3*p.OutputBytes+1)
		}},
		{name: "copy failure", mutate: func(r map[string]any) {
			r["fileError"] = []any{map[string]string{"name": "stdout", "type": "CopyOutOpen"}}
		}},
		{name: "stderr collector unsupported", mutate: func(r map[string]any) {
			r["fileError"] = []any{map[string]string{"name": "stderr", "type": "CollectSizeExceeded"}}
		}},
		{name: "stdout collector retains truncation", ok: true, trim: true, mutate: func(r map[string]any) {
			r["files"].(map[string]any)["stdout"] = strings.Repeat("x", p.OutputBytes+1)
			r["fileError"] = []any{map[string]string{"name": "stdout", "type": "CollectSizeExceeded"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := map[string]any{"status": "Output Limit Exceeded", "exitStatus": 25, "time": 12, "memory": 32,
				"files": map[string]any{"stdout": "", "stderr": ""}}
			if tc.mutate != nil {
				tc.mutate(r)
			}
			body, err := json.Marshal([]any{r})
			if err != nil {
				t.Fatal(err)
			}
			g := NewGoJudge(transportFunc(func(*http.Request) (*http.Response, error) { return response(string(body)), nil }), nil)
			var got Observation
			if tc.compile {
				got, err = g.Compile(context.Background(), p, "source")
			} else {
				got, err = g.Run(context.Background(), p, "artifact", nil)
			}
			if !tc.ok {
				if err == nil {
					t.Fatalf("invalid file-size response accepted: %+v", got)
				}
				return
			}
			if err != nil || got.Status != "Output Limit Exceeded" || got.Exit != 25 || got.CPU != 12 || got.Memory != 32 || got.Artifact != "" || got.OutputTruncated != tc.trim {
				t.Fatalf("file-size evidence: status=%s exit=%d truncated=%v stdout=%d error=%v", got.Status, got.Exit, got.OutputTruncated, len(got.Stdout), err)
			}
		})
	}
}

func TestSignalNumberDoesNotSelectOutputVerdict(t *testing.T) {
	for _, tc := range []struct{ status, verdict string }{
		{"Nonzero Exit Status", "RE"}, {"Signalled", "RE"}, {"Accepted", "SE"}, {"unknown", "SE"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			body, err := json.Marshal([]any{map[string]any{"status": tc.status, "exitStatus": 25, "time": 12, "memory": 32,
				"files": map[string]string{"stdout": "", "stderr": ""}}})
			if err != nil {
				t.Fatal(err)
			}
			g := NewGoJudge(transportFunc(func(*http.Request) (*http.Response, error) { return response(string(body)), nil }), nil)
			got, err := g.Run(context.Background(), experiment.PinnedEnvironment(), "artifact", nil)
			if err != nil {
				t.Fatal(err)
			}
			verdict, err := classify(got, false)
			if verdict != tc.verdict || (err != nil) != (tc.verdict == "SE") || got.OutputTruncated {
				t.Fatalf("exit25 changed unrelated status semantics: %s error=%v", verdict, err)
			}
		})
	}
}

func TestFileSizeOutcomeAndEvidence(t *testing.T) {
	d, blobs := fixture()
	d.SubmissionID = "file-size-submission"
	s := goodSandbox()
	s.runs = append([]Observation{{Status: "Output Limit Exceeded", Exit: 25, CPU: 12, Memory: 32}}, s.runs...)
	runner := New(s)
	r, err := runner.Judge(context.Background(), d, blobs)
	if err != nil || r.Verdict != "OLE" || r.Points[0].Verdict != "OLE" || r.Points[0].OutputTruncated || r.Points[1].Verdict != "NOT_RUN" || r.Points[1].Usage != nil {
		t.Fatalf("file-size attempt: %+v error=%v", r, err)
	}
	if len(s.calls) != 4 || s.calls[3] != "remove:artifact" {
		t.Fatalf("attempt did not stop after first point/cleanup: %v", s.calls)
	}
	l := submission.Lease{SubmissionID: d.SubmissionID, JudgeGeneration: 1, RecoveryGeneration: 1, AttemptNumber: 1}
	frozen := freezeReport(l, d, r)
	if frozen.Verdict != "OLE" || len(frozen.EvidenceSHA256) != 64 {
		t.Fatal("OLE evidence not frozen")
	}
	r.Points[0].OutputTruncated = true
	if freezeReport(l, d, r) == frozen {
		t.Fatal("truncation flag not bound to frozen evidence")
	}
	r, err = runner.Judge(context.Background(), d, blobs)
	if err != nil || r.Verdict != "AC" {
		t.Fatalf("same execution slot was not reusable: %+v error=%v", r, err)
	}
}
