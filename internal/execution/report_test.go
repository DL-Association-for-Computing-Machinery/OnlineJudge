package execution

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

func TestJudgeReport(t *testing.T) {
	for _, name := range []string{"AC", "WA", "CE", "cleanup", "lease", "cancel", "attribution"} {
		t.Run(name, func(t *testing.T) {
			d, blobs := fixture()
			d.SubmissionID = "submission-a"
			l := submission.Lease{Token: strings.Repeat("a", 64), SubmissionID: d.SubmissionID, JudgeGeneration: 1, RecoveryGeneration: 1, AttemptNumber: 1}
			s := goodSandbox()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			renew := func(context.Context, string) error { return nil }
			switch name {
			case "WA":
				s.runs[0].Stdout = []byte("wrong")
			case "CE":
				s.compiled.Status, s.compiled.Exit = "Nonzero Exit Status", 1
			case "cleanup":
				s.removeErr = errors.New("cleanup failed")
			case "lease":
				renew = func(context.Context, string) error { return submission.ErrLease }
			case "cancel":
				cancel()
			case "attribution":
				l.SubmissionID = "other"
			}
			report, err := NewLeased(s, renew, func(context.Context) error { return nil }).JudgeReport(ctx, l, d, blobs)
			if name == "AC" || name == "WA" || name == "CE" {
				if err != nil || report.Verdict != name || len(report.EvidenceSHA256) != 64 {
					t.Fatalf("report=%+v err=%v", report, err)
				}
				if s.calls[len(s.calls)-1] != "remove:artifact" {
					t.Fatal("report prepared before cleanup")
				}
			} else if err == nil || report != (submission.Result{}) {
				t.Fatal("failed attempt produced report")
			}
			if name == "attribution" && len(s.calls) != 0 {
				t.Fatal("wrong submission executed")
			}
		})
	}
}

func TestEvidenceAttributionAndFrozenValue(t *testing.T) {
	d, blobs := fixture()
	d.SubmissionID = "submission-a"
	l := submission.Lease{SubmissionID: d.SubmissionID, JudgeGeneration: 1, RecoveryGeneration: 1, AttemptNumber: 1}
	r, err := New(goodSandbox()).Judge(context.Background(), d, blobs)
	if err != nil {
		t.Fatal(err)
	}
	original := freezeReport(l, d, r)
	for _, name := range []string{"submission", "judge", "recovery", "attempt", "source", "manifest", "verdict", "point", "compile", "max"} {
		t.Run(name, func(t *testing.T) {
			lease, descriptor := l, d
			result, _ := New(goodSandbox()).Judge(context.Background(), d, blobs)
			switch name {
			case "submission":
				lease.SubmissionID = "submission-b"
			case "judge":
				lease.JudgeGeneration++
			case "recovery":
				lease.RecoveryGeneration++
			case "attempt":
				lease.AttemptNumber++
			case "source":
				descriptor.SourceSHA256 = strings.Repeat("b", 64)
			case "manifest":
				descriptor.ManifestSHA256 = strings.Repeat("c", 64)
			case "verdict":
				result.Verdict = "WA"
			case "point":
				result.Points[1].Verdict = "NOT_RUN"
			case "compile":
				result.Compile.CPU++
			case "max":
				result.Max.Memory++
			}
			if freezeReport(lease, descriptor, result).EvidenceSHA256 == original.EvidenceSHA256 {
				t.Fatal("changed evidence not bound")
			}
		})
	}
	l.Token = "private token"
	r.Compile.Artifact, r.Compile.Stdout = "private ephemeral ID", []byte("private diagnostics")
	if freezeReport(l, d, r) != original {
		t.Fatal("private ephemeral data changed evidence")
	}
	r.Points[0].Usage.CPU++
	if freezeReport(l, d, r) == original {
		t.Fatal("point usage not bound")
	}
	if original.Verdict != "AC" {
		t.Fatal("frozen report changed")
	}
}
