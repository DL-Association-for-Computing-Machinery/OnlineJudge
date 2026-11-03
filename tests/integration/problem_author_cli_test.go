//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

func TestPrivateDraftRoleCommand(t *testing.T) {
	f := newDraftFixture(t)
	binary, err := filepath.Abs("../../.local/bin/problem-author")
	must(t, err)
	var schema string
	must(t, f.a.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema))
	cfg := f.a.Config().ConnConfig
	// ConnString does not serialize typed RuntimeParams edits. Carry the owned
	// schema explicitly in a URL, so the actual command cannot reach public.
	u := url.URL{Scheme: "postgres", User: url.User(cfg.User), Host: "localhost", Path: "/" + cfg.Database}
	q := url.Values{"host": {cfg.Host}, "sslmode": {"disable"}, "search_path": {schema}}
	u.RawQuery = q.Encode()
	run := func(wantSuccess bool, args ...string) identity.ProblemAuthorRole {
		t.Helper()
		cmd := exec.CommandContext(f.ctx, binary, args...)
		cmd.Env = append(os.Environ(), "OJ_DATABASE_URL="+u.String())
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		e := cmd.Run()
		if wantSuccess {
			must(t, e)
			var r identity.ProblemAuthorRole
			must(t, json.Unmarshal(out.Bytes(), &r))
			return r
		}
		if e == nil || out.Len() != 0 || stderr.Len() == 0 {
			t.Fatal("failed role command emitted success or no stable error")
		}
		return identity.ProblemAuthorRole{}
	}
	run(false, "inspect", "--subject", f.actor.SubjectID)
	args := []string{"set", "--subject", f.actor.SubjectID, "--expected-revision", "0", "--enabled", "true", "--operator", "synthetic-cli-maintainer", "--reason", "isolated command acceptance"}
	r := run(true, args...)
	if r.SubjectID != f.actor.SubjectID || !r.Enabled || r.Revision != 1 {
		t.Fatal("role command returned wrong initial grant")
	}
	run(false, args...)
	r = run(true, "inspect", "--subject", f.actor.SubjectID)
	if !r.Enabled || r.Revision != 1 {
		t.Fatal("conflicting CLI retry changed role")
	}
	args[4] = "1"
	args[6] = "false"
	r = run(true, args...)
	if r.Enabled || r.Revision != 2 {
		t.Fatal("explicit CLI revocation failed")
	}
	var audit int
	must(t, f.a.QueryRow(f.ctx, `SELECT count(*) FROM identity_problem_author_audit WHERE subject_id=$1 AND maintenance_operator='synthetic-cli-maintainer'`, f.actor.SubjectID).Scan(&audit))
	if audit != 2 {
		t.Fatal("role command did not persist audit attribution")
	}
}
