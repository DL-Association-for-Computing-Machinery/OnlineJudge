package workerhost

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestStopEnumerationTimeoutStillDispatchesKill(t *testing.T) {
	h, f := opened(t)
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	kills := 0
	enumerationReturned := false
	h.run = func(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
		switch args[2] {
		case "top":
			<-ctx.Done()
			enumerationReturned = true
			return nil, ctx.Err()
		case "kill":
			kills++
			if !enumerationReturned || ctx.Err() != nil || parent.Err() != nil {
				t.Fatal("kill overlapped enumeration or inherited its cancellation")
			}
			out, err := runCommand(ctx, nil, "/bin/sh", "-c", "printf dispatched")
			if err != nil || string(out) != "dispatched" {
				t.Fatal("trusted local termination command was not started", err)
			}
		}
		return f.run(ctx, input, name, args...)
	}
	err := h.Stop(parent)
	d := StopDetails(err)
	if kills != 1 || d == nil || d.Stage != "enumerate" || d.Code != "deadline" || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("missing enumeration evidence must remain failure: %+v, kills=%d", d, kills)
	}
	if d.Steps[3].ContextBefore != "active" || d.Steps[3].Code != "ok" || f.s.Running {
		t.Fatal("termination failed after independent enumeration timeout", d)
	}
}

func TestStopPhaseBudgetPreservesParent(t *testing.T) {
	for _, remaining := range []time.Duration{10 * time.Second, time.Second, -time.Second} {
		parent, cancel := context.WithTimeout(context.Background(), remaining)
		deadline, _ := parent.Deadline()
		child, done := stopPhaseContext(parent)
		childDeadline, _ := child.Deadline()
		if childDeadline.After(deadline) || time.Until(childDeadline) > 2*time.Second {
			t.Fatal("phase extended the total or its own budget")
		}
		if remaining > 0 && deadline.Sub(childDeadline) < remaining/3 {
			t.Fatal("phase did not reserve time for the next operation")
		}
		done()
		if remaining > 0 && parent.Err() != nil {
			t.Fatal("phase cancellation escaped into total context")
		}
		cancel()
	}
}

func TestStopKillTimeoutRetainsFailure(t *testing.T) {
	h, f := opened(t)
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	kills := 0
	h.run = func(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
		if args[2] == "kill" {
			kills++
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return f.run(ctx, input, name, args...)
	}
	err := h.Stop(parent)
	d := StopDetails(err)
	if kills != 1 || d == nil || d.Stage != "kill" || d.Code != "deadline" || parent.Err() != nil || !f.s.Running {
		t.Fatal("kill timeout lost failure, retried or canceled total context", d)
	}
}
