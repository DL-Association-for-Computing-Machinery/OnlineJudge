//go:build sandbox

package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

const container = "oj-m0-sandbox"

type outcome struct {
	Status string            `json:"status"`
	Exit   int               `json:"exitStatus"`
	Time   int64             `json:"time"`
	Memory int64             `json:"memory"`
	Files  map[string]string `json:"files"`
	IDs    map[string]string `json:"fileIds"`
}

func api(t *testing.T, method, path string, value any) []byte {
	t.Helper()
	if os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" {
		t.Fatal("explicit OJ_SANDBOX_EXPERIMENT=1 required")
	}
	// Fixed dedicated VM context: never follows the user's active Docker context.
	args := []string{"--context", "colima-oj-m0", "exec", "-i", container, "curl", "-fsS", "--max-time", "20", "-X", method, "-H", "Content-Type: application/json"}
	var data []byte
	if value != nil {
		var err error
		data, err = json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		args = append(args, "--data-binary", "@-")
	}
	args = append(args, "http://127.0.0.1:5050"+path)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", args...)
	command.Stdin = bytes.NewReader(data)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("sandbox request failed: %v: %s", err, output)
	}
	return output
}

func execute(t *testing.T, command map[string]any) outcome {
	t.Helper()
	var results []outcome
	if err := json.Unmarshal(api(t, "POST", "/run", map[string]any{"cmd": []any{command}}), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatal("invalid result count")
	}
	return results[0]
}

func base(args []string, input string) map[string]any {
	return map[string]any{
		"args": args, "env": []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C", "LD_LIBRARY_PATH=/usr/local/lib64"},
		"files":    []any{map[string]any{"content": input}, map[string]any{"name": "stdout", "max": 4096}, map[string]any{"name": "stderr", "max": 4096}},
		"cpuLimit": 200000000, "clockLimit": 2000000000, "memoryLimit": 32 << 20, "procLimit": 16,
	}
}

func compile(t *testing.T, source string) (string, outcome) {
	t.Helper()
	command := base([]string{"/usr/local/bin/g++", "-std=gnu++17", "-O2", "-pipe", "main.cpp", "-o", "main"}, "")
	command["cpuLimit"] = 10000000000
	command["clockLimit"] = 15000000000
	command["memoryLimit"] = 512 << 20
	command["copyIn"] = map[string]any{"main.cpp": map[string]any{"content": source}}
	command["copyOutCached"] = []string{"main?"}
	command["copyOutMax"] = 1 << 20
	result := execute(t, command)
	id := result.IDs["main"]
	if id != "" {
		t.Cleanup(func() { api(t, "DELETE", "/file/"+id, nil) })
	}
	return id, result
}

func run(t *testing.T, id, input string) outcome {
	t.Helper()
	command := base([]string{"main"}, input)
	command["copyIn"] = map[string]any{"main": map[string]any{"fileId": id}}
	return execute(t, command)
}

func TestSandbox(t *testing.T) {
	var config struct {
		Runner struct {
			CgroupType  int      `json:"cgroupType"`
			Controllers []string `json:"cgroupControllers"`
			Seccomp     bool     `json:"seccomp"`
		} `json:"runnerConfig"`
	}
	if err := json.Unmarshal(api(t, "GET", "/config", nil), &config); err != nil {
		t.Fatal(err)
	}
	controllers := strings.Join(config.Runner.Controllers, ",")
	if config.Runner.CgroupType != 2 || !config.Runner.Seccomp || !strings.Contains(controllers, "memory") || !strings.Contains(controllers, "pids") {
		t.Fatal("required isolation unavailable")
	}
	t.Logf("sandbox version: %s", api(t, "GET", "/version", nil))

	t.Run("GNU++17 compile once and ordered text tests", func(t *testing.T) {
		id, result := compile(t, "#include <iostream>\nint main(){long long a,b;std::cin>>a>>b;std::cout<<a+b<<'\\n';}")
		if result.Status != "Accepted" || id == "" {
			t.Fatalf("compile: %+v", result)
		}
		for _, point := range [][2]string{{"1 2\n", "3\n"}, {"-7 4\n", "-3\n"}} {
			result := run(t, id, point[0])
			ok, err := judging.CompareText([]byte(point[1]), []byte(result.Files["stdout"]), false)
			if result.Status != "Accepted" || result.Exit != 0 || err != nil || !ok {
				t.Fatalf("point: %+v compare=%v", result, err)
			}
			t.Logf("AC point: cpu=%dns memory=%d bytes", result.Time, result.Memory)
		}
	})
	t.Run("normal exit with wrong output is WA", func(t *testing.T) {
		id, result := compile(t, "#include <cstdio>\nint main(){puts(\"wrong\");}")
		if result.Status != "Accepted" {
			t.Fatalf("compile: %+v", result)
		}
		result = run(t, id, "1 2\n")
		ok, err := judging.CompareText([]byte("3\n"), []byte(result.Files["stdout"]), false)
		if result.Status != "Accepted" || err != nil || ok {
			t.Fatalf("wrong output accepted: %+v", result)
		}
	})
	t.Run("compile failure", func(t *testing.T) {
		_, result := compile(t, "this is invalid C++")
		if result.Status != "Nonzero Exit Status" {
			t.Fatalf("CE evidence: %+v", result)
		}
	})
	cases := []struct{ name, source, status string }{
		{"RE", "int main(){return 1;}", "Nonzero Exit Status"},
		{"TLE", "int main(){for(;;){asm volatile(\"\");}}", "Time Limit Exceeded"},
		{"MLE", "#include <cstdlib>\nint main(){volatile char *p=(char*)malloc(128*1024*1024);if(!p)return 2;for(long i=0;i<128*1024*1024;i+=4096)p[i]=1;for(;;){asm volatile(\"\");}}", "Memory Limit Exceeded"},
		{"OLE", "#include <cstdio>\nint main(){for(;;)puts(\"unbounded output fixture\");}", "Output Limit Exceeded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, result := compile(t, tc.source)
			if result.Status != "Accepted" {
				t.Fatalf("compile: %+v", result)
			}
			result = run(t, id, "")
			if result.Status != tc.status {
				t.Fatalf("wanted %s: %+v", tc.status, result)
			}
			t.Logf("%s: cpu=%dns memory=%d bytes", tc.name, result.Time, result.Memory)
		})
	}
	t.Run("missing executor is system evidence", func(t *testing.T) {
		result := execute(t, base([]string{"/missing-executable"}, ""))
		if result.Status != "Internal Error" {
			t.Fatalf("missing executable: %+v", result)
		}
	})
	t.Run("cache and node filesystem inaccessible", func(t *testing.T) {
		source := `#include <cstdio>
int main(){const char* paths[]={"/opt/go-judge","/dev/shm/go-judge","/root/.ssh/id_rsa","/Users"};
for(auto p:paths){FILE*f=fopen(p,"r");if(f){fclose(f);return 1;}}return 0;}`
		id, result := compile(t, source)
		if result.Status != "Accepted" {
			t.Fatalf("compile: %+v", result)
		}
		result = run(t, id, "")
		if result.Status != "Accepted" || result.Exit != 0 {
			t.Fatalf("filesystem isolation: %+v", result)
		}
	})
	t.Run("per execution temporary files are cleared", func(t *testing.T) {
		id, result := compile(t, `#include <cstdio>
int main(){FILE*f=fopen("/tmp/oj-previous-point","r");if(f){fclose(f);return 1;} f=fopen("/tmp/oj-previous-point","w");if(!f)return 2;fputs("marker",f);fclose(f);}`)
		if result.Status != "Accepted" {
			t.Fatalf("compile: %+v", result)
		}
		for range 2 {
			result = run(t, id, "")
			if result.Status != "Accepted" || result.Exit != 0 {
				t.Fatalf("fresh environment: %+v", result)
			}
		}
	})
	t.Run("sandbox cannot connect to node service", func(t *testing.T) {
		source := `#include <sys/socket.h>
#include <netinet/in.h>
#include <unistd.h>
int main(){int fd=socket(AF_INET,SOCK_STREAM,0);if(fd<0)return 0;sockaddr_in a{};a.sin_family=AF_INET;a.sin_port=htons(5050);a.sin_addr.s_addr=htonl(0x7f000001);int r=connect(fd,(sockaddr*)&a,sizeof(a));close(fd);return r==0?1:0;}`
		id, result := compile(t, source)
		if result.Status != "Accepted" {
			t.Fatalf("compile: %+v", result)
		}
		result = run(t, id, "")
		if result.Status != "Dangerous Syscall" && (result.Status != "Accepted" || result.Exit != 0) {
			t.Fatalf("network isolation: %+v", result)
		}
	})
	t.Run("file cache empty after fixture cleanup", func(t *testing.T) {
		var cache map[string]string
		if err := json.Unmarshal(api(t, "GET", "/file", nil), &cache); err != nil {
			t.Fatal(err)
		}
		if len(cache) != 0 {
			t.Fatal(fmt.Sprintf("cache leaked %d entries", len(cache)))
		}
	})
}
