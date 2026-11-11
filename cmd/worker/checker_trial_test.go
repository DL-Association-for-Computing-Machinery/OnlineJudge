package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestCheckerTrialOwnerFinalizesOnlyAfterConfirmedCleanup(t *testing.T) {
	for _, scenario := range []string{"pass", "mismatch", "token", "stop", "clean", "cancel", "open", "start-error"} {
		t.Run(scenario, func(t *testing.T) {
			p := validationFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			plan, err := p.PrepareReference(ctx, problem.SmallReferenceLimits())
			if err != nil {
				t.Fatal(err)
			}
			limits := judging.TextLimits{Bytes: experiment.PinnedReferenceEnvironment().OutputBytes, TokenBytes: 2}
			c, err := problemvalidation.PreflightCheckerCandidate(ctx, p, plan.Digest(), judging.LibraryCheckerTextVersion, limits)
			if err != nil {
				t.Fatal(err)
			}
			trial, err := problemvalidation.PrepareCheckerTrial(ctx, p, plan.Digest(), c.Digest(), limits)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "lock")
			lock, err := acquireLock(path)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			h := &referenceHost{}
			if scenario == "clean" {
				lock.file = &syncFailureFile{File: lock.file.(*os.File)}
			}
			if scenario == "stop" {
				h.stopErr = errors.New("stop failed")
			}
			if scenario == "cancel" {
				h.cancel = cancel
			}
			if scenario == "mismatch" {
				h.stdout = []byte("4\n")
			}
			if scenario == "token" {
				h.stdout = []byte("333\n")
			}
			report, err := runCheckerTrialLocked(ctx, trial, lock, func(context.Context) (validationHost, error) {
				raw, err := os.ReadFile(path)
				if err != nil || len(raw) == 0 {
					t.Fatal("opened slot before persisted marker", err)
				}
				if scenario == "open" {
					return nil, errors.New("preflight failed")
				}
				if scenario == "start-error" {
					return h, errors.New("start receipt unknown")
				}
				return h, nil
			})
			conclusive := scenario == "pass" || scenario == "mismatch" || scenario == "token"
			if (err == nil) != conclusive || report.Complete != conclusive {
				t.Fatalf("incorrect completion: %+v, %v", report, err)
			}
			if conclusive {
				want := map[string]string{"pass": "AC", "mismatch": "WA", "token": "OLE"}[scenario]
				if report.Verdict != want || len(report.Points) != 1 || report.Points[0].Verdict != want {
					t.Fatalf("wrong outcome: %+v", report)
				}
			}
			if scenario != "open" && (h.stops != 1 || !h.independent || report.StopConfirmed != (scenario != "stop")) {
				t.Fatal("stop was not independently confirmed")
			}
			if (scenario == "open" || scenario == "start-error") && h.requests != 0 {
				t.Fatal("ambiguous startup reached execution")
			}
			raw, err := os.ReadFile(path)
			quarantine := scenario == "open" || scenario == "stop" || scenario == "clean"
			if err != nil || (len(raw) != 0) != quarantine {
				t.Fatal("incorrect quarantine state", err)
			}
		})
	}
}
