package emailconfirm_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/netip"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailconfirm"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
)

type activationStub struct {
	activate func(password.Record) error
}

func (s activationStub) ActivateEmail(_ context.Context, _ emailtoken.Token, r password.Record) error {
	return s.activate(r)
}

func (activationStub) ResetPasswordByEmail(context.Context, emailtoken.Token, password.Record) error {
	panic("unexpected reset")
}

func TestConfirmationDoesNotRetryUnknownCommitOrWriteAfterCancellation(t *testing.T) {
	h, err := password.New(password.Policy{Current: password.Parameters{
		MemoryKiB: 64, Iterations: 1, Parallelism: 1, SaltBytes: 16, DigestBytes: 32,
	}}, password.Limits{MaxMemoryKiB: 64, MaxIterations: 1, MaxParallelism: 1, MaxPasswordBytes: 128, MaxSaltBytes: 16, MaxDigestBytes: 32})
	if err != nil {
		t.Fatal(err)
	}
	work, err := password.NewBounded(h, password.AdmissionLimits{MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	rules, err := password.NewAcceptance(password.MinimumNewPasswordCharacters, work.MaxPasswordBytes(),
		[][32]byte{sha256.Sum256([]byte("synthetic blocked password"))})
	if err != nil {
		t.Fatal(err)
	}
	token, err := emailtoken.New()
	if err != nil {
		t.Fatal(err)
	}
	unknown := errors.New("synthetic unknown commit")
	calls := 0
	store := activationStub{activate: func(r password.Record) error {
		calls++
		if len(r.Digest) != 32 {
			t.Fatal("expected a current password record")
		}
		return unknown
	}}
	source := netip.MustParseAddr("192.0.2.1")
	secret := []byte("synthetic replacement password")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	service, err := emailconfirm.New(store, work, rules, func(context.Context, netip.Addr, emailtoken.Purpose, [32]byte) error {
		cancel()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Activate(ctx, source, token.LinkValue(), secret); got != context.Canceled || calls != 0 {
		t.Fatalf("canceled admission: %v, calls=%d", got, calls)
	}
	service, err = emailconfirm.New(store, work, rules, func(context.Context, netip.Addr, emailtoken.Purpose, [32]byte) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := service.Activate(t.Context(), source, token.LinkValue(), secret); got != unknown || calls != 1 {
		t.Fatalf("unknown commit: %v, calls=%d", got, calls)
	}
	if _, err := emailconfirm.New(store, work, rules, nil); err != identity.ErrInvalid {
		t.Fatal("missing quota owner was accepted")
	}
}
