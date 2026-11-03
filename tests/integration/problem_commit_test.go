//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

// loseDraftCommitConn forwards the real fixture's PostgreSQL wire bytes. For
// exactly one simple COMMIT it consumes the actual COMMIT/ReadyForQuery reply,
// then closes transport before pgx sees the response. No response is fabricated,
// and the adapter therefore observes a genuinely uncertain Commit call.
type loseDraftCommitConn struct {
	net.Conn
	drop     *atomic.Bool
	armed    bool
	observed chan bool
}

func (c *loseDraftCommitConn) Write(p []byte) (int, error) {
	if len(p) >= 6 && p[0] == 'Q' && int(binary.BigEndian.Uint32(p[1:5])) == len(p)-1 && bytes.Equal(p[5:], []byte("commit\x00")) && c.drop.CompareAndSwap(true, false) {
		c.armed = true
	}
	return c.Conn.Write(p)
}
func (c *loseDraftCommitConn) Read(p []byte) (int, error) {
	if !c.armed {
		return c.Conn.Read(p)
	}
	c.armed = false
	committed := false
	for {
		var header [5]byte
		if _, err := io.ReadFull(c.Conn, header[:]); err != nil {
			return 0, err
		}
		size := int(binary.BigEndian.Uint32(header[1:]))
		if size < 4 || size > 8192 {
			return 0, errors.New("invalid disposable PostgreSQL frame")
		}
		body := make([]byte, size-4)
		if _, err := io.ReadFull(c.Conn, body); err != nil {
			return 0, err
		}
		if header[0] == 'C' && bytes.Equal(body, []byte("COMMIT\x00")) {
			committed = true
		}
		if header[0] == 'Z' {
			c.observed <- committed
			_ = c.Conn.Close()
			return 0, io.EOF
		}
	}
}

func TestPrivateDraftCommitUnknown(t *testing.T) {
	f := newDraftFixture(t)
	f.role(t, f.actor.SubjectID, 0, true)
	var drop atomic.Bool
	drop.Store(true)
	observed := make(chan bool, 1)
	cfg := f.b.Config()
	cfg.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &loseDraftCommitConn{Conn: conn, drop: &drop, observed: observed}, nil
	}
	pool, err := pgxpool.NewWithConfig(f.ctx, cfg)
	must(t, err)
	defer pool.Close()
	repository, err := problempg.New(pool, f.access)
	must(t, err)
	_, err = repository.Create(f.ctx, f.actor, "lost-commit-original-key-01", f.prepared)
	if !errors.Is(err, problem.ErrCommitUnknown) {
		t.Fatalf("lost COMMIT reply misclassified: %v", err)
	}
	select {
	case committed := <-observed:
		if !committed {
			t.Fatal("did not observe the real server commit")
		}
	default:
		t.Fatal("COMMIT transport fault was not reached")
	}
	f.count(t, 1, len(f.prepared.Content().References))
	again, err := repository.Create(f.ctx, f.actor, "lost-commit-original-key-01", f.prepared)
	must(t, err)
	got, err := f.repository.GetOwn(f.ctx, f.actor, again.ID)
	must(t, err)
	if got.ID != again.ID || got.RequestKey != "lost-commit-original-key-01" {
		t.Fatal("original-key recovery changed receipt")
	}
	f.count(t, 1, len(f.prepared.Content().References))
	t.Log("real PostgreSQL COMMIT reply consumed then transport closed; ErrCommitUnknown returned and original key recovered the single complete draft")
}

func TestPrivateDraftRoleAuditRollback(t *testing.T) {
	f := newDraftFixture(t)
	_, err := f.a.Exec(f.ctx, `CREATE FUNCTION reject_author_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit write failure'; END $$; CREATE TRIGGER reject_author_audit BEFORE INSERT ON identity_problem_author_audit FOR EACH ROW EXECUTE FUNCTION reject_author_audit()`)
	must(t, err)
	change := identity.ProblemRoleChange{SubjectID: f.actor.SubjectID, ExpectedRevision: 0, Enabled: true, Operator: "audit-fixture", Reason: "must rollback if audit fails"}
	_, err = f.access.SetAuthorRole(f.ctx, change)
	if !errors.Is(err, identity.ErrStore) {
		t.Fatal("audit failure ignored")
	}
	_, err = f.access.AuthorRole(f.ctx, f.actor.SubjectID)
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatal("grant survived missing audit")
	}
	_, err = f.a.Exec(f.ctx, `ALTER TABLE identity_problem_author_audit DISABLE TRIGGER reject_author_audit`)
	must(t, err)
	f.role(t, f.actor.SubjectID, 0, true)
	_, err = f.a.Exec(f.ctx, `ALTER TABLE identity_problem_author_audit ENABLE TRIGGER reject_author_audit`)
	must(t, err)
	change.ExpectedRevision = 1
	change.Enabled = false
	_, err = f.access.SetAuthorRole(f.ctx, change)
	if !errors.Is(err, identity.ErrStore) {
		t.Fatal("revoke ignored audit failure")
	}
	role, err := f.access.AuthorRole(f.ctx, f.actor.SubjectID)
	must(t, err)
	if !role.Enabled || role.Revision != 1 {
		t.Fatal("role/revision escaped failed audit transaction")
	}
	var count int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM identity_problem_author_audit`).Scan(&count))
	if count != 1 {
		t.Fatal("partial audit committed")
	}
}
