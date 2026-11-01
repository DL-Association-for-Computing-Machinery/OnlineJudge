package identity

import (
	"bytes"
	"context"
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

type changeStore struct {
	get      func(context.Context, string) (Snapshot, error)
	recovery func(context.Context) (RecoveryState, error)
	replace  func(context.Context, string, Revision, string, password.Record) (Snapshot, error)
}

func (s *changeStore) Get(ctx context.Context, id string) (Snapshot, error) { return s.get(ctx, id) }
func (s *changeStore) RecoveryState(ctx context.Context) (RecoveryState, error) {
	return s.recovery(ctx)
}
func (s *changeStore) ReplacePasswordForSession(ctx context.Context, id string, want Revision, epoch string, r password.Record) (Snapshot, error) {
	return s.replace(ctx, id, want, epoch, r)
}

type changeFixture struct {
	changer              *PasswordChanger
	store                *changeStore
	work                 *verificationWork
	row                  Snapshot
	version              SessionVersion
	events               []string
	current, replacement []byte
	recovery             RecoveryState
}

func newChangeFixture(t *testing.T, legacy bool) *changeFixture {
	t.Helper()
	f := &changeFixture{row: Snapshot{State: State{ID: verificationID, Enabled: true, Revision: Revision{4, 7}},
		Record: password.Record{FormatVersion: 1, Algorithm: "argon2id", ArgonVersion: 0x13,
			Parameters: password.Parameters{MemoryKiB: 8, Iterations: 1, Parallelism: 1, SaltBytes: 8, DigestBytes: 16},
			Salt:       make([]byte, 8), Digest: make([]byte, 16)}},
		version: SessionVersion{AuthGeneration: 7, RecoveryID: verificationID},
		current: []byte(" synthetic\x00old 密码 "), replacement: []byte(" synthetic\x00new 密码 "),
		recovery: RecoveryState{Generation: verificationID, Enabled: true}}
	f.store = &changeStore{
		get: func(_ context.Context, id string) (Snapshot, error) {
			f.events = append(f.events, "get")
			if id != verificationID {
				t.Fatal("wrong subject read")
			}
			return f.row, nil
		},
		recovery: func(context.Context) (RecoveryState, error) {
			f.events = append(f.events, "recovery")
			return f.recovery, nil
		},
		replace: func(_ context.Context, id string, want Revision, epoch string, r password.Record) (Snapshot, error) {
			f.events = append(f.events, "replace")
			if id != verificationID || want != f.row.Revision || !f.row.Enabled || !f.recovery.Enabled || epoch != f.recovery.Generation {
				return Snapshot{}, ErrConflict
			}
			f.row.CredentialVersion++
			f.row.AuthGeneration++
			f.row.Record = r
			return f.row, nil
		},
	}
	f.work = &verificationWork{
		verify: func(_ context.Context, secret []byte, r password.Record) (password.Verification, error) {
			f.events = append(f.events, "verify")
			if !bytes.Equal(secret, f.current) || !samePasswordRecord(r, f.row.Record) {
				t.Fatal("current verification input changed")
			}
			return password.Verification{Match: true, NeedsUpgrade: legacy}, nil
		},
		create: func(_ context.Context, secret []byte) (password.Record, error) {
			f.events = append(f.events, "create")
			if !bytes.Equal(secret, f.replacement) {
				t.Fatal("replacement bytes normalized or changed")
			}
			r := f.row.Record
			r.Salt, r.Digest = bytes.Repeat([]byte{1}, 8), bytes.Repeat([]byte{2}, 16)
			return r, nil
		},
	}
	f.changer = &PasswordChanger{store: f.store, work: f.work}
	return f
}

func changeCall(t *testing.T, f *changeFixture, ctx context.Context, want error) State {
	t.Helper()
	got, err := f.changer.Change(ctx, verificationID, f.version, f.current, f.replacement)
	if want == nil {
		if err != nil {
			t.Fatal("valid password change failed")
		}
	} else if !errors.Is(err, want) || got != (State{}) {
		t.Fatal("failed password change returned authority or wrong classification")
	}
	return got
}

func TestPasswordChangeCurrentAndLegacyHaveSingleAtomicWrite(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		f := newChangeFixture(t, legacy)
		before := f.row.State
		old, replacement := slices.Clone(f.current), slices.Clone(f.replacement)
		got := changeCall(t, f, t.Context(), nil)
		before.CredentialVersion++
		before.AuthGeneration++
		if got != before || !slices.Equal(f.events, []string{"recovery", "get", "verify", "create", "replace"}) ||
			!bytes.Equal(old, f.current) || !bytes.Equal(replacement, f.replacement) {
			t.Fatal("change did not verify/create outside one exact replacement")
		}
	}
}

func TestPasswordChangeRejectsInvalidAndStaleFencesBeforeWork(t *testing.T) {
	for _, kind := range []string{"closed", "epoch changed", "bad recovery", "bad row id", "bad revision", "disabled", "auth changed", "credential overflow", "auth overflow", "not found"} {
		t.Run(kind, func(t *testing.T) {
			f := newChangeFixture(t, false)
			want := ErrConflict
			switch kind {
			case "closed":
				f.recovery.Enabled = false
				want = ErrRecovering
			case "epoch changed":
				f.recovery.Generation = "00000000000000000000000000000002"
			case "bad recovery":
				f.recovery.Generation = "INVALID"
				want = ErrStore
			case "bad row id":
				f.row.ID = "00000000000000000000000000000002"
				want = ErrStore
			case "bad revision":
				f.row.CredentialVersion = 0
				want = ErrStore
			case "disabled":
				f.row.Enabled = false
			case "auth changed":
				f.row.AuthGeneration++
			case "credential overflow":
				f.row.CredentialVersion = math.MaxInt64
			case "auth overflow":
				f.row.AuthGeneration = math.MaxInt64
				f.version.AuthGeneration = math.MaxInt64
			case "not found":
				f.store.get = func(context.Context, string) (Snapshot, error) { return Snapshot{}, ErrNotFound }
				want = ErrCredentials
			}
			changeCall(t, f, t.Context(), want)
			if slices.Contains(f.events, "verify") || slices.Contains(f.events, "replace") {
				t.Fatal("invalid authority reached password work/write")
			}
		})
	}
	f := newChangeFixture(t, false)
	f.version.RecoveryID = ""
	changeCall(t, f, t.Context(), ErrInvalid)
	if len(f.events) != 0 {
		t.Fatal("invalid session fence reached store")
	}
	if _, err := NewPasswordChanger(nil, new(password.BoundedHasher)); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing store accepted")
	}
	if _, err := NewPasswordChanger(f.store, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing shared budget accepted")
	}
	var zero PasswordChanger
	if got, err := zero.Change(t.Context(), verificationID, f.version, nil, nil); got != (State{}) || !errors.Is(err, ErrInvalid) {
		t.Fatal("zero changer accepted")
	}
}

func TestPasswordChangeErrorsDoNotWriteOrRetry(t *testing.T) {
	for _, phase := range []string{"recovery", "get", "verify", "mismatch", "unchanged", "create", "replace"} {
		t.Run(phase, func(t *testing.T) {
			f := newChangeFixture(t, true)
			want := errors.New("synthetic stage failure")
			switch phase {
			case "recovery":
				f.store.recovery = func(context.Context) (RecoveryState, error) { return RecoveryState{}, want }
			case "get":
				f.store.get = func(context.Context, string) (Snapshot, error) { return Snapshot{}, want }
			case "verify":
				f.work.verify = func(context.Context, []byte, password.Record) (password.Verification, error) {
					return password.Verification{}, want
				}
			case "mismatch":
				f.work.verify = func(context.Context, []byte, password.Record) (password.Verification, error) {
					return password.Verification{}, nil
				}
				want = ErrCredentials
			case "unchanged":
				f.replacement = slices.Clone(f.current)
				want = ErrPasswordUnchanged
			case "create":
				f.work.create = func(context.Context, []byte) (password.Record, error) { return password.Record{}, want }
			case "replace":
				f.store.replace = func(context.Context, string, Revision, string, password.Record) (Snapshot, error) {
					f.events = append(f.events, "replace")
					return Snapshot{}, want
				}
			}
			changeCall(t, f, t.Context(), want)
			if f.row.CredentialVersion != 4 || f.row.AuthGeneration != 7 || slices.Contains(f.events, "replace") != (phase == "replace") {
				t.Fatal("failed stage changed authority or reached unexpected write")
			}
		})
	}
}

func TestPasswordChangeCancellationStopsFollowingStages(t *testing.T) {
	for _, phase := range []string{"before", "recovery", "get", "verify", "create", "replace"} {
		t.Run(phase, func(t *testing.T) {
			f := newChangeFixture(t, true)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch phase {
			case "before":
				cancel()
			case "recovery":
				call := f.store.recovery
				f.store.recovery = func(ctx context.Context) (RecoveryState, error) { r, e := call(ctx); cancel(); return r, e }
			case "get":
				call := f.store.get
				f.store.get = func(ctx context.Context, id string) (Snapshot, error) { r, e := call(ctx, id); cancel(); return r, e }
			case "verify":
				call := f.work.verify
				f.work.verify = func(ctx context.Context, p []byte, r password.Record) (password.Verification, error) {
					v, e := call(ctx, p, r)
					cancel()
					return v, e
				}
			case "create":
				call := f.work.create
				f.work.create = func(ctx context.Context, p []byte) (password.Record, error) {
					r, e := call(ctx, p)
					cancel()
					return r, e
				}
			case "replace":
				call := f.store.replace
				f.store.replace = func(ctx context.Context, id string, want Revision, epoch string, r password.Record) (Snapshot, error) {
					s, e := call(ctx, id, want, epoch, r)
					cancel()
					return s, e
				}
			}
			changeCall(t, f, ctx, context.Canceled)
			if phase == "before" {
				if len(f.events) != 0 {
					t.Fatal("canceled call reached store")
				}
			} else if f.events[len(f.events)-1] != phase {
				t.Fatal("canceled stage was followed by additional work")
			}
			want := int64(4)
			if phase == "replace" {
				want++
			}
			if f.row.CredentialVersion != want {
				t.Fatal("cancellation retried or compensated write")
			}
		})
	}
}

func TestPasswordChangeFinalFenceRejectsRaces(t *testing.T) {
	for _, kind := range []string{"change", "disable", "recovery"} {
		t.Run(kind, func(t *testing.T) {
			f := newChangeFixture(t, true)
			create := f.work.create
			f.work.create = func(ctx context.Context, p []byte) (password.Record, error) {
				r, e := create(ctx, p)
				switch kind {
				case "change":
					f.row.CredentialVersion++
					f.row.AuthGeneration++
				case "disable":
					f.row.Enabled = false
					f.row.AuthGeneration++
				case "recovery":
					f.recovery.Generation = "00000000000000000000000000000002"
				}
				return r, e
			}
			changeCall(t, f, t.Context(), ErrConflict)
			if !slices.Equal(f.events, []string{"recovery", "get", "verify", "create", "replace"}) {
				t.Fatal("race triggered reread or retry")
			}
		})
	}
}

func TestPasswordChangeRejectsInconsistentWriteReceipt(t *testing.T) {
	for _, kind := range []string{"id", "enabled", "credential", "auth", "record", "salt", "digest"} {
		t.Run(kind, func(t *testing.T) {
			f := newChangeFixture(t, false)
			replace := f.store.replace
			f.store.replace = func(ctx context.Context, id string, want Revision, epoch string, r password.Record) (Snapshot, error) {
				row, e := replace(ctx, id, want, epoch, r)
				switch kind {
				case "id":
					row.ID = "00000000000000000000000000000002"
				case "enabled":
					row.Enabled = false
				case "credential":
					row.CredentialVersion--
				case "auth":
					row.AuthGeneration++
				case "record":
					row.Record.Parameters.Iterations++
				case "salt":
					row.Record.Salt = slices.Clone(row.Record.Salt)
					row.Record.Salt[0]++
				case "digest":
					row.Record.Digest = slices.Clone(row.Record.Digest)
					row.Record.Digest[0]++
				}
				return row, e
			}
			changeCall(t, f, t.Context(), ErrStore)
			if f.row.CredentialVersion != 5 || f.row.AuthGeneration != 8 {
				t.Fatal("bad receipt compensated committed change")
			}
		})
	}
}

func TestPasswordChangeUnknownCommitPreservesStorageClassification(t *testing.T) {
	f := newChangeFixture(t, true)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	unknown := errors.Join(ErrStore, context.Canceled)
	replace := f.store.replace
	f.store.replace = func(ctx context.Context, id string, want Revision, epoch string, r password.Record) (Snapshot, error) {
		_, e := replace(ctx, id, want, epoch, r)
		if e != nil {
			t.Fatal("fixture write failed")
		}
		cancel()
		return Snapshot{}, unknown
	}
	got, err := f.changer.Change(ctx, verificationID, f.version, f.current, f.replacement)
	if got != (State{}) || err != unknown || !errors.Is(err, ErrStore) || !slices.Equal(f.events, []string{"recovery", "get", "verify", "create", "replace"}) || f.row.CredentialVersion != 5 || f.row.AuthGeneration != 8 {
		t.Fatal("unknown committed change was authorized, retried or reclassified")
	}
}
