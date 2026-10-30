package sessionexperiment

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

const sessionRedisPoolLimit = 8

func sessionRedisClient(addr, label string, limit int) *redis.Client {
	// A dedicated pipeline pool has its own budget in go-redis v9.23.0.
	// These experiments route pipelines through the capped main pool instead.
	return redis.NewClient(&redis.Options{Addr: addr, ClientName: label + rand.Text(), MaxRetries: -1,
		DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second,
		PoolSize: limit, MaxActiveConns: limit, PipelinePoolSize: -1,
		PoolTimeout: time.Second, ContextTimeoutEnabled: true})
}

// Hold real, initialized sticky connections, not mocked pool counters. Cleanup
// is registered before acquisition so a failed assertion cannot strand a slot.
func holdSessionPool(t *testing.T, client *redis.Client) func() {
	t.Helper()
	var held []*redis.Conn
	release := func() {
		for _, conn := range held {
			if err := conn.Close(); err != nil {
				t.Errorf("release held Redis connection: %v", err)
			}
		}
		held = nil
	}
	t.Cleanup(release)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for range sessionRedisPoolLimit {
		conn := client.Conn()
		held = append(held, conn)
		if err := conn.Ping(ctx).Err(); err != nil {
			t.Fatal(err)
		}
	}
	return release
}

func TestSessionPipelineSharesConnectionBudget(t *testing.T) {
	f := setup(t)
	release := holdSessionPool(t, f.stores[0].client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, err := f.stores[0].client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Ping(ctx)
		return nil
	})
	if !errors.Is(err, redis.ErrPoolTimeout) && !errors.Is(err, redis.ErrPoolExhausted) {
		t.Fatalf("pipeline bypassed saturated pool or failed for another reason: %v", err)
	}
	if time.Since(start) >= 3*time.Second {
		t.Fatal("saturated pipeline exceeded failure budget")
	}
	release()
	recovered, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	if _, err := f.stores[0].client.Pipelined(recovered, func(pipe redis.Pipeliner) error {
		pipe.Ping(recovered)
		return nil
	}); err != nil {
		t.Fatalf("pipeline did not recover after releasing connections: %v", err)
	}
}

func poolObserver(t *testing.T, f *fixture) *redis.Client {
	t.Helper()
	observer := sessionRedisClient(f.stores[0].client.Options().Addr, f.stores[0].prefix+"observer:", 1)
	t.Cleanup(func() { _ = observer.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := observer.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	return observer
}

// Count live Redis sockets by exact, server-visible random labels, including
// all connection types. The one observer connection is outside the API budget.
func apiConnectionCounts(t *testing.T, f *fixture, observer *redis.Client) [2]int {
	t.Helper()
	names := [2]string{f.stores[0].client.Options().ClientName, f.stores[1].client.Options().ClientName}
	if names[0] == "" || names[1] == "" || names[0] == names[1] {
		t.Fatal("API clients need distinct server-visible labels")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	list, err := observer.ClientList(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	var counts [2]int
	for line := range strings.SplitSeq(list, "\n") {
		for _, field := range strings.Fields(line) {
			name, ok := strings.CutPrefix(field, "name=")
			if !ok {
				continue
			}
			for replica, expected := range names {
				if name == expected {
					counts[replica]++
				}
			}
		}
	}
	return counts
}

func observedFixture(f *fixture, observer *redis.Client) *fixture {
	view := *f
	store := *f.stores[0]
	store.client = observer
	view.stores[0] = &store
	return &view
}

func TestSessionRedisPoolPolicy(t *testing.T) {
	f := setup(t)
	for replica := range 2 {
		client := f.stores[replica].client
		options := client.Options()
		if options.PoolSize != sessionRedisPoolLimit || options.MaxActiveConns != sessionRedisPoolLimit ||
			options.PipelinePoolSize >= 0 || options.MaxRetries != 0 || options.PoolTimeout != time.Second {
			t.Fatal("missing explicit connection cap, shared pipeline budget or bounded waiting")
		}
		if f.adminStores[replica].client != client {
			t.Fatal("management allocated another API connection pool")
		}
	}
	if f.stores[0].client == f.stores[1].client {
		t.Fatal("replicas unexpectedly share one client")
	}
}
