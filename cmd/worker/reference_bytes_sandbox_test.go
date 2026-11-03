//go:build sandbox

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestReferenceBytesSandbox(t *testing.T) {
	if os.Getenv("OJ_REFERENCE_BYTES") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit dedicated Desktop byte experiment required")
	}
	cap := experiment.PinnedReferenceFiles().FileBytes
	pattern := make([]byte, 256)
	for i := range pattern {
		pattern[i] = byte(i)
	}
	data := bytes.Repeat(pattern, int(cap)/len(pattern))
	t.Run("file-boundary", func(t *testing.T) {
		h := referenceByteHost(t)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		referenceStorageSample(t, h, "before-upload")
		id, err := h.UploadFile(ctx, data)
		if err != nil {
			t.Fatal("boundary upload", err)
		}
		referenceStorageSample(t, h, "after-upload")
		raw, err := h.ReadFile(ctx, id, cap)
		if err != nil || len(raw) != len(data) || sha256.Sum256(raw) != sha256.Sum256(data) {
			t.Fatal("boundary roundtrip failed", err)
		}
		if _, err := h.ReadFile(ctx, id, cap+1); err == nil {
			t.Fatal("download cap bypassed")
		}
		if _, err := h.UploadFile(ctx, append(data, 0)); err == nil {
			t.Fatal("upload cap bypassed")
		}
		if err := execution.NewGoJudge(h, h.Verify).Remove(ctx, id); err != nil {
			t.Fatal("owned probe cleanup", err)
		}
		referenceStorageSample(t, h, "after-delete")
		t.Logf("raw %d bytes SHA256=%x; exact boundary passed, over-boundary refused", len(data), sha256.Sum256(data))
	})
	t.Run("cached-stdin-stdout", func(t *testing.T) {
		h := referenceByteHost(t)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		files := &measuredReferenceFiles{host: h, t: t}
		g := execution.NewCachedGoJudge(h, h.Verify, files)
		p := experiment.PinnedEnvironment()
		if err := g.Verify(ctx, p); err != nil {
			t.Fatal(err)
		}
		o, err := g.Compile(ctx, p, `#include <cstdio>
#include <cstring>
int main(){unsigned char b[8192],want[8192];for(int i=0;i<8192;i++)want[i]=(unsigned char)i;
unsigned long long total=0;size_t n;while((n=fread(b,1,sizeof b,stdin))!=0){if(memcmp(b,want,n))return 7;total+=n;}
for(int i=0;i<256;i++)putchar(i);printf(":%llu\n200",total);}`)
		if err != nil || o.Artifact == "" {
			t.Fatal("byte probe compile", err)
		}
		for _, input := range [][]byte{nil, pattern, data} {
			observed, err := g.Run(ctx, p, o.Artifact, input)
			want := append(append([]byte(nil), pattern...), []byte(fmt.Sprintf(":%d\n200", len(input)))...)
			if err != nil || observed.Status != "Accepted" || observed.Exit != 0 || !bytes.Equal(observed.Stdout, want) {
				t.Fatalf("raw cached execution size=%d status=%s err=%v", len(input), observed.Status, err)
			}
			t.Logf("stdin=%d stdout=%d SHA256=%x CPU=%d memory=%d; all-byte pattern verified", len(input), len(observed.Stdout), sha256.Sum256(observed.Stdout), observed.CPU, observed.Memory)
		}
		if err := g.Remove(ctx, o.Artifact); err != nil {
			t.Fatal("byte executable cleanup", err)
		}
		referenceStorageSample(t, h, "cached-after-cleanup")
	})
}

func referenceByteHost(t *testing.T) *workerhost.Host {
	t.Helper()
	slot, err := workerhost.ParseM0Slot(os.Getenv("OJ_WORKER_M0_SLOT"))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := lockM0(slot)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.dirty(); err != nil {
		lock.Close()
		t.Fatal(err)
	}
	h, startErr := workerhost.StartM0Slot(context.Background(), slot)
	t.Cleanup(func() {
		defer lock.Close()
		if h == nil {
			t.Error("slot remains quarantined after unavailable host")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := h.Stop(ctx); err != nil {
			raw, _ := json.Marshal(workerhost.StopDetails(err))
			t.Errorf("byte experiment stop unconfirmed: %s", raw)
			return
		}
		if err := lock.clean(); err != nil {
			t.Error("byte marker cleanup failed")
		}
	})
	if startErr != nil {
		t.Fatal("byte slot start failed", startErr)
	}
	return h
}

type measuredReferenceFiles struct {
	host *workerhost.Host
	t    *testing.T
}

func (f *measuredReferenceFiles) UploadFile(ctx context.Context, data []byte) (string, error) {
	id, err := f.host.UploadFile(ctx, data)
	if err == nil {
		referenceStorageSample(f.t, f.host, fmt.Sprintf("uploaded-%d", len(data)))
	}
	return id, err
}
func (f *measuredReferenceFiles) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	raw, err := f.host.ReadFile(ctx, id, maximum)
	if err == nil {
		referenceStorageSample(f.t, f.host, fmt.Sprintf("read-%d", len(raw)))
	}
	return raw, err
}

// Read only numeric resource evidence from this already-owned lifetime. The
// commands finish synchronously before test cleanup can initiate Host.Stop.
func referenceStorageSample(t *testing.T, h *workerhost.Host, phase string) {
	t.Helper()
	id, _ := h.Identity()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "--context", "desktop-linux", "exec", id, "/bin/sh", "-ec", `stat -f -c '%S %b %a' /tmp /dev/shm; cat /sys/fs/cgroup/memory.max /sys/fs/cgroup/memory.peak`)
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal("resource sample unavailable")
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 8 {
		t.Fatal("unexpected numeric resource sample")
	}
	var n [8]uint64
	for i, value := range fields {
		n[i], err = strconv.ParseUint(value, 10, 64)
		if err != nil {
			t.Fatal("outer cgroup evidence unavailable")
		}
	}
	if n[2] > n[1] || n[5] > n[4] || n[6] != 1<<30 || n[7] == 0 || n[7] > n[6] {
		t.Fatal("outer memory budget mismatch")
	}
	t.Logf("resources %s: tmp-capacity=%d tmp-used=%d shm-capacity=%d shm-used=%d outer-memory-peak=%d", phase, n[0]*n[1], n[0]*(n[1]-n[2]), n[3]*n[4], n[3]*(n[4]-n[5]), n[7])
}
