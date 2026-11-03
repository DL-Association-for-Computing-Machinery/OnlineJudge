package workerhost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"time"
)

type command func(context.Context, io.Reader, string, ...string) ([]byte, error)
type streamCommand func(context.Context, io.Reader, io.Writer, int64, string, ...string) error

// Only these locally produced failures retain an exit code. Neither an exec
// error nor its command/path/stderr is retained across the host boundary.
type commandFailure struct {
	code  string
	exit  int
	cause error
}

func (e *commandFailure) Error() string { return "host command " + e.code }
func (e *commandFailure) Unwrap() error { return e.cause }

func contextCause(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return nil
}

// Output and runtime are bounded before buffering. Never propagate stderr or
// command arguments: requests may contain private source and test input.
func runCommand(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
	var out limitedBuffer
	if err := runCommandTo(ctx, input, &out, 128*1024+1, name, args...); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func runCommandTo(ctx context.Context, input io.Reader, output io.Writer, maximum int64, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = "/tmp" // Control commands do not depend on the repository working directory.
	cmd.Stdin = input
	out := &commandSink{target: output, maximum: maximum}
	cmd.Stdout = out
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		failure := &commandFailure{code: "unknown", exit: -1}
		if cause := ctx.Err(); cause != nil {
			failure.cause = cause
			failure.code = "canceled"
			if errors.Is(cause, context.DeadlineExceeded) {
				failure.code = "deadline"
			}
		} else if out.exceeded {
			failure.code = "output_limit"
		} else if out.failed {
			failure.code = "unknown"
		} else {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				failure.code, failure.exit = "command_exit", exit.ExitCode()
			} else if errors.Is(err, exec.ErrWaitDelay) {
				failure.code = "wait_delay"
			} else if cmd.Process == nil {
				failure.code = "launch_failed"
			}
		}
		return failure
	}
	return nil
}

// No ReaderFrom is promoted: every byte must pass the counter before its sink.
type commandSink struct {
	target           io.Writer
	maximum, written int64
	exceeded, failed bool
}

func (s *commandSink) Write(p []byte) (int, error) {
	if int64(len(p)) > s.maximum-s.written {
		s.exceeded = true
		return 0, errors.New("host output exceeds budget")
	}
	n, err := s.target.Write(p)
	s.written += int64(n)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		s.failed = true
	}
	return n, err
}

// Embedding bytes.Buffer would promote ReadFrom and let io.Copy bypass Write.
type limitedBuffer struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (b *limitedBuffer) Len() int      { return b.buffer.Len() }
func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > 128*1024+1-b.Len() {
		b.exceeded = true
		return 0, errors.New("host output exceeds budget")
	}
	return b.buffer.Write(p)
}
