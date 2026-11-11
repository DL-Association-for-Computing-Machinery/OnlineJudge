package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func TestConvolutionRetainsAmbiguousStartupEvidence(t *testing.T) {
	for _, stopFails := range []bool{false, true} {
		name := "stopped"
		if stopFails {
			name = "unknown stop"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			lockPath := filepath.Join(root, "lock")
			lock, err := acquireLock(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			h := &referenceHost{}
			if stopFails {
				h.stopErr = errors.New("private stop error")
			}
			report, err := runConvolutionLocked(t.Context(), problemvalidation.ConvolutionSources{}, lock, func(context.Context) (validationHost, error) {
				if raw, e := os.ReadFile(lockPath); e != nil || string(raw) != "active\n" {
					t.Fatalf("sandbox opened before durable quarantine: %q, %v", raw, e)
				}
				return h, errors.New("private startup error")
			})
			id, started := h.Identity()
			if err == nil || h.stops != 1 || !h.independent || h.requests != 0 ||
				report.Status != "incomplete" || report.Publication != "blocked" || report.FailureStage != "startup" ||
				report.ContainerID != id || report.StartedAt != started || report.StopConfirmed == stopFails {
				t.Fatalf("ambiguous lifetime evidence lost: %+v, %v", report, err)
			}
			if raw, e := os.ReadFile(lockPath); e != nil || (len(raw) != 0) != stopFails {
				t.Fatalf("quarantine disagrees with independent stop: %q, %v", raw, e)
			}
			incomplete := filepath.Join(root, "INCOMPLETE")
			if err := os.WriteFile(incomplete, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if err := persistPrivateValidationReport(root, report, false); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(root, "report.json")
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(p)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("non-private report: %v, %v", info, err)
			}
			var saved problemvalidation.ConvolutionReport
			if err := json.Unmarshal(raw, &saved); err != nil || saved.ContainerID != id || saved.StopConfirmed == stopFails || saved.Status != "incomplete" {
				t.Fatalf("incorrect private report: %+v, %v", saved, err)
			}
			if _, err := os.Stat(incomplete); err != nil {
				t.Fatal("incomplete marker lost", err)
			}
		})
	}
}
