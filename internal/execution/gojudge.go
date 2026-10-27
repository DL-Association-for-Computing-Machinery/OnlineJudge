package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// GoJudge adapts the fixed v1.13.3 REST contract. The endpoint is container-local;
// an injected transport can drive the dedicated offline VM without exposing ports.
type GoJudge struct {
	client http.Client
	verify func(context.Context, experiment.ExecutionProfile) error
}

// NewGoJudge requires a trusted host inspector for image, kernel, compiler and
// outer container isolation. REST self-reports alone cannot establish identity.
// No default network transport is installed: callers must explicitly provide it.
func NewGoJudge(transport http.RoundTripper, verify func(context.Context, experiment.ExecutionProfile) error) *GoJudge {
	return &GoJudge{client: http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("sandbox redirect refused") }}, verify: verify}
}

// Verify checks host identity and the executor's version and isolation features.
func (g *GoJudge) Verify(ctx context.Context, p experiment.ExecutionProfile) error {
	if g.verify == nil || g.client.Transport == nil {
		return errors.New("sandbox inspector and transport required")
	}
	if err := g.verify(ctx, p); err != nil {
		return err
	}
	var version struct {
		Build    string `json:"buildVersion"`
		OS       string `json:"os"`
		Platform string `json:"platform"`
	}
	if err := g.request(ctx, "GET", "/version", nil, &version); err != nil {
		return err
	}
	if version.Build != p.SandboxVersion || version.OS+"/"+version.Platform != p.Platform {
		return errors.New("sandbox version mismatch")
	}
	var config struct {
		Runner struct {
			Cgroup      int      `json:"cgroupType"`
			Controllers []string `json:"cgroupControllers"`
			Seccomp     bool     `json:"seccomp"`
		} `json:"runnerConfig"`
	}
	if err := g.request(ctx, "GET", "/config", nil, &config); err != nil {
		return err
	}
	controllers := map[string]bool{}
	for _, c := range config.Runner.Controllers {
		controllers[c] = true
	}
	if config.Runner.Cgroup != 2 || !config.Runner.Seccomp || !controllers["memory"] || !controllers["pids"] || !controllers["cpu"] {
		return errors.New("sandbox isolation unavailable")
	}
	return nil
}

// Compile sends only source, never host paths or reference answers.
func (g *GoJudge) Compile(ctx context.Context, p experiment.ExecutionProfile, source string) (Observation, error) {
	c := command(p, p.CompileArgs, p.Compile, nil)
	c["copyIn"] = map[string]any{"main.cpp": map[string]any{"content": source}}
	c["copyOutCached"] = []string{"main?"}
	c["copyOutMax"] = p.ExecutableBytes
	return g.execute(ctx, c, p, true)
}

// Run copies the attempt-local executable into a fresh sandbox for one point.
func (g *GoJudge) Run(ctx context.Context, p experiment.ExecutionProfile, id string, input []byte) (Observation, error) {
	if !artifactID.MatchString(id) {
		return Observation{}, errors.New("invalid artifact ID")
	}
	c := command(p, p.RunArgs, p.Run, input)
	c["copyIn"] = map[string]any{"main": map[string]any{"fileId": id}}
	return g.execute(ctx, c, p, false)
}

// Remove deletes only the executable returned for this attempt.
func (g *GoJudge) Remove(ctx context.Context, id string) error {
	if !artifactID.MatchString(id) {
		return errors.New("invalid artifact ID")
	}
	return g.request(ctx, "DELETE", "/file/"+id, nil, nil)
}

var artifactID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func command(p experiment.ExecutionProfile, args []string, l experiment.Limits, input []byte) map[string]any {
	return map[string]any{
		"args": args, "env": p.Env,
		"files":    []any{map[string]any{"content": string(input)}, map[string]any{"name": "stdout", "max": p.OutputBytes}, map[string]any{"name": "stderr", "max": p.OutputBytes}},
		"cpuLimit": l.CPU, "clockLimit": l.Wall, "memoryLimit": l.Memory, "procLimit": l.Processes,
	}
}

func (g *GoJudge) execute(ctx context.Context, c map[string]any, p experiment.ExecutionProfile, compile bool) (Observation, error) {
	var results []struct {
		Status     string            `json:"status"`
		Exit       *int              `json:"exitStatus"`
		CPU        *int64            `json:"time"`
		Memory     *int64            `json:"memory"`
		Files      map[string]string `json:"files"`
		IDs        map[string]string `json:"fileIds"`
		Error      string            `json:"error"`
		FileErrors []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"fileError"`
	}
	if err := g.request(ctx, "POST", "/run", map[string]any{"cmd": []any{c}}, &results); err != nil {
		return Observation{}, err
	}
	if len(results) != 1 {
		return Observation{}, errors.New("invalid sandbox result count")
	}
	r := results[0]
	o := Observation{Status: r.Status, Stdout: []byte(r.Files["stdout"]), Artifact: r.IDs["main"]}
	// go-judge keeps max+1 bytes on overflow. JSON can replace each invalid UTF-8
	// byte with a three-byte U+FFFD. The fixed numeric M0 answers cannot match that
	// replacement; this text transport is not a general byte-preserving checker.
	if r.Exit == nil || r.CPU == nil || r.Memory == nil || r.Error != "" ||
		len(r.Files["stdout"]) > 3*(p.OutputBytes+1) || len(r.Files["stderr"]) > 3*(p.OutputBytes+1) ||
		(o.Artifact != "" && (!compile || !artifactID.MatchString(o.Artifact))) || len(r.IDs) > 1 {
		return o, fmt.Errorf("invalid sandbox evidence (stdout=%d, stderr=%d, file_errors=%d)", len(r.Files["stdout"]), len(r.Files["stderr"]), len(r.FileErrors))
	}
	for _, e := range r.FileErrors {
		if r.Status != "Output Limit Exceeded" || e.Type != "CollectSizeExceeded" ||
			(e.Name != "stdout" && !(compile && e.Name == "stderr")) {
			return o, errors.New("sandbox file operation failed")
		}
	}
	if r.Status == "Output Limit Exceeded" && len(r.FileErrors) == 0 {
		return o, errors.New("missing output overflow evidence")
	}
	o.OutputTruncated = len(r.FileErrors) > 0
	if r.Status == "Accepted" {
		limits := p.Run
		if compile {
			limits = p.Compile
		}
		if *r.CPU > limits.CPU || *r.Memory > limits.Memory {
			return o, errors.New("accepted result contradicts resource evidence")
		}
		if _, ok := r.Files["stdout"]; !ok {
			return o, errors.New("sandbox stdout missing")
		}
	}
	o.Exit = *r.Exit
	o.CPU = *r.CPU
	o.Memory = *r.Memory
	return o, nil
}

func (g *GoJudge) request(ctx context.Context, method, path string, value, target any) error {
	if g.client.Transport == nil {
		return errors.New("sandbox transport required")
	}
	var body bytes.Buffer
	if value != nil {
		if err := json.NewEncoder(&body).Encode(value); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:5050"+path, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return errors.New("sandbox transport failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024+1))
	if err != nil || len(data) > 128*1024 || resp.StatusCode != http.StatusOK {
		return errors.New("sandbox response unavailable or oversized")
	}
	if target == nil {
		return nil
	}
	return json.Unmarshal(data, target)
}
