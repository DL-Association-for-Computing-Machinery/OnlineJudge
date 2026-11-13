//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/netip"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailconfirm"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func TestEmailConfirmationAppliesPasswordPolicy(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	consumer, err := identitypg.New(f.b, h)
	must(t, err)
	work, err := password.NewBounded(h, password.AdmissionLimits{MaxInFlight: 1})
	must(t, err)
	const weak = "synthetic common password"
	rules, err := password.NewAcceptance(password.MinimumNewPasswordCharacters, work.MaxPasswordBytes(),
		[][32]byte{sha256.Sum256([]byte(weak))})
	must(t, err)
	keys, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	reader, err := identitypg.NewEmailDeliveryReader(f.a, keys)
	must(t, err)
	addr, err := identity.ParseEmail("confirm@example.invalid")
	must(t, err)
	subject := identityID(1)
	_, err = issuer.InitializePersonal(f.ctx, subject, addr, true, identityRecord(t, h, "synthetic placeholder password"))
	must(t, err)
	epoch, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	must(t, issuer.FinishRecovery(f.ctx, epoch))
	version := identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch}
	_, err = f.a.Exec(f.ctx, "CREATE TABLE fixture_confirmation_jobs(id text PRIMARY KEY)")
	must(t, err)
	issue := func(p emailtoken.Purpose) emailtoken.Token {
		t.Helper()
		id, err := issuer.IssueEmailToken(f.ctx, subject, version, p, keys,
			func(context.Context, string) error { return nil },
			func(ctx context.Context, tx pgx.Tx, id string) error {
				_, err := tx.Exec(ctx, "INSERT INTO fixture_confirmation_jobs VALUES ($1)", id)
				return err
			})
		must(t, err)
		delivery, err := reader.Prepare(f.ctx, id)
		must(t, err)
		return delivery.Token()
	}
	source := netip.MustParseAddr("192.0.2.7")
	requestSource := netip.MustParseAddr("::ffff:192.0.2.7")
	blocked := errors.New("fixture request admission denied")
	var admittedPurpose emailtoken.Purpose
	var admittedDigest [32]byte
	var charges int
	allow := true
	service, err := emailconfirm.New(consumer, work, rules, func(_ context.Context, addr netip.Addr, p emailtoken.Purpose, digest [32]byte) error {
		if addr != source {
			t.Fatal("expected canonical transport source")
		}
		charges++
		admittedPurpose, admittedDigest = p, digest
		if !allow {
			return blocked
		}
		return nil
	})
	must(t, err)
	activation := issue(emailtoken.Activation)
	expect := func(got, want error) {
		t.Helper()
		if !errors.Is(got, want) {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
	before, err := issuer.Get(f.ctx, subject)
	must(t, err)
	const chosen = " 密码是自己选择的这一串字符 "
	expect(service.Activate(f.ctx, requestSource, "invalid", []byte(chosen)), emailconfirm.ErrLink)
	if charges != 0 {
		t.Fatal("malformed token reached admission")
	}
	allow = false
	expect(service.Activate(f.ctx, requestSource, activation.LinkValue(), []byte("short")), blocked)
	allow = true
	expect(service.Activate(f.ctx, requestSource, activation.LinkValue(), []byte(weak)), password.ErrWeakPassword)
	expect(service.Activate(f.ctx, requestSource, activation.LinkValue(), []byte("short")), password.ErrNewPasswordLength)
	expect(service.Reset(f.ctx, requestSource, activation.LinkValue(), []byte(chosen)), emailconfirm.ErrLink)
	sameIdentity(t, f.ctx, issuer, before)
	must(t, service.Activate(f.ctx, requestSource, activation.LinkValue(), []byte(chosen)))
	wantDigest, err := activation.Digest(emailtoken.Activation)
	must(t, err)
	if charges != 5 || admittedPurpose != emailtoken.Activation || admittedDigest != wantDigest {
		t.Fatal("activation was not admitted with its purpose-specific digest")
	}
	activated, err := issuer.Get(f.ctx, subject)
	must(t, err)
	exact, err := h.Verify([]byte(chosen), activated.Record)
	must(t, err)
	trimmed, err := h.Verify([]byte("密码是自己选择的这一串字符"), activated.Record)
	must(t, err)
	if !exact.Match || trimmed.Match {
		t.Fatal("confirmation did not preserve the exact password bytes")
	}
	expect(service.Activate(f.ctx, requestSource, activation.LinkValue(), []byte(chosen)), emailconfirm.ErrLink)
	version.AuthGeneration++
	recovery := issue(emailtoken.Recovery)
	const replacement = "synthetic mailbox recovery replacement"
	must(t, service.Reset(f.ctx, requestSource, recovery.LinkValue(), []byte(replacement)))
	wantDigest, err = recovery.Digest(emailtoken.Recovery)
	must(t, err)
	if admittedPurpose != emailtoken.Recovery || admittedDigest != wantDigest {
		t.Fatal("reset was not admitted with its purpose-specific digest")
	}
	after, err := issuer.Get(f.ctx, subject)
	must(t, err)
	newMatch, err := h.Verify([]byte(replacement), after.Record)
	must(t, err)
	oldMatch, err := h.Verify([]byte(chosen), after.Record)
	must(t, err)
	if !newMatch.Match || oldMatch.Match || after.AuthGeneration != 3 || after.CredentialVersion != 3 {
		t.Fatal("confirmation did not apply the expected password and generations")
	}
}
