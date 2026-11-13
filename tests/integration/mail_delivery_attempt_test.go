//go:build integration

package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailmessage"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/maildelivery"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailqueue"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailtransport"
)

func TestAccountMailDeliveryAttempt(t *testing.T) {
	f := newBacklogFixture(t)
	h := identityHasher(t, false)
	issuer, err := identitypg.New(f.a, h)
	must(t, err)
	subject := identityID(1)
	addr, err := identity.ParseEmail("Delivery+tag@example.invalid")
	must(t, err)
	record := identityRecord(t, h, "synthetic mail delivery attempt password")
	_, err = issuer.InitializePersonal(f.ctx, subject, addr, true, record)
	must(t, err)
	epoch, err := issuer.BeginRecovery(f.ctx)
	must(t, err)
	must(t, issuer.FinishRecovery(f.ctx, epoch))
	protector, err := emailtoken.NewProtector("fixture", map[string][32]byte{"fixture": {1}})
	must(t, err)
	// The callback stands in for the queue: this test drives already claimed jobs.
	id, err := issuer.IssueEmailToken(f.ctx, subject, identity.SessionVersion{AuthGeneration: 1, RecoveryID: epoch},
		emailtoken.Activation, protector, func(context.Context, string) error { return nil },
		func(context.Context, pgx.Tx, string) error { return nil })
	must(t, err)
	reader, err := identitypg.NewEmailDeliveryReader(f.b, protector)
	must(t, err)
	renderer, err := emailmessage.New("https://oj.example.invalid")
	must(t, err)
	calls := 0
	var link string
	var reply mailtransport.Outcome
	var cancelWhileSending context.CancelFunc
	var latestDeadline time.Time
	const linkPrefix = "https://oj.example.invalid/account/activate#token="
	permitCalls := 0
	attempt, err := maildelivery.New(reader, renderer,
		func(context.Context) (bool, error) { permitCalls++; return true, nil },
		func(ctx context.Context, to identity.Email, subject, body string) mailtransport.Outcome {
			calls++
			deadline, ok := ctx.Deadline()
			if !ok || deadline.After(latestDeadline) || ctx.Err() != nil {
				t.Error("SMTP did not receive a live, bounded context")
			}
			if to != addr || subject == "" {
				t.Error("rendering did not preserve the verified envelope inputs")
			}
			_, candidate, found := strings.Cut(body, linkPrefix)
			if !found {
				t.Error("mail did not contain the trusted activation link")
			}
			candidate, _, _ = strings.Cut(candidate, "\n")
			if link != "" && candidate != link {
				t.Error("retry generated a different link")
			}
			link = candidate
			if cancelWhileSending != nil {
				cancelWhileSending()
			}
			return reply
		})
	must(t, err)
	prepared, err := reader.Prepare(f.ctx, id)
	must(t, err)
	if d := prepared.SendDeadline(); d == d.Round(0) {
		t.Fatal("prepared deadline does not use the local monotonic clock")
	}
	latestDeadline = time.Now().Add(emailtoken.MaxSendWindow)
	for _, tc := range []struct {
		from mailtransport.Outcome
		want maildelivery.Outcome
	}{
		{mailtransport.TemporaryFailure, maildelivery.Retry},
		{mailtransport.Unknown, maildelivery.Unknown},
		{mailtransport.PermanentFailure, maildelivery.Rejected},
		{mailtransport.Accepted, maildelivery.Accepted},
	} {
		reply = tc.from
		if got := attempt.Run(f.ctx, id); got != tc.want {
			t.Fatalf("delivery result = %d, want %d", got, tc.want)
		}
	}
	if calls != 4 || link != prepared.Token().LinkValue() {
		t.Fatal("one transport call per attempt must carry the issued credential")
	}
	for _, tc := range []struct {
		reply mailtransport.Outcome
		want  maildelivery.Outcome
	}{{mailtransport.Accepted, maildelivery.Accepted}, {mailtransport.Unknown, maildelivery.Unknown}} {
		ctx, cancelSend := context.WithCancel(f.ctx)
		reply, cancelWhileSending = tc.reply, cancelSend
		got := attempt.Run(ctx, id)
		cancelSend()
		if got != tc.want {
			t.Fatalf("cancellation during SMTP erased result %d: got %d", tc.want, got)
		}
	}
	cancelWhileSending = nil
	reply = mailtransport.Accepted

	short, cancel := context.WithTimeout(f.ctx, time.Second)
	defer cancel()
	latestDeadline, _ = short.Deadline()
	if attempt.Run(short, id) != maildelivery.Accepted || calls != 7 {
		t.Fatal("caller deadline did not permit a single bounded send")
	}
	cancel()
	if attempt.Run(short, id) != maildelivery.Retry || calls != 7 {
		t.Fatal("canceled job reached SMTP")
	}
	if attempt.Run(f.ctx, identityID(99)) != maildelivery.Obsolete || calls != 7 {
		t.Fatal("missing payload reached SMTP")
	}
	if permitCalls != 7 {
		t.Fatal("canceled or missing payload consumed a permit")
	}
	_, err = f.a.Exec(f.ctx, "INSERT INTO account_mail_send_rate (spacing_ms) VALUES ($1)", 30000)
	must(t, err)
	firstRate, err := mailqueue.NewSendRate(f.ctx, f.a)
	must(t, err)
	secondRate, err := mailqueue.NewSendRate(f.ctx, f.b)
	must(t, err)
	sent := 0
	send := func(context.Context, identity.Email, string, string) mailtransport.Outcome {
		sent++
		return mailtransport.Accepted
	}
	first, err := maildelivery.New(reader, renderer, firstRate.TryTake, send)
	must(t, err)
	second, err := maildelivery.New(reader, renderer, secondRate.TryTake, send)
	must(t, err)
	if first.Run(f.ctx, id) != maildelivery.Accepted || second.Run(f.ctx, id) != maildelivery.Deferred || sent != 1 {
		t.Fatal("shared rate did not defer the second sender before SMTP")
	}
	_, err = f.a.Exec(f.ctx, "DELETE FROM account_mail_send_rate")
	must(t, err)
	if second.Run(f.ctx, id) != maildelivery.Deferred || sent != 1 {
		t.Fatal("missing rate state allowed SMTP")
	}
	waiting, stopWaiting := context.WithCancel(f.ctx)
	defer stopWaiting()
	interrupted, err := maildelivery.New(reader, renderer, func(context.Context) (bool, error) {
		stopWaiting()
		return true, nil
	}, send)
	must(t, err)
	if interrupted.Run(waiting, id) != maildelivery.Deferred || sent != 1 {
		t.Fatal("cancellation while requesting a permit still allowed SMTP")
	}
	shortWindow := prepared.Scope()
	shortWindow.SendUntil = shortWindow.IssuedAt.Add(2 * time.Second)
	// Place the two-second window relative to the database clock instead
	// of counting on the initial fixture setup to take less than two seconds.
	var dbNow time.Time
	must(t, f.a.QueryRow(f.ctx, "SELECT clock_timestamp()").Scan(&dbNow))
	shift := dbNow.Truncate(time.Microsecond).Sub(shortWindow.IssuedAt)
	shortWindow.IssuedAt = shortWindow.IssuedAt.Add(shift)
	shortWindow.SendUntil = shortWindow.SendUntil.Add(shift)
	shortWindow.TokenExpiry = shortWindow.TokenExpiry.Add(shift)
	sealed, err := protector.Seal(shortWindow.IssuedAt, shortWindow, prepared.Token())
	must(t, err)
	tx, err := f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, "UPDATE identity_email_tokens SET issued_at=$2,expires_at=$3 WHERE id=$1",
		id, shortWindow.IssuedAt, shortWindow.TokenExpiry)
	must(t, err)
	_, err = tx.Exec(f.ctx, "UPDATE identity_email_payloads SET send_until=$2,ciphertext=$3 WHERE token_id=$1",
		id, shortWindow.SendUntil, sealed.Ciphertext)
	must(t, err)
	must(t, tx.Commit(f.ctx))
	entered := false
	expires, err := maildelivery.New(reader, renderer, func(ctx context.Context) (bool, error) {
		entered = true
		<-ctx.Done()
		return true, nil
	}, send)
	must(t, err)
	if expires.Run(f.ctx, id) != maildelivery.Obsolete || !entered || sent != 1 {
		t.Fatal("expiry while requesting a permit still allowed SMTP")
	}
	_, err = f.a.Exec(f.ctx, "UPDATE identity_email_payloads SET send_until=clock_timestamp()-interval '1 second' WHERE token_id=$1", id)
	must(t, err)
	if attempt.Run(f.ctx, id) != maildelivery.Obsolete || calls != 7 {
		t.Fatal("expired payload reached SMTP")
	}
	original := prepared.Scope()
	originalSeal, err := protector.Seal(original.IssuedAt, original, prepared.Token())
	must(t, err)
	tx, err = f.a.Begin(f.ctx)
	must(t, err)
	defer func() { _ = tx.Rollback(f.ctx) }()
	_, err = tx.Exec(f.ctx, "UPDATE identity_email_tokens SET issued_at=$2,expires_at=$3 WHERE id=$1",
		id, original.IssuedAt, original.TokenExpiry)
	must(t, err)
	_, err = tx.Exec(f.ctx, "UPDATE identity_email_payloads SET send_until=$2,ciphertext=$3 WHERE token_id=$1",
		id, original.SendUntil, originalSeal.Ciphertext)
	must(t, err)
	must(t, tx.Commit(f.ctx))
	must(t, issuer.ActivateEmail(f.ctx, prepared.Token(), record))
	if attempt.Run(f.ctx, id) != maildelivery.Obsolete || calls != 7 {
		t.Fatal("consumed credential reached SMTP")
	}
	if permitCalls != 7 {
		t.Fatal("expired or consumed payload consumed a permit")
	}
}
