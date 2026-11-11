//go:build integration && linux

package integration_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

type preparationGate struct {
	reader  problem.ReferenceObjectReader
	entered chan context.Context
	release chan struct{}
	once    sync.Once
}

func (g *preparationGate) Open(ctx context.Context, object materialstore.Object) (*materialstore.Snapshot, error) {
	g.once.Do(func() { g.entered <- ctx })
	select {
	case <-g.release:
		return g.reader.Open(ctx, object)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestReferencePreparationIsBoundedAndDoesNotPublishAfterLoss(t *testing.T) {
	for _, mode := range []string{"cancel", "logout", "shutdown"} {
		t.Run(mode, func(t *testing.T) {
			f := newExpectedPlanFixture(t)
			f.limits(t, 2, 2)
			const other = "another-preparation-request"
			_, err := f.store.Enqueue(f.ctx, f.actor, other, f.draft.ID, f.catalog)
			must(t, err)
			gate := &preparationGate{reader: f.materials, entered: make(chan context.Context, 1), release: make(chan struct{})}
			var revoked atomic.Bool
			rechecked := make(chan struct{}, 1)
			sessions := expectedSessionFunc(func(ctx context.Context, credential redisstore.Credential) (redisstore.View, error) {
				if credential != f.credential || revoked.Load() {
					select {
					case rechecked <- struct{}{}:
					default:
					}
					return redisstore.View{}, redisstore.ErrNotFound
				}
				return f.session.Read(ctx, credential)
			})
			plans, err := problemreference.NewPlanService(sessions, f.store, f.repository, gate)
			must(t, err)
			prepare, err := problemreference.NewPreparation(plans)
			must(t, err)
			t.Cleanup(prepare.Close)
			if _, err := prepare.Start(f.ctx, f.credential, strings.Repeat("f", 32), expectedPlanKey); !errors.Is(err, problemreference.ErrSubjectChanged) {
				t.Fatal("wrong subject could prepare", err)
			}
			ready, err := prepare.Start(f.ctx, f.credential, f.actor.SubjectID, expectedPlanKey)
			must(t, err)
			if ready {
				t.Fatal("accepted work was reported as durable")
			}
			var job context.Context
			select {
			case job = <-gate.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("job did not open its first material")
			}
			if _, err := prepare.Start(f.ctx, f.credential, f.actor.SubjectID, other); !errors.Is(err, problemreference.ErrPreparationBusy) {
				t.Fatal("second job was queued in a busy process", err)
			}
			if mode == "cancel" {
				_, err := f.store.Cancel(f.ctx, f.actor, expectedPlanKey)
				must(t, err)
			}
			if mode == "logout" {
				revoked.Store(true)
			}
			if mode == "shutdown" {
				prepare.Close()
			} else {
				close(gate.release)
			}
			select {
			case <-job.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("lost authority did not release preparation")
			}
			if mode == "logout" {
				select {
				case <-rechecked:
				case <-time.After(3 * time.Second):
					t.Fatal("job never re-observed the revoked session after freezing")
				}
			}
			if mode == "cancel" {
				for deadline := time.Now().Add(3 * time.Second); ; {
					_, err := prepare.Start(f.ctx, f.credential, f.actor.SubjectID, other)
					if !errors.Is(err, problemreference.ErrPreparationBusy) {
						must(t, err)
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("cancelled intent never left the active slot")
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			prepare.Close()
			if _, err := f.store.ExpectedPlan(f.ctx, f.actor, expectedPlanKey); !errors.Is(err, problemreference.ErrNotFound) {
				t.Fatal("lost authority nevertheless produced a plan", err)
			}
		})
	}
}
