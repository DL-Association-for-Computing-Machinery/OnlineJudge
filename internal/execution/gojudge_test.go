package execution

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

const success = `[{"status":"Accepted","exitStatus":0,"time":12,"memory":32,"files":{"stdout":"3\n","stderr":""},"fileIds":{}}]`

func TestGoJudgeBoundedCommands(t *testing.T) {
	p := experiment.PinnedEnvironment()
	var commands []map[string]any
	g := NewGoJudge(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "127.0.0.1:5050" || r.URL.Path != "/run" || r.Method != "POST" {
			t.Fatalf("unexpected target %s", r.URL)
		}
		var request struct {
			Cmd []map[string]any `json:"cmd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Cmd) != 1 {
			t.Fatal("invalid command envelope")
		}
		commands = append(commands, request.Cmd[0])
		return response(success), nil
	}), nil)
	if _, err := g.Compile(context.Background(), p, "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Run(context.Background(), p, "artifact", []byte("input")); err != nil {
		t.Fatal(err)
	}
	for i, c := range commands {
		l := p.Run
		args := p.RunArgs
		if i == 0 {
			l = p.Compile
			args = p.CompileArgs
		}
		encoded, _ := json.Marshal(args)
		got, _ := json.Marshal(c["args"])
		if string(encoded) != string(got) || c["cpuLimit"] != float64(l.CPU) || c["clockLimit"] != float64(l.Wall) || c["memoryLimit"] != float64(l.Memory) || c["procLimit"] != float64(l.Processes) {
			t.Fatalf("limits changed: %v", c)
		}
		files := c["files"].([]any)
		for _, f := range files[1:] {
			if f.(map[string]any)["max"] != float64(p.OutputBytes) {
				t.Fatal("unbounded output")
			}
		}
		copyIn := c["copyIn"].(map[string]any)
		if len(copyIn) != 1 {
			t.Fatal("unexpected material exposed")
		}
	}
	if commands[0]["copyOutMax"] != float64(p.ExecutableBytes) || commands[1]["copyOutCached"] != nil {
		t.Fatal("artifact lifetime changed")
	}
}

func TestGoJudgeRejectsInvalidResponses(t *testing.T) {
	for _, body := range []string{
		`[]`, `[{},{}]`, `[{"status":"Accepted"}]`, success + `{}`,
		strings.Repeat(" ", 128*1024+1),
		strings.Replace(success, `"stdout":"3\n"`, `"other":"3\n"`, 1),
		strings.Replace(success, `"fileIds":{}`, `"fileIds":{"main":"../../other"}`, 1),
		strings.Replace(success, `"fileIds":{}`, `"fileError":[{"type":"CopyOut"}]`, 1),
		strings.Replace(success, `"stderr":""`, `"stderr":"`+strings.Repeat("a", 3*4097+1)+`"`, 1),
		strings.Replace(success, `"time":12`, `"time":10000000001`, 1),
		strings.Replace(success, `"memory":32`, `"memory":536870913`, 1),
	} {
		g := NewGoJudge(transportFunc(func(*http.Request) (*http.Response, error) { return response(body), nil }), nil)
		if _, err := g.Compile(context.Background(), experiment.PinnedEnvironment(), "source"); err == nil {
			t.Fatalf("accepted invalid response %.80s", body)
		}
	}
	for _, status := range []int{302, 404, 500} {
		g := NewGoJudge(transportFunc(func(*http.Request) (*http.Response, error) {
			r := response(success)
			r.StatusCode = status
			return r, nil
		}), nil)
		if _, err := g.Run(context.Background(), experiment.PinnedEnvironment(), "artifact", nil); err == nil {
			t.Fatalf("accepted HTTP %d", status)
		}
	}
}

func TestGoJudgeVerification(t *testing.T) {
	p := experiment.PinnedEnvironment()
	for _, bad := range []string{"", "inspector", "version", "cgroup", "seccomp", "controllers"} {
		t.Run(bad, func(t *testing.T) {
			var paths []string
			g := NewGoJudge(transportFunc(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				body := `{"buildVersion":"v1.13.3","os":"linux","platform":"arm64"}`
				if r.URL.Path == "/version" && bad == "version" {
					body = `{}`
				}
				if r.URL.Path == "/config" {
					body = `{"runnerConfig":{"cgroupType":2,"seccomp":true,"cgroupControllers":["cpu","memory","pids"]}}`
					switch bad {
					case "cgroup":
						body = strings.Replace(body, ":2", ":1", 1)
					case "seccomp":
						body = strings.Replace(body, "true", "false", 1)
					case "controllers":
						body = strings.Replace(body, `"memory"`, `"memory-other"`, 1)
					}
				}
				return response(body), nil
			}), func(context.Context, experiment.ExecutionProfile) error {
				if bad == "inspector" {
					return errors.New("identity mismatch")
				}
				return nil
			})
			err := g.Verify(context.Background(), p)
			if (err != nil) != (bad != "") {
				t.Fatal(err)
			}
			if bad == "inspector" && len(paths) != 0 {
				t.Fatal("identity failure ignored")
			}
			if bad == "" && !reflect.DeepEqual(paths, []string{"/version", "/config"}) {
				t.Fatal(paths)
			}
		})
	}
	if err := NewGoJudge(nil, nil).Verify(context.Background(), p); err == nil {
		t.Fatal("missing inspector allowed")
	}
}

func TestRemoveAndPartialArtifact(t *testing.T) {
	g := NewGoJudge(transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == "DELETE" {
			if r.URL.Path != "/file/artifact" {
				t.Fatal(r.URL)
			}
			return response(""), nil
		}
		return response(`[{"status":"Accepted","fileIds":{"main":"artifact"}}]`), nil
	}), nil)
	o, err := g.Compile(context.Background(), experiment.PinnedEnvironment(), "source")
	if err == nil || o.Artifact != "artifact" {
		t.Fatal("partial artifact lost")
	}
	if err := g.Remove(context.Background(), o.Artifact); err != nil {
		t.Fatal(err)
	}
	if err := g.Remove(context.Background(), "../other"); err == nil {
		t.Fatal("unsafe removal")
	}
}

func TestOutputOverflowEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, status, kind, file string
		ok                       bool
	}{
		{"stdout overflow", "Output Limit Exceeded", "CollectSizeExceeded", "stdout", true},
		{"accepted with overflow", "Accepted", "CollectSizeExceeded", "stdout", false},
		{"copy failure", "Output Limit Exceeded", "CopyOutOpen", "stdout", false},
		{"stderr only", "Output Limit Exceeded", "CollectSizeExceeded", "stderr", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal([]any{map[string]any{"status": tc.status, "exitStatus": 9, "time": 12, "memory": 32, "files": map[string]string{"stdout": strings.Repeat("a", 4097), "stderr": ""}, "fileError": []any{map[string]string{"name": tc.file, "type": tc.kind}}}})
			g := NewGoJudge(transportFunc(func(*http.Request) (*http.Response, error) { return response(string(body)), nil }), nil)
			_, err := g.Run(context.Background(), experiment.PinnedEnvironment(), "artifact", nil)
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected overflow handling: %v", err)
			}
		})
	}
}
