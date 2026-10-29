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

// Output and runtime are bounded before buffering. Never propagate stderr or
// command arguments: requests may contain private source and test input.
func runCommand(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = "/tmp" // Control commands do not depend on the repository working directory.
	cmd.Stdin = input
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return nil, errors.Join(errors.New("host command failed"), ctx.Err())
	}
	return out.Bytes(), nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > 128*1024+1-b.Len() {
		return 0, errors.New("host output exceeds budget")
	}
	return b.Buffer.Write(p)
}
