//go:build sandbox

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerhost"
)

func TestMediumReferenceSandbox(t *testing.T) {
	if os.Getenv("OJ_MEDIUM_REFERENCE") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || os.Getenv("OJ_WORKER_M0_SLOT") != "" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("explicit independent Desktop medium reference experiment required")
	}
	t.Run("complete-twenty-nine-point-cli", func(t *testing.T) {
		referenceSourceCLI(t, "library-checker:zalgorithm", "OJ_MEDIUM_OUTPUT", 29)
	})
	t.Run("raw-four-MiB-collector", func(t *testing.T) {
		h := mediumSandboxHost(t)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		c, err := experiment.ResolveReferenceTier(experiment.ReferenceMedium)
		if err != nil {
			t.Fatal(err)
		}
		files := &mediumMeasuredFiles{host: h, t: t}
		g, err := execution.NewReferenceGoJudgeForTier(referenceProbeTransport{h, t}, h.Verify, files, experiment.ReferenceMedium)
		if err != nil {
			t.Fatal(err)
		}
		if err := g.Verify(ctx, c.Runtime); err != nil {
			t.Fatal(err)
		}
		o, err := g.Compile(ctx, c.Runtime, `#include <cstdio>
int main(){int n;if(scanf("%d",&n)!=1)return 3;for(int i=0;i<n;i++)putchar((unsigned char)i);}`)
		if err != nil || o.Artifact == "" {
			t.Fatal("medium output probe compile", err)
		}
		for _, size := range []int{(1 << 20) + 1, 4 << 20, (4 << 20) + 1} {
			out, err := g.Run(ctx, c.Runtime, o.Artifact, []byte(strconv.Itoa(size)))
			if err != nil {
				t.Fatal("medium output", size, err)
			}
			v, err := execution.Classify(out, false)
			if err != nil {
				t.Fatal(err)
			}
			if size <= c.Runtime.OutputBytes {
				want := make([]byte, size)
				for i := range want {
					want[i] = byte(i)
				}
				if v != "AC" || !bytes.Equal(out.Stdout, want) {
					t.Fatal("medium byte output changed")
				}
			} else if v != "OLE" || !out.OutputTruncated {
				t.Fatal("missing collector overflow evidence")
			}
			t.Logf("raw requested=%d returned=%d SHA256=%x verdict=%s CPU=%d memory=%d", size, len(out.Stdout), sha256.Sum256(out.Stdout), v, out.CPU, out.Memory)
		}
		if err := g.Remove(ctx, o.Artifact); err != nil {
			t.Fatal(err)
		}
		referenceStorageSample(t, h, "medium-collector-after-cleanup")
	})
	t.Run("global-file-limit-eight-MiB", func(t *testing.T) {
		h := mediumSandboxHost(t)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		c, err := experiment.ResolveReferenceTier(experiment.ReferenceMedium)
		if err != nil {
			t.Fatal(err)
		}
		g, err := execution.NewReferenceGoJudgeForTier(h, h.Verify, h, experiment.ReferenceMedium)
		if err != nil {
			t.Fatal(err)
		}
		if err := g.Verify(ctx, c.Runtime); err != nil {
			t.Fatal(err)
		}
		o, err := g.Compile(ctx, c.Runtime, `#include <cstdio>
#include <sys/stat.h>
#include <unistd.h>
int main(){int n;if(scanf("%d",&n)!=1)return 3;FILE*f=fopen("artifact","wb");if(!f)return 4;char b[65536]={};
for(int i=0;i<n;){int k=n-i;if(k>65536)k=65536;if(fwrite(b,1,k,f)!=(size_t)k)return 5;i+=k;}if(fflush(f))return 6;
struct stat s;if(fstat(fileno(f),&s))return 7;printf("%lld",(long long)s.st_size);}`)
		if err != nil || o.Artifact == "" {
			t.Fatal("global limit probe compile", err)
		}
		for _, size := range []int{(1 << 20) + 1, 8 << 20, (8 << 20) + 1} {
			out, err := g.Run(ctx, c.Runtime, o.Artifact, []byte(strconv.Itoa(size)))
			if err != nil {
				t.Fatal("global file limit", size, err)
			}
			v, err := execution.Classify(out, false)
			if err != nil {
				t.Fatal(err)
			}
			if size <= 8<<20 {
				if v != "AC" || string(out.Stdout) != strconv.Itoa(size) {
					t.Fatal("global file size evidence")
				}
			} else if v != "OLE" || out.Exit != 25 || out.OutputTruncated {
				t.Fatal("global SIGXFSZ evidence")
			}
			t.Logf("global file requested=%d stdout=%d verdict=%s exit=%d CPU=%d memory=%d", size, len(out.Stdout), v, out.Exit, out.CPU, out.Memory)
		}
		if err := g.Remove(ctx, o.Artifact); err != nil {
			t.Fatal(err)
		}
		referenceStorageSample(t, h, "medium-global-after-cleanup")
	})
}

type mediumMeasuredFiles struct {
	host    *workerhost.Host
	t       *testing.T
	sampled bool
}

func (f *mediumMeasuredFiles) UploadFile(ctx context.Context, raw []byte) (string, error) {
	return f.host.UploadFile(ctx, raw)
}
func (f *mediumMeasuredFiles) ReadFile(ctx context.Context, id string, maximum int64) ([]byte, error) {
	raw, err := f.host.ReadFile(ctx, id, maximum)
	if err == nil && !f.sampled && len(raw) >= 4<<20 {
		f.sampled = true
		referenceStorageSample(f.t, f.host, "medium-stdout-cache-live")
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		f.t.Logf("host Go heap stage sample heap-alloc=%d heap-inuse=%d sys=%d bytes; not process RSS peak", m.HeapAlloc, m.HeapInuse, m.Sys)
	}
	return raw, err
}

func mediumSandboxHost(t *testing.T) *workerhost.Host {
	return referenceTierSandboxHost(t, experiment.ReferenceMedium)
}

func referenceTierSandboxHost(t *testing.T, tier experiment.ReferenceTier) *workerhost.Host {
	t.Helper()
	slot, err := workerhost.ReferenceSlotForTier(tier)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := lockSlotMode(slot.LockName(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.dirty(); err != nil {
		lock.Close()
		t.Fatal(err)
	}
	h, openErr := workerhost.StartReferenceSlot(context.Background(), slot)
	t.Cleanup(func() {
		defer lock.Close()
		if h == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := h.Stop(ctx); err != nil {
			t.Error("medium stop unconfirmed; preserve quarantine", err)
			return
		}
		if err := lock.clean(); err != nil {
			t.Error(err)
		}
	})
	if openErr != nil || h == nil {
		t.Fatal("medium start", openErr)
	}
	return h
}
