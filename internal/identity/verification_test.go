package identity

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

const verificationID = "00000000000000000000000000000001"

type verificationStore struct {
	get    func(context.Context, string) (Snapshot, error)
	state  func(context.Context, string) (State, error)
	rehash func(context.Context, string, Revision, password.Record) (Snapshot, error)
}

func (s *verificationStore) Get(ctx context.Context, id string) (Snapshot, error) {
	return s.get(ctx, id)
}
func (s *verificationStore) State(ctx context.Context, id string) (State, error) {
	return s.state(ctx, id)
}
func (s *verificationStore) RehashCAS(ctx context.Context, id string, rev Revision, record password.Record) (Snapshot, error) {
	return s.rehash(ctx, id, rev, record)
}

type verificationWork struct {
	verify func(context.Context, []byte, password.Record) (password.Verification, error)
	create func(context.Context, []byte) (password.Record, error)
}

func (w *verificationWork) Verify(ctx context.Context, p []byte, r password.Record) (password.Verification, error) {
	return w.verify(ctx, p, r)
}
func (w *verificationWork) Create(ctx context.Context, p []byte) (password.Record, error) {
	return w.create(ctx, p)
}

type verificationFixture struct {
	v      *CredentialVerifier
	store  *verificationStore
	work   *verificationWork
	row    Snapshot
	secret []byte
	events []string
}

func newVerificationFixture(t *testing.T, legacy bool) *verificationFixture {
	t.Helper()
	f := &verificationFixture{
		row: Snapshot{State: State{ID: verificationID, Enabled: true, Revision: Revision{CredentialVersion: 4, AuthGeneration: 7}},
			Record: password.Record{FormatVersion: 1, Algorithm: "argon2id", ArgonVersion: 0x13,
				Parameters: password.Parameters{MemoryKiB: 8, Iterations: 1, Parallelism: 1, SaltBytes: 8, DigestBytes: 16},
				Salt:       make([]byte, 8), Digest: make([]byte, 16)}},
		secret: []byte(" synthetic\x00密码 "),
	}
	f.store = &verificationStore{
		get: func(_ context.Context, id string) (Snapshot, error) {
			f.events = append(f.events, "get")
			if id != verificationID {
				t.Fatal("wrong subject lookup")
			}
			return f.row, nil
		},
		state: func(_ context.Context, id string) (State, error) {
			f.events = append(f.events, "state")
			if id != verificationID {
				t.Fatal("wrong final subject lookup")
			}
			return f.row.State, nil
		},
		rehash: func(_ context.Context, id string, rev Revision, record password.Record) (Snapshot, error) {
			f.events = append(f.events, "rehash")
			if id != verificationID || rev != f.row.Revision {
				t.Fatal("upgrade did not use the verified revisions")
			}
			f.row.CredentialVersion++
			f.row.Record = record
			return f.row, nil
		},
	}
	f.work = &verificationWork{
		verify: func(_ context.Context, secret []byte, record password.Record) (password.Verification, error) {
			f.events = append(f.events, "verify")
			if !bytes.Equal(secret, f.secret) || !reflect.DeepEqual(record, f.row.Record) {
				t.Fatal("verification input changed")
			}
			return password.Verification{Match: true, NeedsUpgrade: legacy}, nil
		},
		create: func(_ context.Context, secret []byte) (password.Record, error) {
			f.events = append(f.events, "create")
			if !bytes.Equal(secret, f.secret) {
				t.Fatal("upgrade used another password")
			}
			r := f.row.Record
			r.Salt = bytes.Repeat([]byte{1}, 8)
			r.Digest = bytes.Repeat([]byte{2}, 16)
			return r, nil
		},
	}
	f.v = &CredentialVerifier{store: f.store, work: f.work}
	return f
}

func verifyFixture(t *testing.T, f *verificationFixture, want error) CredentialVerification {
	t.Helper()
	got, err := f.v.VerifyPassword(context.Background(), verificationID, f.secret)
	if want != nil {
		if !errors.Is(err, want) || got != (CredentialVerification{}) {
			t.Fatalf("expected zero result and %v, got error=%v result_zero=%v", want, err, got == (CredentialVerification{}))
		}
	} else if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCredentialVerificationCurrentAndUpgrade(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "current read only", true: "legacy same password upgrade"}[legacy], func(t *testing.T) {
			f := newVerificationFixture(t, legacy)
			before, secret := f.row, slices.Clone(f.secret)
			got := verifyFixture(t, f, nil)
			want := before.State
			if legacy {
				want.CredentialVersion++
			}
			if got != (CredentialVerification{State: want, Upgraded: legacy}) || !bytes.Equal(secret, f.secret) {
				t.Fatal("verification state, revisions, or input changed unexpectedly")
			}
			wantEvents := []string{"get", "verify", "state"}
			if legacy {
				wantEvents = []string{"get", "verify", "create", "rehash", "state"}
			} else if !reflect.DeepEqual(f.row, before) {
				t.Fatal("current verification wrote credentials")
			}
			if !slices.Equal(f.events, wantEvents) {
				t.Fatalf("workflow stages = %v", f.events)
			}
		})
	}
}

func TestCredentialVerificationRejectsWithoutUpgrade(t *testing.T) {
	for _, scenario := range []string{"missing", "disabled", "wrong", "invalid id", "invalid credential version", "invalid auth generation", "malformed record"} {
		t.Run(scenario, func(t *testing.T) {
			f := newVerificationFixture(t, true)
			want := ErrCredentials
			switch scenario {
			case "missing":
				f.store.get = func(context.Context, string) (Snapshot, error) { return Snapshot{}, ErrNotFound }
			case "disabled":
				f.row.Enabled = false
			case "wrong":
				f.work.verify = func(context.Context, []byte, password.Record) (password.Verification, error) {
					return password.Verification{}, nil
				}
			case "invalid id":
				f.row.ID = "00000000000000000000000000000002"
				want = ErrStore
			case "invalid credential version":
				f.row.CredentialVersion = 0
				want = ErrStore
			case "invalid auth generation":
				f.row.AuthGeneration = 0
				want = ErrStore
			case "malformed record":
				f.work.verify = func(context.Context, []byte, password.Record) (password.Verification, error) {
					return password.Verification{}, password.ErrRecord
				}
				want = password.ErrRecord
			}
			before := f.row
			verifyFixture(t, f, want)
			if !reflect.DeepEqual(f.row, before) || slices.Contains(f.events, "create") || slices.Contains(f.events, "rehash") || slices.Contains(f.events, "state") {
				t.Fatal("rejected credentials continued the workflow")
			}
		})
	}
}

func TestCredentialVerificationExactFinalState(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, changed := range []string{"id", "disabled", "credential version", "auth generation"} {
			t.Run(map[bool]string{false: "current", true: "upgraded"}[legacy]+"/"+changed, func(t *testing.T) {
				f := newVerificationFixture(t, legacy)
				f.store.state = func(context.Context, string) (State, error) {
					state := f.row.State
					switch changed {
					case "id":
						state.ID = "00000000000000000000000000000002"
					case "disabled":
						state.Enabled = false
					case "credential version":
						state.CredentialVersion++
					case "auth generation":
						state.AuthGeneration++
					}
					return state, nil
				}
				verifyFixture(t, f, ErrConflict)
			})
		}
	}
}
