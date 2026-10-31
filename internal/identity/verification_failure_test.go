package identity

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

func TestCredentialVerifierInvalidAndPreCanceled(t *testing.T) {
	for _, v := range []*CredentialVerifier{nil, {}} {
		got, err := v.VerifyPassword(context.Background(), verificationID, nil)
		if got != (CredentialVerification{}) || !errors.Is(err, ErrInvalid) {
			t.Fatal("zero verifier did not fail closed")
		}
	}
	if v, err := NewCredentialVerifier(nil, &password.BoundedHasher{}); v != nil || !errors.Is(err, ErrInvalid) {
		t.Fatal("nil store accepted")
	}
	f := newVerificationFixture(t, true)
	if v, err := NewCredentialVerifier(f.store, nil); v != nil || !errors.Is(err, ErrInvalid) {
		t.Fatal("nil password worker accepted")
	}
	for _, id := range []string{"", "person@example.invalid", "../subject"} {
		if got, err := f.v.VerifyPassword(context.Background(), id, f.secret); got != (CredentialVerification{}) || !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid opaque subject accepted")
		}
	}
	// Deliberately exercise an absent context as invalid boundary input.
	var absent context.Context
	if got, err := f.v.VerifyPassword(absent, verificationID, f.secret); got != (CredentialVerification{}) || !errors.Is(err, ErrInvalid) {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := f.v.VerifyPassword(ctx, verificationID, f.secret); got != (CredentialVerification{}) || !errors.Is(err, context.Canceled) || len(f.events) != 0 {
		t.Fatal("pre-canceled call started work")
	}
}

func TestCredentialVerificationUpgradeFailuresAreFatal(t *testing.T) {
	for _, want := range []error{password.ErrBusy, password.ErrQueueTimeout, password.ErrRandom} {
		t.Run(want.Error(), func(t *testing.T) {
			f := newVerificationFixture(t, true)
			f.work.create = func(context.Context, []byte) (password.Record, error) { return password.Record{}, want }
			verifyFixture(t, f, want)
			if f.row.CredentialVersion != 4 || slices.Contains(f.events, "rehash") || slices.Contains(f.events, "state") {
				t.Fatal("failed upgrade fell back to successful verification")
			}
		})
	}
	f := newVerificationFixture(t, true)
	f.row.CredentialVersion = math.MaxInt64
	verifyFixture(t, f, ErrConflict)
	if slices.Contains(f.events, "create") || slices.Contains(f.events, "rehash") {
		t.Fatal("overflowing upgrade computed or wrote credentials")
	}
}

func TestCredentialVerificationChecksUpgradeReturnContract(t *testing.T) {
	for _, changed := range []string{"id", "disabled", "unchanged version", "skipped version", "auth generation"} {
		t.Run(changed, func(t *testing.T) {
			f := newVerificationFixture(t, true)
			rehash := f.store.rehash
			f.store.rehash = func(ctx context.Context, id string, rev Revision, r password.Record) (Snapshot, error) {
				row, err := rehash(ctx, id, rev, r)
				switch changed {
				case "id":
					row.ID = "00000000000000000000000000000002"
				case "disabled":
					row.Enabled = false
				case "unchanged version":
					row.CredentialVersion--
				case "skipped version":
					row.CredentialVersion++
				case "auth generation":
					row.AuthGeneration++
				}
				return row, err
			}
			verifyFixture(t, f, ErrStore)
			if slices.Contains(f.events, "state") {
				t.Fatal("invalid upgrade return was trusted for final verification")
			}
		})
	}
}

func TestCredentialVerificationPreservesStorageErrors(t *testing.T) {
	for _, phase := range []string{"get", "rehash", "state"} {
		t.Run(phase, func(t *testing.T) {
			f := newVerificationFixture(t, true)
			want := errors.Join(ErrStore, errors.New("synthetic storage failure"))
			switch phase {
			case "get":
				f.store.get = func(context.Context, string) (Snapshot, error) { return Snapshot{}, want }
			case "rehash":
				f.store.rehash = func(context.Context, string, Revision, password.Record) (Snapshot, error) { return Snapshot{}, want }
			case "state":
				f.store.state = func(context.Context, string) (State, error) { return State{}, want }
			}
			got, err := f.v.VerifyPassword(context.Background(), verificationID, f.secret)
			if got != (CredentialVerification{}) || err != want {
				t.Fatal("storage error replaced or returned a successful result")
			}
			if phase == "state" && f.row.CredentialVersion != 5 {
				t.Fatal("failed final read compensated a committed upgrade")
			}
			rehashes := 0
			for _, event := range f.events {
				if event == "rehash" {
					rehashes++
				}
			}
			if rehashes > 1 {
				t.Fatal("storage failure retried the upgrade")
			}
		})
	}
}

func TestCredentialVerificationUncertainCommittedUpgradePreservesClassification(t *testing.T) {
	f := newVerificationFixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	unknown := errors.Join(ErrStore, context.Canceled)
	rehash := f.store.rehash
	f.store.rehash = func(ctx context.Context, id string, rev Revision, r password.Record) (Snapshot, error) {
		_, err := rehash(ctx, id, rev, r)
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		return Snapshot{}, unknown
	}
	got, err := f.v.VerifyPassword(ctx, verificationID, f.secret)
	if got != (CredentialVerification{}) || err != unknown || !errors.Is(err, ErrStore) || !errors.Is(err, context.Canceled) {
		t.Fatal("lost uncertain-write classification or returned a result")
	}
	if f.row.CredentialVersion != 5 || !slices.Equal(f.events, []string{"get", "verify", "create", "rehash"}) {
		t.Fatal("unknown write was reread, retried or compensated")
	}
}

func TestCredentialVerificationCancellationStopsFollowingStages(t *testing.T) {
	for _, phase := range []string{"get", "verify", "create", "rehash", "state"} {
		t.Run(phase, func(t *testing.T) {
			f := newVerificationFixture(t, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch phase {
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
			case "rehash":
				call := f.store.rehash
				f.store.rehash = func(ctx context.Context, id string, rev Revision, r password.Record) (Snapshot, error) {
					s, e := call(ctx, id, rev, r)
					cancel()
					return s, e
				}
			case "state":
				call := f.store.state
				f.store.state = func(ctx context.Context, id string) (State, error) { s, e := call(ctx, id); cancel(); return s, e }
			}
			got, err := f.v.VerifyPassword(ctx, verificationID, f.secret)
			if got != (CredentialVerification{}) || !errors.Is(err, context.Canceled) || f.events[len(f.events)-1] != phase {
				t.Fatal("cancellation returned success or started a following stage")
			}
			want := int64(4)
			if phase == "rehash" || phase == "state" {
				want = 5
			}
			if f.row.CredentialVersion != want {
				t.Fatal("cancellation lost or compensated the actual upgrade")
			}
		})
	}
}
