//go:build sandbox

package main

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/execution"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
)

type pauseBeforeReferenceCompile struct {
	validationHost
	ready chan struct{}
}

func (p pauseBeforeReferenceCompile) Verify(ctx context.Context, _ experiment.ExecutionProfile) error {
	close(p.ready)
	<-ctx.Done()
	return ctx.Err()
}

// This explicit Desktop test runs only a fixed synthetic sum program in the
// pre-existing owned sandbox. PostgreSQL and mTLS are verified separately.
func TestClaimedReferenceOwnedSandbox(t *testing.T) {
	if os.Getenv("OJ_REFERENCE_LEASED_SANDBOX_TEST") != "1" || os.Getenv("OJ_SANDBOX_EXPERIMENT") != "1" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || os.Getenv(dualModeEnv) != "" || os.Getenv("OJ_WORKER_M0_SLOT") != "" {
		t.Fatal("requires explicitly opted-in, fixed Desktop reference sandbox")
	}
	claim, fetch := claimedReferenceFixture(t, []byte("#include <iostream>\nint main(){long long a,b; if (!(std::cin>>a>>b)) return 1; std::cout<<a+b<<'\\n';}\n"))
	slot, err := referenceClaimSlot(problemreference.ClaimInput{RequestID: claim.RequestID, SourceID: claim.SourceID, PlanSHA256: claim.PlanSHA256})
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []bool{false, true} {
		name := "confirmed"
		if fault {
			name = "renewal denied"
		}
		t.Run(name, func(t *testing.T) {
			lock, err := lockSlotMode(slot, false)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			ready := make(chan struct{})
			renewals := 0
			reported := false
			err = runClaimedReference(t.Context(), lock, claim, fetch, func(context.Context, problemreference.WorkerClaim) error {
				renewals++
				if fault && renewals > 1 {
					select {
					case <-ready:
						return problemreference.ErrClaimUnavailable
					default:
					}
				}
				return nil
			}, func(ctx context.Context, source string) (validationHost, error) {
				h, err := startClaimedReference(ctx, source)
				if fault && err == nil {
					return pauseBeforeReferenceCompile{validationHost: h, ready: ready}, nil
				}
				return h, err
			}, func(_ context.Context, input problemreference.WorkerReportInput) error {
				evidence, err := input.DecodeEvidence()
				if reported || err != nil || evidence.Verdict != "AC" || !evidence.StopConfirmed || input.Token != claim.Token {
					t.Fatal("unexpected evidence from the confirmed sandbox", err)
				}
				reported = true
				return nil
			})
			if fault {
				if !errors.Is(err, execution.ErrLeaseLost) || errors.Is(err, execution.ErrStopUnconfirmed) {
					t.Fatal("lease loss did not end with confirmed stop", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if reported == fault {
				t.Fatal("wrong report outcome after sandbox supervision")
			}
			lock.Close()
			probe, err := lockSlotMode(slot, false)
			if err != nil {
				t.Fatal("confirmed stop did not release isolation", err)
			}
			probe.Close()
		})
		if t.Failed() {
			return
		}
	}
}
