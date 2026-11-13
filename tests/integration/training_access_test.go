//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
)

func TestTrainingAccountAuthority(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	first, err := identitypg.New(f.a, h)
	must(t, err)
	second, err := identitypg.New(f.b, h)
	must(t, err)
	addr, err := identity.ParseEmail("training@example.invalid")
	must(t, err)
	record := identityRecord(t, h, "synthetic training fixture password")
	personal, err := first.InitializePersonal(f.ctx, identityID(801), addr, true, record)
	must(t, err)
	generic, err := first.Initialize(f.ctx, identityID(802), true, record)
	must(t, err)
	epoch, err := first.BeginRecovery(f.ctx)
	must(t, err)
	actor := identity.TrainingActor{SubjectID: personal.ID, SessionVersion: identity.SessionVersion{
		AuthGeneration: personal.AuthGeneration, RecoveryID: epoch,
	}}
	read := func(a identity.TrainingActor, want error) {
		t.Helper()
		tx, err := f.a.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		must(t, err)
		defer func() { _ = tx.Rollback(f.ctx) }()
		if err := identitypg.LockTrainingAccount(f.ctx, tx, a); !errors.Is(err, want) {
			t.Fatalf("training authority = %v; want %v", err, want)
		}
	}
	read(actor, identity.ErrRecovering)
	must(t, first.FinishRecovery(f.ctx, epoch))
	read(actor, identity.ErrTrainingAccountRequired)
	missingEmail := actor
	missingEmail.SubjectID = generic.ID
	read(missingEmail, identity.ErrTrainingAccountRequired)
	read(identity.TrainingActor{}, identity.ErrInvalid)
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	canceled, cancel := context.WithCancel(f.ctx)
	cancel()
	if err := identitypg.LockTrainingAccount(canceled, tx, actor); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled call = %v", err)
	}
	must(t, tx.Rollback(f.ctx))
	if err := identitypg.LockTrainingAccount(f.ctx, tx, actor); !errors.Is(err, identity.ErrStore) {
		t.Fatalf("closed transaction = %v", err)
	}

	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	payload, err := first.IssueEmailToken(f.ctx, actor.SubjectID, actor.SessionVersion, emailtoken.Activation, protector,
		func(context.Context, *emailtoken.Protector) error { return nil }, func(context.Context, pgx.Tx, string) error { return nil })
	must(t, err)
	reader, err := identitypg.NewEmailDeliveryReader(f.b, protector)
	must(t, err)
	mail, err := reader.Prepare(f.ctx, payload)
	must(t, err)
	must(t, second.ActivateEmail(f.ctx, mail.Token(), identityRecord(t, h, "synthetic activated training password")))
	read(actor, identity.ErrConflict)
	personal, err = first.PersonalByID(f.ctx, personal.ID)
	must(t, err)
	actor.AuthGeneration = personal.AuthGeneration
	read(actor, nil)

	setVerified := func(value bool) error {
		// Fixture-only email state change, used to isolate the email row lock
		// from the account lock shared by production activation.
		_, err := f.b.Exec(f.ctx, "UPDATE identity_personal_emails SET email_verified=$2 WHERE subject_id=$1", actor.SubjectID, value)
		return err
	}
	// Keep the same business transaction open and prove competing changes
	// actually wait on it, then check their effect on a fresh transaction.
	whileAuthorized := func(change func() error) {
		t.Helper()
		tx, err := f.a.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		must(t, err)
		defer func() { _ = tx.Rollback(f.ctx) }()
		var pid int32
		must(t, tx.QueryRow(f.ctx, "SELECT pg_backend_pid()").Scan(&pid))
		must(t, identitypg.LockTrainingAccount(f.ctx, tx, actor))
		done := make(chan error, 1)
		go func() { done <- change() }()
		waitIdentityBlocked(t, f.ctx, f.a, pid)
		must(t, tx.Commit(f.ctx))
		must(t, awaitAuthority(t, done))
	}
	whileAuthorized(func() error { return setVerified(false) })
	read(actor, identity.ErrTrainingAccountRequired)
	must(t, setVerified(true))
	var nextEpoch string
	whileAuthorized(func() error {
		var err error
		nextEpoch, err = second.BeginRecovery(f.ctx)
		return err
	})
	read(actor, identity.ErrRecovering)
	must(t, second.FinishRecovery(f.ctx, nextEpoch))
	read(actor, identity.ErrConflict)
	actor.RecoveryID = nextEpoch
	var disabled identity.Revision
	whileAuthorized(func() error {
		var err error
		disabled, err = second.DisableCAS(f.ctx, personal.ID, personal.Revision)
		return err
	})
	read(actor, identity.ErrConflict)
	actor.AuthGeneration = disabled.AuthGeneration
	read(actor, identity.ErrConflict) // Current generation still cannot bypass disabled status.
}
