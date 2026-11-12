//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

const (
	loadSubjects = 99
	loadParallel = 16
	loadPerUser  = 32
	loadGlobal   = 128
)

type trainingLoadResult struct {
	receipt submission.Receipt
	created bool
	elapsed time.Duration
	err     error
}

func TestTrainingAdmissionLoad(t *testing.T) {
	if os.Getenv("OJ_TRAINING_LOAD") != "1" {
		t.Skip("run toolchain training-load")
	}
	f := newBacklogFixture(t)
	backlogPolicy(t, f, loadGlobal, loadPerUser)
	node := submission.NodeIdentity{ID: "training-probe", CredentialVersion: 1}
	_, err := f.a.Exec(f.ctx, "INSERT INTO worker_nodes(id, credential_version) VALUES($1,1)", node.ID)
	must(t, err)
	add := func(subject string, index int, key string) trainingLoadResult {
		store := f.first
		if index%2 != 0 {
			store = f.second
		}
		start := time.Now()
		r, created, err := store.Accept(f.ctx, subject, fmt.Sprintf("training-context-%d", index%3), key, f.request)
		return trainingLoadResult{r, created, time.Since(start), err}
	}
	hot := make([]submission.Receipt, loadPerUser)
	for i := range hot {
		r := add("hot", i, fmt.Sprintf("hot-probe-key-%04d", i))
		must(t, r.err)
		if !r.created {
			t.Fatal("expected new hot submission")
		}
		hot[i] = r.receipt
	}
	if r := add("hot", loadPerUser, "hot-probe-key-over"); !errors.Is(r.err, submission.ErrBacklog) || r.created {
		t.Fatalf("subject cap was not enforced: %v", r.err)
	}
	replayed, created, err := f.second.Accept(f.ctx, "hot", "training-context-0", "hot-probe-key-0000", f.request)
	must(t, err)
	if created || replayed.ID != hot[0].ID {
		t.Fatal("at-limit original key did not return its original task")
	}

	burst := func(prefix string) []trainingLoadResult {
		result := make([]trainingLoadResult, loadSubjects)
		var wg sync.WaitGroup
		wg.Add(loadParallel)
		for i := range loadParallel {
			go func() {
				defer wg.Done()
				for j := i; j < len(result); j += loadParallel {
					result[j] = add(fmt.Sprintf("cold-%02d", j), j, fmt.Sprintf("cold-%s-key-%04d", prefix, j))
				}
			}()
		}
		wg.Wait()
		return result
	}
	prefilled := summarizeTrainingLoad(t, "prefill", burst("prefill"))
	if len(prefilled) != loadGlobal-loadPerUser {
		t.Fatalf("global cap accepted %d, expected %d", len(prefilled), loadGlobal-loadPerUser)
	}
	backlogCounts(t, f, loadGlobal)

	first, err := f.first.Claim(f.ctx, node)
	observed := time.Now()
	must(t, err)
	if first.SubmissionID != hot[0].ID {
		t.Fatal("oldest rank-1 task was not first")
	}
	finished := make(chan trainingLoadClaim, loadGlobal+loadSubjects)
	arrivalsDone := make(chan struct{})
	workerDone := make(chan struct{})
	ctx, cancel := context.WithCancel(f.ctx)
	defer func() {
		cancel()
		<-workerDone
	}()
	go func() {
		defer close(workerDone)
		defer close(finished)
		if err := drainTrainingLoad(ctx, f, node, first, observed, arrivalsDone, finished); err != nil {
			// On cancellation the caller may already have stopped reading.
			select {
			case finished <- trainingLoadClaim{err: err}:
			case <-ctx.Done():
			}
		}
	}()
	arrivals := burst("overlap")
	close(arrivalsDone)
	overlap := summarizeTrainingLoad(t, "with_claim_report", arrivals)
	measureTrainingLoad(t, f, hot, prefilled, overlap, finished)
}

type trainingLoadClaim struct {
	id  string
	at  time.Time
	err error
}

func drainTrainingLoad(ctx context.Context, f backlogFixture, node submission.NodeIdentity, lease submission.Lease, observed time.Time, arrivalsDone <-chan struct{}, finished chan<- trainingLoadClaim) error {
	result := submission.Result{Verdict: "AC", EvidenceSHA256: strings.Repeat("a", 64)}
	for {
		// Pacing stands in for execution duration; no submitted source is run.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(12 * time.Millisecond):
		}
		if err := f.second.Report(ctx, node, lease.Token, result); err != nil {
			return err
		}
		select {
		case finished <- trainingLoadClaim{id: lease.SubmissionID, at: observed}:
		case <-ctx.Done():
			return ctx.Err()
		}
		for {
			select {
			case <-arrivalsDone:
				arrivalsDone = nil
			default:
			}
			next, err := f.first.Claim(ctx, node)
			if errors.Is(err, submission.ErrNoTask) && arrivalsDone != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-arrivalsDone:
				case <-time.After(time.Millisecond):
				}
				continue
			}
			if errors.Is(err, submission.ErrNoTask) {
				return nil
			}
			if err != nil {
				return err
			}
			lease, observed = next, time.Now()
			break
		}
	}
}

func summarizeTrainingLoad(t *testing.T, label string, results []trainingLoadResult) map[string]time.Time {
	t.Helper()
	accepted := map[string]time.Time{}
	var acceptTimes, rejectTimes []time.Duration
	for _, r := range results {
		switch {
		case r.err == nil && r.created:
			if _, exists := accepted[r.receipt.ID]; exists {
				t.Fatal("duplicate new receipt")
			}
			accepted[r.receipt.ID] = r.receipt.CreatedAt
			acceptTimes = append(acceptTimes, r.elapsed)
		case errors.Is(r.err, submission.ErrBacklog) && !r.created:
			rejectTimes = append(rejectTimes, r.elapsed)
		default:
			t.Fatalf("unexpected burst outcome: created=%v err=%v", r.created, r.err)
		}
	}
	t.Logf("%s accepted=%d cap_rejected=%d accepted_latency=%s rejected_latency=%s", label, len(accepted), len(rejectTimes), trainingLoadPercentiles(acceptTimes), trainingLoadPercentiles(rejectTimes))
	return accepted
}

func measureTrainingLoad(t *testing.T, f backlogFixture, hot []submission.Receipt, prefilled, overlap map[string]time.Time, finished <-chan trainingLoadClaim) {
	t.Helper()
	createdAt := make(map[string]time.Time, loadGlobal+len(overlap))
	for id, at := range prefilled {
		createdAt[id] = at
	}
	for id, at := range overlap {
		createdAt[id] = at
	}
	for _, receipt := range hot {
		createdAt[receipt.ID] = receipt.CreatedAt
	}
	claimed := map[string]int{}
	var allWait, coldWait []time.Duration
	firstCold := 0
	for result := range finished {
		must(t, result.err)
		if claimed[result.id] != 0 {
			t.Fatal("duplicate claimed ID")
		}
		claimed[result.id] = len(claimed) + 1
		submitted, exists := createdAt[result.id]
		if !exists || result.at.Before(submitted) {
			t.Fatal("claim has no matching acceptance or timestamps are invalid")
		}
		wait := result.at.Sub(submitted)
		allWait = append(allWait, wait)
		_, before := prefilled[result.id]
		_, during := overlap[result.id]
		if before || during {
			if firstCold == 0 {
				firstCold = claimed[result.id]
			}
			coldWait = append(coldWait, wait)
		}
	}
	if len(claimed) != loadGlobal+len(overlap) {
		t.Fatal("accepted work was lost")
	}
	for id := range prefilled {
		if claimed[id] < 2 || claimed[id] > 1+len(prefilled) {
			t.Fatal("a prefilled rank-1 task was overtaken by rank-2 work")
		}
	}
	for i, receipt := range hot {
		if claimed[receipt.ID] == 0 || (i > 0 && claimed[receipt.ID] <= claimed[hot[i-1].ID]) {
			t.Fatal("hot user did not retain order")
		}
	}
	var terminal, attempts int
	must(t, f.a.QueryRow(f.ctx, "SELECT (SELECT count(*) FROM judge_tasks WHERE state='finished'), (SELECT count(*) FROM judge_attempts WHERE state='completed')").Scan(&terminal, &attempts))
	if terminal != len(claimed) || attempts != terminal {
		t.Fatal("terminal or attempt totals inconsistent")
	}
	t.Logf("drained=%d first_cold_claim=%d second_hot_claim=%d all_queue_wait=%s cold_queue_wait=%s", terminal, firstCold, claimed[hot[1].ID], trainingLoadPercentiles(allWait), trainingLoadPercentiles(coldWait))
}

func trainingLoadPercentiles(values []time.Duration) string {
	if len(values) == 0 {
		return "n/a"
	}
	slices.Sort(values)
	p := func(percent int) time.Duration { return values[(len(values)*percent+99)/100-1].Round(time.Microsecond) }
	return fmt.Sprintf("p50=%s,p95=%s,max=%s", p(50), p(95), p(100))
}
