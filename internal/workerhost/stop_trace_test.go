package workerhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStopTraceDistinguishesExpiredKillAttempt(t *testing.T) {
	h, f := opened(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	stopCtx := ctx
	kills := 0
	h.run = func(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
		switch args[2] {
		case "top":
			// Model teardown that consumes the remaining total stop deadline.
			<-stopCtx.Done()
			return nil, stopCtx.Err()
		case "kill":
			kills++
			return runCommand(ctx, nil, "/bin/sh", "-c", "printf SHOULD_NOT_RUN")
		default:
			return f.run(ctx, input, name, args...)
		}
	}
	err := h.Stop(ctx)
	d := StopDetails(err)
	if d.Stage != "kill" || d.Code != "deadline" || d.Observed != 0 || kills != 1 || !errors.Is(err, context.DeadlineExceeded) || len(d.Steps) != 4 {
		t.Fatalf("expired stop=%+v attempts=%d", d, kills)
	}
	enumerate, kill := d.Steps[2], d.Steps[3]
	if enumerate.Stage != "enumerate" || enumerate.Code != "deadline" || kill.Stage != "kill" || kill.ContextBefore != "deadline" || !kill.CallAttempted || kill.LocalProcessStart != "not_started" {
		t.Fatalf("deadline attribution lost: %+v", d.Steps)
	}
	for _, s := range d.Steps {
		if s.ElapsedMS < 0 || s.ElapsedMS > 60000 {
			t.Fatal("unbounded duration")
		}
	}
	d.Steps[0].Code = "PRIVATE"
	if StopDetails(err).Steps[0].Code != "ok" {
		t.Fatal("returned trace aliases failure")
	}
	raw, err := json.Marshal(StopDetails(err))
	if err != nil || strings.Contains(string(raw), "SHOULD_NOT_RUN") || strings.Contains(string(raw), h.id) {
		t.Fatal("stop trace leaked command or identity")
	}
}

func TestLocalCommandStartEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// The fixed shell signals actual startup before exec'ing a bounded sleep.
	// CommandContext kills that same local process; no sandbox code is executed.
	signal := &commandStartedSignal{started: make(chan struct{})}
	err := runCommandTo(ctx, nil, signal, 16, "/bin/sh", "-c", "printf ready; exec sleep 30")
	select {
	case <-signal.started:
	default:
		t.Fatal("local command did not signal startup")
	}
	var command *commandFailure
	if !errors.As(err, &command) || command.processStart != "started" || command.code != "deadline" {
		t.Fatalf("started process timeout evidence=%v", err)
	}
	trace := stopTrace{started: time.Now()}
	trace.recordKill(time.Now(), "active", err)
	if trace.steps[0].LocalProcessStart != "started" || trace.steps[0].ContextBefore != "active" {
		t.Fatal("started local command became unknown")
	}
	trace = stopTrace{started: time.Now()}
	trace.recordKill(time.Now(), "active", errors.New("PRIVATE"))
	if trace.steps[0].LocalProcessStart != "unknown" {
		t.Fatal("test double acquired process evidence")
	}
}

type commandStartedSignal struct {
	started chan struct{}
	once    sync.Once
}

func (s *commandStartedSignal) Write(p []byte) (int, error) {
	s.once.Do(func() { close(s.started) })
	return len(p), nil
}

func TestStopElapsedIsBoundedAndMarked(t *testing.T) {
	if value, capped := boundedElapsed(time.Now().Add(-2 * time.Minute)); value != 60000 || !capped {
		t.Fatal("large elapsed time silently truncated")
	}
	if value, capped := boundedElapsed(time.Now().Add(time.Minute)); value != 0 || !capped {
		t.Fatal("negative elapsed time accepted")
	}
	h, _ := opened(t)
	if err := h.Stop(context.Background()); err != nil || StopDetails(err) != nil {
		t.Fatal("success acquired failure diagnostics", err)
	}
}
