package workerhost

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReferenceStartPreflightAndFullIdentity(t *testing.T) {
	for _, scenario := range []string{"good", "owner", "running", "paused", "restarting", "pid", "image", "mount", "changed-id", "lost-start-ack"} {
		t.Run(scenario, func(t *testing.T) {
			f := &fakeHost{s: goodState()}
			f.s.Running = false
			f.s.PID = 0
			original := f.s.ID
			starts := 0
			switch scenario {
			case "running":
				f.s.Running = true
			case "paused":
				f.s.Paused = true
			case "restarting":
				f.s.Restarting = true
			case "pid":
				f.s.PID = 42
			case "image":
				f.s.Image = "other"
			case "mount":
				f.s.Mounts = 1
			}
			run := func(ctx context.Context, input io.Reader, name string, args ...string) ([]byte, error) {
				if len(args) >= 6 && args[2] == "inspect" && args[4] == `{{index .Config.Labels "owner"}}` {
					if scenario == "owner" {
						return []byte("other"), nil
					}
					return []byte("onlinejudge.desktop-test"), nil
				}
				if len(args) >= 4 && args[2] == "start" {
					starts++
					if args[3] != original {
						t.Fatal("start used a mutable alias")
					}
					f.s.Running = true
					f.s.PID = 42
					f.s.Started = "new-start"
					if scenario == "changed-id" {
						f.s.ID = strings.Repeat("b", 64)
					}
					if scenario == "lost-start-ack" {
						return nil, errors.New("lost")
					}
					return []byte(original), nil
				}
				return f.run(ctx, input, name, args...)
			}
			h, err := startM0Slot(context.Background(), run, M0Slot{})
			if scenario == "good" {
				if err != nil || h == nil {
					t.Fatal(err)
				}
				id, started := h.Identity()
				if id != original || started != "new-start" {
					t.Fatal("binding")
				}
				if err := h.Stop(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("unsafe start accepted")
			}
			if scenario == "lost-start-ack" {
				if h == nil {
					t.Fatal("lost original binding")
				}
				if err := h.Stop(context.Background()); err != nil {
					t.Fatal("could not stop after lost ack", err)
				}
			}
			wantStarts := 0
			if scenario == "good" || scenario == "changed-id" || scenario == "lost-start-ack" {
				wantStarts = 1
			}
			if starts != wantStarts {
				t.Fatalf("start calls %d", starts)
			}
		})
	}
}
