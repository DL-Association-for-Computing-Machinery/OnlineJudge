package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
)

type fakeStore struct {
	role   identity.ProblemAuthorRole
	err    error
	calls  int
	change identity.ProblemRoleChange
}

func (s *fakeStore) AuthorRole(_ context.Context, _ string) (identity.ProblemAuthorRole, error) {
	s.calls++
	return s.role, s.err
}
func (s *fakeStore) SetAuthorRole(_ context.Context, c identity.ProblemRoleChange) (identity.ProblemAuthorRole, error) {
	s.calls++
	s.change = c
	return s.role, s.err
}

func TestParseRequiresExplicitSetArguments(t *testing.T) {
	valid := []string{"set", "--subject", "00000000000000000000000000000001", "--expected-revision", "0", "--enabled", "false", "--operator", "maintainer", "--reason", "revoke"}
	c, err := parse(valid)
	if err != nil || c.change.ExpectedRevision != 0 || c.change.Enabled {
		t.Fatalf("valid initial CAS rejected: %+v %v", c, err)
	}
	for i := 1; i < len(valid); i += 2 {
		args := append([]string{}, valid[:i]...)
		args = append(args, valid[i+2:]...)
		if _, err := parse(args); err == nil {
			t.Errorf("accepted missing %s", valid[i])
		}
	}
	for _, args := range [][]string{
		{"set", "--subject", "00000000000000000000000000000001", "--expected-revision", "-1", "--enabled", "true", "--operator", "a", "--reason", "b"},
		{"set", "--subject", "00000000000000000000000000000001", "--expected-revision", "0", "--enabled", "1", "--operator", "a", "--reason", "b"},
		{"inspect", "--subject", "00000000000000000000000000000001", "extra"},
		{"inspect", "--subject", "00000000000000000000000000000001", "--enabled", "true"},
		{"inspect"}, {"unknown"},
	} {
		if _, err := parse(args); err == nil {
			t.Errorf("accepted invalid arguments %v", args)
		}
	}
	if _, err := parse([]string{"inspect", "--subject", "00000000000000000000000000000001"}); err != nil {
		t.Fatalf("inspect: %v", err)
	}
}

func TestExecuteCASAndUncertainOutcomes(t *testing.T) {
	c, err := parse([]string{"set", "--subject", "00000000000000000000000000000001", "--expected-revision", "2", "--enabled", "true", "--operator", "admin", "--reason", "grant"})
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeStore{role: identity.ProblemAuthorRole{SubjectID: "00000000000000000000000000000001", Enabled: true, Revision: 3}}
	var out bytes.Buffer
	if err := execute(context.Background(), c, s, &out); err != nil {
		t.Fatal(err)
	}
	if s.calls != 1 || s.change != c.change || out.String() != "{\"subject_id\":\"00000000000000000000000000000001\",\"enabled\":true,\"revision\":3}\n" {
		t.Fatalf("unexpected call/output: %+v %q", s, out.String())
	}
	s.err = errors.New("sensitive database error")
	if err := execute(context.Background(), c, s, io.Discard); !errors.Is(err, errOutcomeUnknown) || strings.Contains(errorMessage(err), "sensitive") {
		t.Fatalf("unsafe error: %v", err)
	}
	if s.calls != 2 {
		t.Fatal("unexpected retry")
	}
	s.err = identity.ErrConflict
	if err := execute(context.Background(), c, s, io.Discard); !errors.Is(err, identity.ErrConflict) || !strings.Contains(errorMessage(err), "inspect") {
		t.Fatalf("conflict: %v", err)
	}
	s.err = nil
	if err := execute(context.Background(), c, s, brokenWriter{}); !errors.Is(err, errOutcomeUnknown) {
		t.Fatalf("output failure: %v", err)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("failed") }
