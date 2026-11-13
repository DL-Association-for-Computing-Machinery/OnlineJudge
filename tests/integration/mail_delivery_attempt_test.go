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
	attempt, err := maildelivery.New(reader, renderer,
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
	_, err = f.a.Exec(f.ctx, "UPDATE identity_email_payloads SET send_until=clock_timestamp()-interval '1 second' WHERE token_id=$1", id)
	must(t, err)
	if attempt.Run(f.ctx, id) != maildelivery.Obsolete || calls != 7 {
		t.Fatal("expired payload reached SMTP")
	}
	_, err = f.a.Exec(f.ctx, "UPDATE identity_email_payloads SET send_until=$2 WHERE token_id=$1", id, prepared.Scope().SendUntil)
	must(t, err)
	must(t, issuer.ActivateEmail(f.ctx, prepared.Token(), record))
	if attempt.Run(f.ctx, id) != maildelivery.Obsolete || calls != 7 {
		t.Fatal("consumed credential reached SMTP")
	}
}
