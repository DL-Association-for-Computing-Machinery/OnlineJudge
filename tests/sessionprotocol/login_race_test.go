//go:build integration

package sessionprotocol_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	transport "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/httptransport"
	protocol "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

func testLoginDeadline(t *testing.T, f *fixture) {
	t.Run("Redis queued write cannot admit after frozen login deadline", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		l := loginManager(t, s, 300*time.Millisecond, loginKey(t))
		c, err := l.Prepare(f.ctx, p.Email, []byte("synthetic protocol password"))
		must(t, err)
		must(t, f.control.Do(f.ctx, "CLIENT", "PAUSE", 650, "WRITE").Err())
		credential, err := l.Complete(f.ctx, c)
		wantError(t, err, protocol.ErrStale)
		if credential != (protocol.Credential{}) || len(namespaceRecords(t, f, ns)) != 0 {
			t.Fatal("late queued write escaped login window")
		}
		// Retaining the opaque envelope permits explicit cleanup after the short
		// recovery window, while the original absolute bound still governs it.
		must(t, l.Abandon(f.ctx, c))
		keys := namespaceRecords(t, f, ns)
		if len(keys) != 1 {
			t.Fatal("expired attempt could not be tombstoned")
		}
		state, err := f.control.HGet(f.ctx, keys[0], "state").Result()
		must(t, err)
		if state != "dead" {
			t.Fatal("cleanup did not leave a tombstone")
		}
	})
	t.Run("check refuses an existing receipt after the same frozen deadline", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		key := loginKey(t)
		normal := f.store(t, ns, 1, protocol.OrdinaryPolicy())
		l := loginManager(t, normal, 300*time.Millisecond, key)
		c, err := l.Prepare(f.ctx, p.Email, []byte("synthetic protocol password"))
		must(t, err)
		h := &hook{op: "check", before: func(context.Context, redis.Cmder) error { f.waitTime(t, c.Deadline()); return nil }}
		delayed := loginManager(t, f.faultStore(t, ns, h, time.Second), 300*time.Millisecond, key)
		credential, err := delayed.Complete(f.ctx, c)
		wantError(t, err, protocol.ErrStale)
		if credential != (protocol.Credential{}) || !h.used.Load() {
			t.Fatal("expired confirmation published credential")
		}
		keys := namespaceRecords(t, f, ns)
		if len(keys) != 1 {
			t.Fatal("confirmation error lost unknown admitted record")
		}
		state, err := f.control.HGet(f.ctx, keys[0], "state").Result()
		must(t, err)
		if state != "live" {
			t.Fatal("confirmation pretended to rollback admission")
		}
		must(t, l.Abandon(f.ctx, c))
	})
	t.Run("malformed protocol deadline never falls back to unbounded", func(t *testing.T) {
		for _, raw := range []string{"", "-1", "00", "invalid", "9223372036854775808"} {
			t.Run("value_"+raw, func(t *testing.T) {
				ns := namespace(t)
				p := f.subject(t, false)
				h := &hook{op: "retry", before: func(_ context.Context, cmd redis.Cmder) error {
					args := cmd.Args()
					args[len(args)-1] = raw
					return nil
				}}
				s := f.faultStore(t, ns, h, time.Second)
				l := loginManager(t, s, time.Minute, loginKey(t))
				c, err := l.Prepare(f.ctx, p.Email, []byte("synthetic protocol password"))
				must(t, err)
				credential, err := l.Complete(f.ctx, c)
				wantError(t, err, protocol.ErrConflict)
				if credential != (protocol.Credential{}) || len(namespaceRecords(t, f, ns)) != 0 {
					t.Fatal("invalid deadline admitted a session")
				}
			})
		}
	})
	t.Run("abandon before delayed retry prevents late admission", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		key := loginKey(t)
		normal := f.store(t, ns, 1, protocol.OrdinaryPolicy())
		l := loginManager(t, normal, time.Minute, key)
		c, err := l.Prepare(f.ctx, p.Email, []byte("synthetic protocol password"))
		must(t, err)
		entered, release := make(chan struct{}), make(chan struct{})
		h := &hook{op: "retry", before: func(ctx context.Context, _ redis.Cmder) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}}
		delayed := loginManager(t, f.faultStore(t, ns, h, time.Second), time.Minute, key)
		out := make(chan error, 1)
		go func() {
			credential, err := delayed.Complete(f.ctx, c)
			if credential != (protocol.Credential{}) {
				out <- protocol.ErrConflict
				return
			}
			out <- err
		}()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("retry did not reach delayed window")
		}
		must(t, l.Abandon(f.ctx, c))
		close(release)
		wantError(t, await(t, out), protocol.ErrStale)
		keys := namespaceRecords(t, f, ns)
		if len(keys) != 1 {
			t.Fatal("late retry lost tombstone")
		}
		state, err := f.control.HGet(f.ctx, keys[0], "state").Result()
		must(t, err)
		if state != "dead" {
			t.Fatal("abandoned candidate revived")
		}
	})
	t.Run("cookie arriving after successful check and abandon is unusable", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		key := loginKey(t)
		normal := f.store(t, ns, 1, protocol.OrdinaryPolicy())
		l := loginManager(t, normal, time.Minute, key)
		c, err := l.Prepare(f.ctx, p.Email, []byte("synthetic protocol password"))
		must(t, err)
		entered, release := make(chan struct{}), make(chan struct{})
		h := &hook{op: "check", after: func(ctx context.Context, _ redis.Cmder) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}}
		x := loginServer(t, loginManager(t, f.faultStore(t, ns, h, time.Second), time.Minute, key))
		out := make(chan loginResponse, 1)
		go func() {
			out <- loginCall(x, "/api/v1/session/login/complete", "", []*http.Cookie{{Name: transport.AttemptCookieName, Value: c.Value()}}, nil)
		}()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("check did not reach publication window")
		}
		must(t, l.Abandon(f.ctx, c))
		close(release)
		late := completedCredential(t, <-out)
		_, err = normal.Read(f.ctx, late)
		wantError(t, err, protocol.ErrStale)
	})
	t.Run("explicit abandon does not clear an unrelated session", func(t *testing.T) {
		ns := namespace(t)
		p := f.subject(t, false)
		other := f.subject(t, false)
		s := f.store(t, ns, 0, protocol.OrdinaryPolicy())
		l := loginManager(t, s, time.Minute, loginKey(t))
		c, err := l.Prepare(f.ctx, p.Email, []byte("synthetic protocol password"))
		must(t, err)
		_, valid := f.create(t, s, other)
		x := loginServer(t, l)
		r := loginCall(x, "/api/v1/session/login/abandon", "", []*http.Cookie{{Name: transport.AttemptCookieName, Value: c.Value()}, {Name: transport.CookieName, Value: valid.Value()}}, nil)
		loginStatus(t, r, 204)
		if len(r.cookies) != 1 || r.cookies[0].Name != transport.AttemptCookieName {
			t.Fatal("abandon overwrote unrelated session cookie")
		}
		v, err := s.Read(f.ctx, valid)
		must(t, err)
		if v.SubjectID != other.ID {
			t.Fatal("abandon changed unrelated identity")
		}
	})
}
