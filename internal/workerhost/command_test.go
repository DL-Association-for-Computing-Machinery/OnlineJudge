package workerhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCommandFailureProjection(t *testing.T) {
	for _, scenario := range []string{"exit", "launch", "stdin", "exact-output", "output", "deadline", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			name, args := "/bin/sh", []string{"-c", "printf 'synthetic-secret' >&2; exit 42", "synthetic-secret"}
			want := "command_exit"
			var input io.Reader
			switch scenario {
			case "launch":
				name, want = "/missing-synthetic-secret", "launch_failed"
			case "stdin":
				name, args, input, want = "/bin/cat", nil, failingCommandInput{}, "unknown"
			case "output":
				args, want = []string{"-c", "head -c 131074 /dev/zero"}, "output_limit"
			case "exact-output":
				args = []string{"-c", "head -c 131073 /dev/zero"}
			case "deadline":
				var done context.CancelFunc
				ctx, done = context.WithTimeout(ctx, 100*time.Millisecond)
				defer done()
				args, want = []string{"-c", "exec sleep 30"}, "deadline"
			case "canceled":
				cancel()
				want = "canceled"
			}
			raw, err := runCommand(ctx, input, name, args...)
			if scenario == "exact-output" {
				if err != nil || len(raw) != 131073 {
					t.Fatal("exact output boundary rejected")
				}
				return
			}
			var failure *commandFailure
			if err == nil || len(raw) != 0 || !errors.As(err, &failure) || failure.code != want {
				t.Fatalf("classification: %s %v", want, err)
			}
			if scenario == "exit" && failure.exit != 42 {
				t.Fatal("lost internal exit status")
			}
			if scenario == "deadline" && !errors.Is(err, context.DeadlineExceeded) || scenario == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost context identity")
			}
			projected, e := json.Marshal(StopDetails(err))
			if e != nil || strings.Contains(err.Error()+string(projected), "synthetic-secret") || strings.Contains(string(projected), `"exit"`) {
				t.Fatal("private command details escaped")
			}
		})
	}
}

type failingCommandInput struct{}

func (failingCommandInput) Read([]byte) (int, error) { return 0, errors.New("synthetic-secret") }
